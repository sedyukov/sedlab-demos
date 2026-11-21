#!/usr/bin/env python3
"""Generate the demo samples from tools/samples_src.json (macOS only: `say` + ffmpeg).

    python3 tools/make_samples.py

For every sample → samples/<id>/input.<mp3|mp4|ogg>, samples/<id>/reply.mp3, samples/<id>/result.json.
The transcript/segments come from the script itself (exact timings measured from the generated audio);
summary, tasks and reply are curated in samples_src.json. Voices are synthetic, no personal data.
"""
from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "tools" / "samples_src.json"
OUT = ROOT / "samples"
RATE = 16000
GAP = 0.45  # seconds of silence between speakers


def run(*args: str) -> None:
    subprocess.run(args, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def duration(p: Path) -> float:
    out = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(p)],
                         check=True, capture_output=True, text=True).stdout
    return float(out.strip())


def speak(voice: str, text: str, pitch: float | None, dst: Path, tmp: Path) -> None:
    aiff = tmp / (dst.stem + ".aiff")
    run("say", "-v", voice, "-o", str(aiff), text)
    af = [f"aresample={RATE}"]
    if pitch:  # lower the pitch, keep the tempo: a second "speaker" from the same voice
        af = [f"asetrate=22050*{pitch}", f"atempo={1 / pitch:.4f}", f"aresample={RATE}"]
    run("ffmpeg", "-y", "-i", str(aiff), "-af", ",".join(af), "-ac", "1", "-ar", str(RATE), str(dst))


def build(s: dict, tmp: Path, order: int) -> None:
    sid = s["id"]
    d = OUT / sid
    d.mkdir(parents=True, exist_ok=True)
    parts, segments, t = [], [], 0.0
    silence = tmp / "gap.wav"
    run("ffmpeg", "-y", "-f", "lavfi", "-i", f"anullsrc=r={RATE}:cl=mono", "-t", str(GAP), str(silence))
    for i, line in enumerate(s["lines"]):
        w = tmp / f"{sid}-{i}.wav"
        speak(line["voice"], line["text"], line.get("pitch"), w, tmp)
        if i:
            parts.append(silence)
            t += GAP
        dur = duration(w)
        segments.append({"start": round(t, 2), "end": round(t + dur, 2), "text": line["text"]})
        parts.append(w)
        t += dur
    lst = tmp / f"{sid}.txt"
    lst.write_text("".join(f"file '{p}'\n" for p in parts))
    full = tmp / f"{sid}.wav"
    run("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", str(lst), "-c", "copy", str(full))

    for old in d.glob("input.*"):
        old.unlink()
    c = s["container"]
    if c == "mp3":
        inp = d / "input.mp3"
        run("ffmpeg", "-y", "-i", str(full), "-c:a", "libmp3lame", "-b:a", "48k", str(inp))
    elif c == "ogg":  # like a Telegram voice message
        inp = d / "input.ogg"
        run("ffmpeg", "-y", "-i", str(full), "-c:a", "libopus", "-b:a", "24k", str(inp))
    elif c == "mp4":  # a "screen recording": waveform video + AAC audio
        inp = d / "input.mp4"
        run("ffmpeg", "-y", "-i", str(full), "-filter_complex",
            "[0:a]showwaves=s=640x240:mode=cline:colors=0x6366f1|0x22d3ee,format=yuv420p[v]",
            "-map", "[v]", "-map", "0:a", "-c:v", "libx264", "-preset", "veryfast", "-crf", "32", "-r", "15",
            "-c:a", "aac", "-b:a", "48k", "-shortest", str(inp))
    else:
        raise SystemExit(f"unknown container {c}")

    # OGG/Opus copy for live STT (SpeechKit takes OGG/WAV/MP3, not MP4): same audio as input.*
    run("ffmpeg", "-y", "-i", str(full), "-c:a", "libopus", "-b:a", "24k", str(d / "stt.ogg"))

    reply_aiff = tmp / f"{sid}-reply.aiff"
    run("say", "-v", s["reply_voice"], "-o", str(reply_aiff), s["reply"])
    run("ffmpeg", "-y", "-i", str(reply_aiff), "-ac", "1", "-c:a", "libmp3lame", "-b:a", "48k", str(d / "reply.mp3"))

    result = {
        "id": sid,
        "order": order,
        "lang": s["lang"],
        "kind": s["kind"],
        "title": s["title"],
        "file": inp.name,
        "duration_sec": round(duration(inp), 1),
        "transcript": {"text": " ".join(x["text"] for x in s["lines"]), "language": s["lang"],
                       "segments": segments},
        "summary": s["summary"],
        "tasks": s["tasks"],
        "reply": {"text": s["reply"], "audio": "reply.mp3"},
    }
    (d / "result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(f"{sid}: {inp.name} {result['duration_sec']}s, {inp.stat().st_size // 1024} KiB")


def main() -> None:
    for tool in ("say", "ffmpeg", "ffprobe"):
        if not shutil.which(tool):
            raise SystemExit(f"{tool} not found (macOS + brew install ffmpeg)")
    src = json.loads(SRC.read_text())
    with tempfile.TemporaryDirectory() as tmp:
        for i, s in enumerate(src["samples"]):
            build(s, Path(tmp), i)


if __name__ == "__main__":
    main()
