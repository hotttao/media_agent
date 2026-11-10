#!/usr/bin/env python3
"""Local short-video workspace, reference evidence and deterministic FFmpeg edit.

No model calls here: Codex reviews evidence and writes the edit plan explicitly.
"""
import argparse
import json
import math
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1] / "short-video"
PRODUCT_DIRS = ("details", "closeup-images", "closeup-videos", "usage-videos",
                "references", "finals", "frames", "segments", "analysis", "plans")


def run(args, **kwargs):
    return subprocess.run([str(x) for x in args], check=True, capture_output=True,
                          text=True, timeout=1800, **kwargs).stdout


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def probe(path):
    result = json.loads(run(["ffprobe", "-v", "error", "-show_format", "-show_streams",
                             "-of", "json", Path(path).resolve()]))
    if not any(s["codec_type"] == "video" for s in result["streams"]):
        raise ValueError("reference/segment must contain a video stream")
    return result


def duration(info):
    value = float(info["format"]["duration"])
    if not math.isfinite(value) or value <= 0:
        raise ValueError("invalid media duration")
    return value


def init(root, product):
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}", product):
        raise ValueError("product ID must be 1-80 letters, digits, '-' or '_'")
    for folder in ("characters", "scenes", "props"):
        (root / "matrials" / folder).mkdir(parents=True, exist_ok=True)
    base = root / "products" / product
    for folder in PRODUCT_DIRS:
        (base / folder).mkdir(parents=True, exist_ok=True)
    if not (base / "product.json").exists():
        save(base / "product.json", {"id": product, "name": "", "buyer": "", "user": "",
            "verified_claims": [], "source_images": [], "missing": ["product_details", "product_images"]})
    return base


def inspect(video, output, interval=2):
    if not math.isfinite(interval) or interval <= 0:
        raise ValueError("sampling interval must be positive")
    info = probe(video)
    length = duration(info)
    output.mkdir(parents=True, exist_ok=True)
    # Limit evidence size. A sample is not a claim that the whole scene was understood.
    count = min(80, max(1, math.ceil(length / interval)))
    frames = []
    for i in range(count):
        second = min(length - .01, i * length / count)
        target = output / f"frame-{i:03d}.jpg"
        run(["ffmpeg", "-v", "error", "-y", "-ss", second, "-i", Path(video).resolve(),
             "-frames:v", "1", "-vf", "scale=640:-2", target.resolve()])
        frames.append({"timestamp_seconds": second, "path": str(target.resolve())})
    save(output / "evidence.json", {"video": str(Path(video).resolve()), "probe": info,
        "frames": frames, "review_status": "awaiting_codex_review",
        "audio_status": "not_transcribed", "note": "Uniform samples; review frames and speech before semantic segmentation."})
    return output / "evidence.json"


def download(url, output):
    if not url.startswith("https://"):
        raise ValueError("reference URL must use HTTPS")
    output.mkdir(parents=True, exist_ok=True)
    # The extractor owns site support and login requirements; never invent success.
    run(["yt-dlp", "--no-playlist", "--restrict-filenames", "--write-info-json",
         "--merge-output-format", "mp4", "-o", output.resolve() / "%(id)s.%(ext)s", "--", url])
    return {"status": "downloaded", "directory": str(output.resolve())}


def number(value, label, minimum=0):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or value < minimum:
        raise ValueError(f"{label} must be a finite number >= {minimum}")
    return value


def load_plan(path):
    plan = json.loads(path.read_text())
    if not isinstance(plan, dict) or plan.get("version") != 1:
        raise ValueError("plan version must be 1")
    maximum = number(plan.get("max_generation_seconds", 10), "max_generation_seconds", 4)
    if maximum > 15:
        raise ValueError("generation duration must match the configured Studio limit (4-15s)")
    clips = plan.get("clips")
    if not isinstance(clips, list) or not 1 <= len(clips) <= 30:
        raise ValueError("plan needs 1-30 clips")
    total = 0
    for clip in clips:
        if not isinstance(clip, dict):
            raise ValueError("each clip must be an object")
        seconds = number(clip.get("duration"), "clip duration", .1)
        start = number(clip.get("start", 0), "clip start")
        if clip.get("role") not in ("presenter", "product", "usage"):
            raise ValueError("clip role must be presenter, product or usage")
        if not clip.get("semantic_beat"):
            raise ValueError("each clip must name its complete semantic beat")
        if clip.get("generated", True) and seconds > maximum:
            raise ValueError("generated clip exceeds configured duration; rewrite the semantic beat")
        source = (path.parent / clip["path"]).resolve()
        info = probe(source)
        if start + seconds > duration(info) + .04:
            raise ValueError(f"clip is shorter than its edit interval: {source.name}")
        clip["source"] = source
        clip["has_audio"] = any(s["codec_type"] == "audio" for s in info["streams"])
        total += seconds
    if not 27 <= total <= 33:
        raise ValueError(f"final duration must be around 30s (27-33), got {total}")
    for name in ("voiceover", "music", "captions"):
        if plan.get(name):
            source = (path.parent / plan[name]).resolve()
            if not source.is_file():
                raise ValueError(f"missing {name} file")
            plan[name] = source
    return plan, total


