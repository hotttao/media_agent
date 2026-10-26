#!/usr/bin/env python3
"""Private JSON bridge. No backend selection, credentials, or raw CLI logs escape it."""
import argparse
import contextlib
import fcntl
import hashlib
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import sys
import time

UPSTREAM_COMMIT = "7f97846e70e1b82b53baac3499f3272e9f5c1aa9"
UPSTREAM_SHA256 = "3e7065002813d5aaf9aca9ae9ca94677c5ed7a95bd7a0938f8032a1d2f85676b"
KEY = re.compile(r"[a-f0-9]{64}")
CONVERSATION = re.compile(r"https://chatgpt\.com/c/[A-Za-z0-9-]+")


def read(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or info.st_size > 4 << 20:
        raise ValueError("private record unavailable")
    return json.loads(path.read_text())


def write(path, data):
    import tempfile
    fd, temp = tempfile.mkstemp(prefix=".write-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as out:
            json.dump(data, out, ensure_ascii=False, sort_keys=True)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temp, path)
        folder = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(folder)
        finally:
            os.close(folder)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def safe_dir(path):
    if not path.is_absolute() or str(path.resolve()) != str(path):
        raise ValueError("unsafe directory")
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_mode & 0o077:
        raise ValueError("private directory required")


def load_upstream(path):
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != UPSTREAM_SHA256:
        raise ValueError("unsupported image-use revision")
    for key in list(os.environ):
        if key.startswith(("IMAGE_USE_", "CHATGPT_IMAGEGEN_")):
            del os.environ[key]
    os.environ.update(IMAGE_USE_BACKEND="web", IMAGE_USE_NO_UPDATE_CHECK="1", USE_NO_UPDATE_CHECK="1")
    loader = importlib.machinery.SourceFileLoader("worker_image_use_upstream", str(path))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[loader.name] = module
    loader.exec_module(module)
    return module


def manifest(folder):
    record = read(folder / "artifact.json")
    output = folder / "output.png"
    info = output.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 0 < info.st_size <= 64 << 20:
        raise ValueError("invalid output")
    sha = hashlib.sha256(output.read_bytes()).hexdigest()
    if record.get("generation_key") != folder.name or record.get("sha256") != sha or record.get("size") != info.st_size or record.get("artifact_id") != "image-" + sha:
        raise ValueError("output changed")
    return {"state": "succeeded", "path": str(output), **record}


def seal(folder):
    output = folder / "output.png"
    info = output.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 0 < info.st_size <= 64 << 20:
        raise ValueError("invalid output")
    data = output.read_bytes()
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        mime = "image/png"
    elif data.startswith(b"\xff\xd8\xff"):
        mime = "image/jpeg"
    elif data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        mime = "image/webp"
    else:
        raise ValueError("not an image")
    with output.open("rb") as stream:
        os.fsync(stream.fileno())
    sha = hashlib.sha256(data).hexdigest()
    write(folder / "artifact.json", {"artifact_id": "image-" + sha, "sha256": sha, "size": len(data), "media_type": mime, "generation_key": folder.name})
    return manifest(folder)


class ConversationChanged(ValueError):
    pass


def owned_context(module, browser, session, timeout=10):
    script = r"""(() => {
      if (location.hostname !== 'chatgpt.com') return JSON.stringify(null);
      const match = location.pathname.match(/\/c\/([0-9a-f-]{16,})/i);
      const selector = %s, turnSelector = %s;
      const turns = new Map(); let unknown = false;
      document.querySelectorAll(selector).forEach(message => {
        if (message.parentElement?.closest(selector)) return;
        const turn = message.closest(turnSelector);
        const key = turn?.getAttribute('data-turn-key') || message.getAttribute('data-message-id') || message.closest('[data-message-id]')?.getAttribute('data-message-id');
        const body = message.querySelector('[data-user-message-bubble], .whitespace-pre-wrap') || message;
        const text = body.innerText || body.textContent || '';
        if (key) { if (turns.has(key) && turns.get(key) !== text) unknown = true; turns.set(key, text); } else unknown = true;
      });
      return JSON.stringify({conversation: match ? 'https://chatgpt.com/c/' + match[1] : null, turns: [...turns.keys()], texts: [...turns.values()], unknown});
    })()""" % (json.dumps(module.WEB_USER_MSG_SEL), json.dumps(module.WEB_TURN_SEL))
    return module._ab_eval(browser, script, session=session, timeout=timeout)


def text_digest(text):
    normalized = text.replace("\r\n", "\n").replace("\r", "\n").replace("\u00a0", " ").strip()
    return hashlib.sha256(normalized.encode()).hexdigest()


def assert_binding(module, browser, session, folder):
    binding = read(folder / "binding.json")
    observed = owned_context(module, browser, session)
    if not isinstance(observed, dict) or observed.get("unknown") or observed.get("conversation") != binding.get("conversation") or observed.get("turns") != [binding.get("turn_id")] or len(observed.get("texts", [])) != 1 or text_digest(observed["texts"][0]) != binding.get("user_text_sha256"):
        raise ConversationChanged("original conversation or turn changed")
    return binding


def install_binding_hook(module, folder):
    submit = module._submit_web_prompt

    def once(*args, **kwargs):
        if not module._chatgpt_web_lock_held:
            raise ValueError("cross-tool lock required")
        ab, session = args[:2]
        remaining = args[4]
        initial = module._web_composer_state(ab, session, remaining)
        if initial.get("user_turns") != 0:
            raise ConversationChanged("new empty conversation required")
        # An unconfirmed Send must never bind an arbitrary current page. Only
        # the upstream's confirmed single send may proceed to identity binding.
        result = submit(*args, **kwargs)
        expected = text_digest(args[2])
        for _ in range(5):
            state = module._web_composer_state(ab, session, remaining)
            observed = owned_context(module, ab, session, min(5.0, remaining()))
            if state.get("user_turns") == 1 and state.get("text") == "" and isinstance(observed, dict) and not observed.get("unknown") and len(observed.get("turns", [])) == 1 and len(observed.get("texts", [])) == 1 and text_digest(observed["texts"][0]) == expected and CONVERSATION.fullmatch(observed.get("conversation") or ""):
                write(folder / "binding.json", {"conversation": observed["conversation"], "turn_id": observed["turns"][0], "user_text_sha256": expected})
                return result
            time.sleep(0.5)
        raise ConversationChanged("could not bind the confirmed original turn")
    module._submit_web_prompt = once


def run_generation(args, request, folder):
    module = load_upstream(Path(args.upstream))
    install_binding_hook(module, folder)
    # No shell, no prompt in OS argv. These become Python parser arguments only.
    cli = [args.upstream, "--backend", "web", "--profile", "relay",
           "--keep-conversation", "--keep-tab", "--no-style", "--no-progress", "--quiet",
           "--size", request["size"], "--web-model", request["web_model"], "--project", args.project,
           "--timeout", str(args.timeout), "--out", str(folder / "output.png")]
    if args.project:
        cli += ["--require-project"]
    for image in request["images"]:
        cli += ["--ref", image]
    cli += ["--", request["prompt"]]
    sys.argv = cli
    result = module.main()
    if result not in (0, None):
        raise ValueError("generation unconfirmed")
    browser = module._find_agent_browser()
    assert_binding(module, browser, module.CHATGPT_WEB_SESSION, folder)
    return seal(folder)


def recover(args, folder):
    binding = read(folder / "binding.json")
    if not CONVERSATION.fullmatch(binding.get("conversation", "")):
        raise ValueError("invalid conversation")
    module = load_upstream(Path(args.upstream))
    browser = module._find_agent_browser()
    if browser is None or not module._relay_connected(browser):
        raise ValueError("relay disconnected")
    with module._chatgpt_web_turn(False, time.monotonic()):
        if not module._chatgpt_web_lock_held:
            raise ValueError("cross-tool lock required")
        module._ab(browser, "open", binding["conversation"], session=module.CHATGPT_WEB_SESSION, timeout=20)
        deadline = time.monotonic() + 20
        if not module._wait_composer(browser, module.CHATGPT_WEB_SESSION, lambda: max(1.0, deadline - time.monotonic())):
            raise ValueError("conversation not ready")
        assert_binding(module, browser, module.CHATGPT_WEB_SESSION, folder)
        # Upstream recovery normally takes this same lock. Keep the outer lock
        # across identity checks and download, without recursively taking it.
        original_turn = module._chatgpt_web_turn
        @contextlib.contextmanager
        def already_held(*unused):
            if not module._chatgpt_web_lock_held:
                raise ValueError("cross-tool lock lost")
            yield
        module._chatgpt_web_turn = already_held
        try:
            result = module._recover_command([binding["conversation"], "--last", "--quiet", "--timeout", str(min(args.timeout, 120)), "--out", str(folder / "output.png")])
        finally:
            module._chatgpt_web_turn = original_turn
        if result not in (0, None):
            raise ValueError("recovery unconfirmed")
        assert_binding(module, browser, module.CHATGPT_WEB_SESSION, folder)
        return seal(folder)


def preflight(args):
    module = load_upstream(Path(args.upstream))
    browser = module._find_agent_browser()
    if browser is None or not module._relay_connected(browser):
        return {"ready": False}
    deadline = time.monotonic() + 25
    remaining = lambda: max(1.0, deadline - time.monotonic())
    with module._chatgpt_web_turn(False, time.monotonic()):
        if not module._chatgpt_web_lock_held:
            return {"ready": False}
        module._ab(browser, "open", module.WEB_NEW_CHAT_URL, session=module.CHATGPT_WEB_SESSION, timeout=15)
        return {"ready": bool(module._wait_composer(browser, module.CHATGPT_WEB_SESSION, remaining))}


def dispatch(args, request):
    root = Path(args.root)
    safe_dir(root)
    safe_dir(Path(args.input_root))
    index = root / "ledger.json"
    marker = root.parent / ".image-use-ledger-initialized"
    if args.action == "init":
        if not index.exists():
            if marker.exists():
                raise ValueError("initialized ledger missing")
            write(index, {})
        keys = read(index)
        if not isinstance(keys, dict):
            raise ValueError("ledger corrupt")
        write(marker, {"protocol": "image-use-web-v1"})
        return {}
    keys = read(index)
    if not isinstance(keys, dict) or any(not KEY.fullmatch(k) or not KEY.fullmatch(v) for k, v in keys.items()):
        raise ValueError("ledger corrupt")
    if args.action == "preflight":
        return preflight(args)
    key = request.get("key", "")
    if not KEY.fullmatch(key):
        raise ValueError("invalid generation key")
    folder = root / key
    exists = key in keys or folder.exists()
    if key in keys:
        safe_dir(folder)
        saved = read(folder / "request.json")
        digest = hashlib.sha256(json.dumps(saved, sort_keys=True).encode()).hexdigest()
        if saved.get("key") != key or digest != keys[key]:
            raise ValueError("request ledger mismatch")
    if args.action == "lookup":
        if not exists:
            return {"state": "absent"}
        safe_dir(folder)
        saved = read(folder / "request.json")
        if saved.get("key") != key:
            raise ValueError("identity conflict")
        return {"state": "found", "reference": key}
    if args.action == "submit":
        prompt = request.get("prompt")
        images = request.get("images") or []
        if not isinstance(prompt, str) or not prompt.strip() or len(prompt) > 48000 or not isinstance(images, list) or len(images) > 4:
            raise ValueError("invalid request")
        for value in images:
            image = Path(value)
            if not image.is_absolute() or str(image.resolve()) != str(image) or not image.is_relative_to(Path(args.input_root)):
                raise ValueError("reference outside input root")
            info = image.lstat()
            if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 0 < info.st_size <= 16 << 20:
                raise ValueError("invalid reference")
        canonical = {"key": key, "prompt": prompt, "images": images, "size": request.get("size") or args.size, "web_model": request.get("web_model") or args.web_model, "project": args.project}
        digest = hashlib.sha256(json.dumps(canonical, sort_keys=True).encode()).hexdigest()
        if exists:
            if read(folder / "request.json") != canonical:
                raise ValueError("request conflict")
            return {"reference": key}
        folder.mkdir(mode=0o700)
        write(folder / "request.json", canonical)
        keys[key] = digest
        # The irreversible start intent precedes any browser interaction.
        write(index, keys)
        try:
            run_generation(args, canonical, folder)
        except (Exception, SystemExit) as error:
            state = "generating" if (folder / "binding.json").exists() and not isinstance(error, ConversationChanged) else "needs_human"
            write(folder / "status.json", {"state": state, "next_recover": time.time() + 60})
        return {"reference": key}
    if not exists:
        raise ValueError("generation missing")
    safe_dir(folder)
    if (folder / "artifact.json").exists():
        return manifest(folder)
    if args.action == "artifact":
        raise ValueError("artifact missing")
    if args.action != "observe":
        raise ValueError("invalid action")
    if not (folder / "binding.json").exists():
        return {"state": "needs_human"}
    state = read(folder / "status.json") if (folder / "status.json").exists() else {}
    if state.get("state") == "needs_human":
        return {"state": "needs_human"}
    if state.get("next_recover", 0) > time.time():
        return {"state": "generating"}
    # Commit the probe delay before launching a read-only recovery operation.
    write(folder / "status.json", {"state": "generating", "next_recover": time.time() + 60})
    try:
        return recover(args, folder)
    except ConversationChanged:
        write(folder / "status.json", {"state": "needs_human"})
        return {"state": "needs_human"}
    except (Exception, SystemExit):
        return {"state": "generating"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=["init", "preflight", "lookup", "submit", "observe", "artifact"])
    parser.add_argument("--backend", choices=["web"], required=True)
    for name in ["root", "input-root", "upstream", "size", "web-model", "project"]:
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--timeout", type=int, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        request = json.loads(sys.stdin.read((1 << 20) + 1))
        # Serializes direct invocations too; interrupted children release it.
        safe_dir(Path(args.root))
        with (Path(args.root) / ".bridge.lock").open("a+") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            with open(os.devnull, "w") as null, contextlib.redirect_stdout(null), contextlib.redirect_stderr(null):
                result = dispatch(args, request)
        print(json.dumps({"ok": True, **result}))
        return 0
    except (Exception, SystemExit):
        print(json.dumps({"ok": False, "code": "IMAGE_USE_WEB_UNCONFIRMED"}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
