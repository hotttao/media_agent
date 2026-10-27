import importlib.util
import json
import os
from pathlib import Path
import tempfile
import types
import unittest
from unittest.mock import patch
import base64
import contextlib
import subprocess
import sys

spec = importlib.util.spec_from_file_location("bridge", Path(__file__).with_name("image_use_bridge.py"))
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)
PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII=")

class BridgeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "ledger"
        self.inputs = Path(self.temp.name) / "inputs"
        self.root.mkdir(mode=0o700)
        self.inputs.mkdir(mode=0o700)
        self.args = types.SimpleNamespace(action="init", root=str(self.root), input_root=str(self.inputs), upstream="/private/image-use", size="auto", web_model="Thinking", project="", timeout=60)
        bridge.dispatch(self.args, {})
        self.key = "a" * 64
        self.request = {"key": self.key, "prompt": "private\nimage request", "images": []}

    def call(self, action, request=None):
        self.args.action = action
        return bridge.dispatch(self.args, request or {"key": self.key})

    def fake_module(self, fail_after_send=False):
        module = types.SimpleNamespace(_chatgpt_web_lock_held=True, _JS_CONV_URL="url")
        module._web_composer_state = lambda *args: {"user_turns": self.sent, "text": ""}
        module.WEB_USER_MSG_SEL = "[user]"
        module.WEB_STOP_SEL = "[stop]"
        module.WEB_TURN_SEL = "[data-turn-key]"
        module.CHATGPT_WEB_SESSION = "chatgpt-web"
        module._find_agent_browser = lambda: "chrome-use"
        module._relay_connected = lambda browser: True
        module._wait_composer = lambda *args, **kwargs: True
        module._ab = lambda *args, **kwargs: None
        module._chatgpt_web_turn = lambda *args: contextlib.nullcontext()
        module._ab_eval = lambda *args, **kwargs: {"conversation": "https://chatgpt.com/c/12345678-abcd", "turns": ["owned-turn"], "texts": ["prompt"], "unknown": False}
        module._JS_FETCH = "download:%s"
        context = module._ab_eval
        module._ab_eval = lambda browser, script, **kwargs: {"ok": True, "b64": base64.b64encode(PNG).decode()} if script.startswith("download:") else context(browser, script, **kwargs)
        self.sent = 0
        def send(*args, **kwargs): self.sent += 1
        module._submit_web_prompt = send
        def main():
            import sys
            argv = sys.argv
            self.assertEqual(argv[argv.index("--backend") + 1], "web")
            self.assertEqual(argv[argv.index("--profile") + 1], "relay")
            self.assertIn("--no-style", argv)
            self.assertIn("--keep-conversation", argv)
            self.assertIn(self.key, bridge.read(self.root / "ledger.json"))
            module._submit_web_prompt("chrome-use", "chatgpt-web", "prompt", 0, lambda: 10, lambda _: None)
            if fail_after_send: raise SystemExit("private upstream error must not escape")
            Path(argv[argv.index("--out") + 1]).write_bytes(PNG)
            Path(argv[argv.index("--out") + 1]).chmod(0o600)
            return 0
        module.main = main
        def recover(browser, adapter, argv, session, **kwargs):
            self.assertEqual(adapter, "chatgpt/images")
            self.assertEqual(argv, ["https://chatgpt.com/c/12345678-abcd", "--last", "true"])
            return {"images": [{"file_id": "file-owned", "download_url": "https://example.test/owned-image"}]}
        module._site_call = recover
        return module

    def test_web_only_intent_precedes_send_and_manifest_is_exact(self):
        with patch.object(bridge, "load_upstream", return_value=self.fake_module()):
            result = self.call("submit", self.request)
            self.assertEqual(result, {"reference": self.key})
            self.assertEqual(self.call("lookup")["state"], "found")
            output = self.call("artifact")
            self.assertEqual(output["size"], len(PNG))
            self.assertEqual(output["generation_key"], self.key)
            self.assertEqual(self.call("submit", self.request), result)
            self.assertEqual(self.sent, 1)
        with open(output["path"], "ab") as stream: stream.write(b"changed")
        with self.assertRaises(ValueError): self.call("artifact")

    def test_lost_generation_response_recovers_original_conversation_without_resubmit(self):
        with patch.object(bridge, "load_upstream", return_value=self.fake_module(True)):
            self.call("submit", self.request)
            folder = self.root / self.key
            self.assertEqual(self.call("observe")["state"], "generating")
            bridge.write(folder / "status.json", {"next_recover": 0})
            self.assertEqual(self.call("observe")["state"], "succeeded")
            self.call("submit", self.request)
            self.assertEqual(self.sent, 1)

    def test_unknown_send_retains_intent_and_never_retries(self):
        with patch.object(bridge, "run_generation", side_effect=SystemExit("unconfirmed")) as run:
            self.call("submit", self.request)
            self.assertEqual(self.call("observe")["state"], "needs_human")
            self.call("submit", self.request)
            self.assertEqual(run.call_count, 1)

    def test_conflicting_prompt_or_corrupt_ledger_fails_closed(self):
        with patch.object(bridge, "run_generation", side_effect=SystemExit()): self.call("submit", self.request)
        with self.assertRaises(ValueError): self.call("submit", {**self.request, "prompt": "changed"})
        folder = self.root / self.key
        (folder / "request.json").unlink()
        with self.assertRaises(FileNotFoundError): self.call("lookup")
        (self.root / "ledger.json").unlink()
        with self.assertRaises(ValueError): self.call("init", {})

    def test_too_many_or_foreign_references_are_rejected_without_side_effect(self):
        for refs in [["/outside/image.png"], ["unused"] * 5]:
            with self.assertRaises(ValueError): self.call("submit", {**self.request, "images": refs})
        self.assertEqual(bridge.read(self.root / "ledger.json"), {})

    def test_unpinned_upstream_is_not_loaded(self):
        script = self.root / "wrong.py"
        script.write_text("raise RuntimeError('must not execute')")
        with self.assertRaises(ValueError): bridge.load_upstream(script)

    def test_manual_followup_blocks_recovery_instead_of_returning_another_turn_image(self):
        module = self.fake_module(True)
        with patch.object(bridge, "load_upstream", return_value=module):
            self.call("submit", self.request)
            bridge.write(self.root / self.key / "status.json", {"next_recover": 0})
            module._ab_eval = lambda *args, **kwargs: {"conversation": "https://chatgpt.com/c/12345678-abcd", "turns": ["owned-turn", "manual-followup"], "texts": ["prompt", "followup"], "unknown": False}
            self.assertEqual(self.call("observe")["state"], "needs_human")
            self.assertEqual(self.sent, 1)
            self.assertFalse((self.root / self.key / "artifact.json").exists())

    def test_pre_send_or_unconfirmed_send_error_never_binds_an_unrelated_conversation(self):
        module = self.fake_module()
        def unconfirmed(*args, **kwargs):
            self.sent += 1
            raise SystemExit("unconfirmed send")
        module._submit_web_prompt = unconfirmed
        with patch.object(bridge, "load_upstream", return_value=module):
            self.call("submit", self.request)
            self.assertFalse((self.root / self.key / "binding.json").exists())
            self.assertEqual(self.call("observe")["state"], "needs_human")

    def test_idle_preflight_never_navigates_or_accepts_drafts_and_busy_pages(self):
        module = self.fake_module()
        module.WEB_COMPOSER_SEL = "#composer"
        opened = []
        module._ab = lambda *args, **kwargs: opened.append(args)
        for state, ready in [({"host": "chatgpt.com", "composer": True, "draft": False, "busy": False}, True),
                             ({"host": "chatgpt.com", "composer": True, "draft": True, "busy": False}, False),
                             ({"host": "chatgpt.com", "composer": True, "draft": False, "busy": True}, False)]:
            module._ab_eval = lambda *args, **kwargs: state
            with patch.object(bridge, "load_upstream", return_value=module):
                self.assertEqual(bridge.preflight(self.args)["ready"], ready)
        self.assertEqual(opened, [])

    def test_process_death_after_intent_does_not_repeat_generation_on_restart(self):
        child = r"""
import importlib.util,json,os,sys
from pathlib import Path
import types
source, root, inputs, action = sys.argv[1:]
spec=importlib.util.spec_from_file_location('bridge',source)
b=importlib.util.module_from_spec(spec);spec.loader.exec_module(b)
a=types.SimpleNamespace(action=action,root=root,input_root=inputs,upstream='/unused',size='auto',web_model='Instant,Auto',project='',timeout=60)
def crash(*args):
 p=Path(root)/'generation-calls';p.write_text((p.read_text() if p.exists() else '')+'1')
 os._exit(23)
b.run_generation=crash
request={'key':'a'*64,'prompt':'private image request','images':[]}
print(json.dumps(b.dispatch(a,request)))
"""
        arguments = [sys.executable, "-c", child, str(Path(bridge.__file__).resolve()), str(self.root), str(self.inputs)]
        first = subprocess.run(arguments + ["submit"], capture_output=True, text=True)
        self.assertEqual(first.returncode, 23)
        lookup = subprocess.run(arguments + ["lookup"], capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(lookup.stdout)["state"], "found")
        second = subprocess.run(arguments + ["submit"], capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(second.stdout)["reference"], self.key)
        self.assertEqual((self.root / "generation-calls").read_text(), "1")
        self.assertEqual(self.call("observe")["state"], "needs_human")

    def test_delayed_conversation_identity_binds_without_sending_again(self):
        module = self.fake_module()
        observed = module._ab_eval
        probes = []
        def delayed(*args, **kwargs):
            probes.append(True)
            if len(probes) <= 2:
                raise ValueError("transient readback failure")
            if len(probes) <= 6:
                return {"conversation": None, "turns": [], "texts": [], "unknown": True}
            return observed(*args, **kwargs)
        module._ab_eval = delayed
        with patch.object(bridge, "load_upstream", return_value=module), patch.object(bridge.time, "sleep"):
            self.call("submit", self.request)
        self.assertGreater(len(probes), 6)
        self.assertEqual(self.sent, 1)
        self.assertEqual(self.call("observe")["state"], "succeeded")

    def test_recovery_deduplicates_file_appearances_and_keeps_tab_for_identity_check(self):
        module = self.fake_module(fail_after_send=True)
        with patch.object(bridge, "load_upstream", return_value=module):
            self.call("submit", self.request)
            original = module._site_call
            module._site_call = lambda *args, **kwargs: {"images": original(*args, **kwargs)["images"] * 2}
            opened = []
            module._ab = lambda *args, **kwargs: opened.append(args[1])
            with patch.object(bridge.time, "time", return_value=10**12):
                self.assertEqual(self.call("observe")["state"], "succeeded")
        self.assertNotIn("close", opened)
        self.assertEqual(self.sent, 1)

    def test_recovery_refuses_multiple_distinct_images_without_selecting_one(self):
        module = self.fake_module(fail_after_send=True)
        with patch.object(bridge, "load_upstream", return_value=module):
            self.call("submit", self.request)
            module._site_call = lambda *args, **kwargs: {"images": [{"file_id": "a", "download_url": "https://example.test/a"}, {"file_id": "b", "download_url": "https://example.test/b"}]}
            with patch.object(bridge.time, "time", return_value=10**12):
                self.assertEqual(self.call("observe")["state"], "needs_human")
        self.assertFalse((self.root / self.key / "output.png").exists())
        self.assertEqual(self.sent, 1)

    def test_artifact_cannot_be_a_symlink(self):
        with patch.object(bridge, "load_upstream", return_value=self.fake_module()): self.call("submit", self.request)
        output = self.root / self.key / "output.png"
        other = self.inputs / "other.png"
        other.write_bytes(PNG)
        output.unlink()
        output.symlink_to(other)
        with self.assertRaises(ValueError): self.call("artifact")

if __name__ == "__main__": unittest.main()