def render(plan_path, output, width=1080, height=1920):
    if width < 64 or height < 64 or width % 2 or height % 2:
        raise ValueError("output dimensions must be positive even integers >=64")
    plan, total = load_plan(plan_path)
    output = output.resolve()
    if output.exists():
        raise ValueError("output already exists; choose a new version name")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="short-video-", dir=output.parent) as folder:
        work = Path(folder)
        for i, clip in enumerate(plan["clips"]):
            seconds = clip["duration"]
            args = ["ffmpeg", "-v", "error", "-n", "-ss", clip.get("start", 0), "-i", clip["source"]]
            if not clip["has_audio"]:
                args += ["-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo"]
            vf = f"scale={width}:{height}:force_original_aspect_ratio=decrease,pad={width}:{height}:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30"
            args += ["-t", seconds, "-map", "0:v:0", "-map", "0:a:0" if clip["has_audio"] else "1:a:0",
                     "-vf", vf, "-af", "aresample=48000,apad", "-c:v", "libx264", "-preset", "fast",
                     "-pix_fmt", "yuv420p", "-c:a", "aac", "-ar", "48000", "-ac", "2", work / f"clip-{i}.mp4"]
            run(args)
        (work / "concat.txt").write_text("".join(f"file 'clip-{i}.mp4'\n" for i in range(len(plan["clips"]))))
        run(["ffmpeg", "-v", "error", "-n", "-f", "concat", "-safe", "1", "-i", "concat.txt",
             "-c", "copy", "joined.mp4"], cwd=work)
        args = ["ffmpeg", "-v", "error", "-n", "-i", "joined.mp4"]
        audio_index = 0
        if plan.get("voiceover"):
            args += ["-i", plan["voiceover"]]
            audio_index = 1
        filters = [f"[{audio_index}:a]apad,atrim=duration={total},asetpts=PTS-STARTPTS[voice]"]
        if plan.get("music"):
            music_index = 2 if plan.get("voiceover") else 1
            args += ["-stream_loop", "-1", "-i", plan["music"]]
            filters += [f"[{music_index}:a]volume=0.12,atrim=duration={total}[music]",
                        "[voice][music]amix=inputs=2:duration=first:normalize=0[audio]"]
        else:
            filters += ["[voice]anull[audio]"]
        args += ["-filter_complex", ";".join(filters), "-map", "0:v:0", "-map", "[audio]", "-t", total]
        if plan.get("captions"):
            shutil.copyfile(plan["captions"], work / "captions.srt")
            args += ["-vf", "subtitles=captions.srt"]
        args += ["-c:v", "libx264", "-preset", "fast", "-pix_fmt", "yuv420p", "-c:a", "aac",
                 "-movflags", "+faststart", "final.mp4"]
        run(args, cwd=work)
        actual = duration(probe(work / "final.mp4"))
        if abs(actual - total) > .15:
            raise ValueError(f"unexpected rendered duration {actual}, expected {total}")
        # Publish only a fully rendered and verified local file.
        (work / "final.mp4").replace(output)
    save(output.with_suffix(".report.json"), {"plan": str(plan_path.resolve()), "duration_seconds": actual,
         "width": width, "height": height, "clips": len(plan["clips"]), "status": "rendered_awaiting_visual_review"})
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("doctor")
    init_cmd = commands.add_parser("init"); init_cmd.add_argument("product")
    inspect_cmd = commands.add_parser("inspect"); inspect_cmd.add_argument("video", type=Path); inspect_cmd.add_argument("output", type=Path)
    download_cmd = commands.add_parser("download"); download_cmd.add_argument("url"); download_cmd.add_argument("output", type=Path)
    for name in ("validate", "render"):
        cmd = commands.add_parser(name); cmd.add_argument("plan", type=Path)
        if name == "render":
            cmd.add_argument("output", type=Path); cmd.add_argument("--width", type=int, default=1080); cmd.add_argument("--height", type=int, default=1920)
    args = parser.parse_args()
    try:
        if args.command == "doctor":
            result = {x: shutil.which(x) for x in ("ffmpeg", "ffprobe", "yt-dlp", "scenedetect", "codex")}
        elif args.command == "init": result = str(init(args.root, args.product))
        elif args.command == "inspect": result = str(inspect(args.video, args.output))
        elif args.command == "download": result = download(args.url, args.output)
        elif args.command == "validate": result = {"valid": True, "duration_seconds": load_plan(args.plan)[1]}
        else: result = str(render(args.plan, args.output, args.width, args.height))
        print(json.dumps(result, ensure_ascii=False))
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        # Tool stderr can contain signed download URLs. Keep it out of the chat.
        parser.exit(1, f"{type(error).__name__}: {error if not isinstance(error, subprocess.SubprocessError) else 'media command failed; check local tools, source and plan'}\n")


if __name__ == "__main__":
    main()
