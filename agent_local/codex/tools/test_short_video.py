"""Real FFmpeg integration checks; no network or generation providers required."""
import copy
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import short_video as video


@unittest.skipUnless(shutil.which("ffmpeg") and shutil.which("ffprobe"), "FFmpeg is required")
class VideoWorkflowTest(unittest.TestCase):
    def test_mixed_audio_render_and_timing_validation(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            for name, color, audio in (("presenter", "red", True), ("product", "green", False), ("usage", "blue", True)):
                args = ["ffmpeg", "-v", "error", "-f", "lavfi", "-i", f"color=c={color}:s=64x96:r=30:d=10"]
                if audio:
                    args += ["-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=10", "-shortest", "-c:a", "aac"]
                subprocess.run(args + ["-c:v", "libx264", "-pix_fmt", "yuv420p", str(root / f"{name}.mp4")], check=True)
            plan = {"version": 1, "max_generation_seconds": 10, "clips": [
                {"path": f"{name}.mp4", "role": name, "duration": 10, "semantic_beat": name}
                for name in ("presenter", "product", "usage")]}
            video.save(root / "edit.json", plan)
            video.render(root / "edit.json", root / "final.mp4", 64, 96)
            report = json.loads((root / "final.report.json").read_text())
            self.assertAlmostEqual(report["duration_seconds"], 30, delta=.15)
            info = video.probe(root / "final.mp4")
            self.assertTrue(any(s["codec_type"] == "audio" for s in info["streams"]))
            self.assertTrue(video.inspect(root / "final.mp4", root / "analysis", 15).is_file())
            with self.assertRaises(ValueError):
                video.render(root / "edit.json", root / "final.mp4", 64, 96)
            for name in ("too_long", "short_source", "missing_semantic_beat", "nan"):
                bad = copy.deepcopy(plan)
                clip = bad["clips"][0]
                if name == "too_long": clip["duration"] = 12
                elif name == "short_source": clip["start"] = 9
                elif name == "missing_semantic_beat": clip["semantic_beat"] = ""
                else: clip["duration"] = float("nan")
                video.save(root / "bad.json", bad)
                with self.subTest(name=name), self.assertRaises(ValueError):
                    video.load_plan(root / "bad.json")

    def test_workspace_is_repeatable_and_confines_product_names(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            base = video.init(root, "example")
            (base / "product.json").write_text('{"name":"preserved"}')
            video.init(root, "example")
            self.assertEqual(json.loads((base / "product.json").read_text())["name"], "preserved")
            self.assertTrue((root / "matrials" / "characters").is_dir())
            with self.assertRaises(ValueError):
                video.init(root, "../escape")


if __name__ == "__main__":
    unittest.main()
