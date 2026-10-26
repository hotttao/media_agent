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
        module.WEB_TURN_SEL = "[data-turn-key]"
        module.CHATGPT_WEB_SESSION = "chatgpt-web"
        module._find_agent_browser = lambda: "chrome-use"
        module._relay_connected = lambda browser: True
        module._wait_composer = lambda *args: True
        module._ab = lambda *args, **kwargs: None
        module._chatgpt_web_turn = lambda *args: contextlib.nullcontext()
        module._ab_eval = lambda *args, **kwargs: {"conversation": "https://chatgpt.com/c/12345678-abcd", "turns": ["owned-turn"], "texts": ["prompt"], "unknown": False}
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
        def recover(argv):
            self.assertEqual(argv[0], "https://chatgpt.com/c/12345678-abcd")
            self.assertIn("--last", argv)
            self.assertNotIn("--backend", argv)
            Path(argv[argv.index("--out") + 1]).write_bytes(PNG)
            Path(argv[argv.index("--out") + 1]).chmod(0o600)
            return 0
        module._recover_command = recover
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

    def test_artifact_cannot_be_a_symlink(self):
        with patch.object(bridge, "load_upstream", return_value=self.fake_module()): self.call("submit", self.request)
        output = self.root / self.key / "output.png"
        other = self.inputs / "other.png"
        other.write_bytes(PNG)
        output.unlink()
        output.symlink_to(other)
        with self.assertRaises(ValueError): self.call("artifact")

if __name__ == "__main__": unittest.main()
