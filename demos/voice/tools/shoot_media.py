"""Screenshots + demo video for the README / portfolio (Playwright, Chromium).

    uv run --with playwright python tools/shoot_media.py [BASE_URL]   # default http://localhost:8791

Writes media/*.png, media/demo.mp4 and media/demo.gif (needs ffmpeg).
"""
from __future__ import annotations

import asyncio
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from playwright.async_api import async_playwright

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8791"
OUT = Path(__file__).resolve().parent.parent / "media"


async def scroll_to(pg, sel: str, block: str = "start") -> None:
    await pg.eval_on_selector(sel, f"e => e.scrollIntoView({{behavior: 'smooth', block: '{block}'}})")
    await pg.wait_for_timeout(900)


async def shots(p) -> None:
    b = await p.chromium.launch()
    pg = await b.new_page(viewport={"width": 1280, "height": 860}, device_scale_factor=2)
    await pg.goto(f"{BASE}/?lang=en")
    await pg.wait_for_selector(".sample")
    await pg.screenshot(path=OUT / "01-home-en.png")
    await pg.click("[data-sample=standup-en]")
    await pg.wait_for_timeout(3500)
    await pg.screenshot(path=OUT / "02-meeting-result-en.png", full_page=True)

    ru = await b.new_page(viewport={"width": 1280, "height": 860}, device_scale_factor=2, color_scheme="dark")
    await ru.goto(f"{BASE}/?lang=ru&sample=planning-ru")
    await ru.wait_for_timeout(3800)
    await scroll_to(ru, "#results")
    await ru.screenshot(path=OUT / "03-video-call-ru-dark.png")

    await pg.click("[data-tab=record]")
    await pg.click("[data-lang=ru]")
    await scroll_to(pg, ".input", "center")
    await pg.screenshot(path=OUT / "04-record-ru.png")

    m = await b.new_page(viewport={"width": 390, "height": 844}, device_scale_factor=3, is_mobile=True)
    await m.goto(f"{BASE}/?lang=ru&sample=voice-note-ru")
    await m.wait_for_timeout(3800)
    await scroll_to(m, "#cardTasks", "center")
    await m.screenshot(path=OUT / "05-mobile-voice-note-ru.png")
    await b.close()


async def video(p, tmp: Path) -> Path:
    b = await p.chromium.launch()
    ctx = await b.new_context(viewport={"width": 1280, "height": 800}, record_video_dir=str(tmp),
                              record_video_size={"width": 1280, "height": 800})
    pg = await ctx.new_page()
    await pg.goto(f"{BASE}/?lang=en")
    await pg.wait_for_selector(".sample")
    await pg.wait_for_timeout(1800)
    for name in ("anthropic", "gigachat", "yandexgpt", "openai"):  # the LLM switch
        await pg.click(f"[data-llm={name}]")
        await pg.wait_for_timeout(450)
    await pg.click("[data-sample=standup-en]")
    await pg.wait_for_timeout(1000)
    await scroll_to(pg, "#results")
    await pg.wait_for_timeout(2600)
    await pg.click(".seg-line[data-i='1']")  # click a segment → audio jumps there
    await pg.wait_for_timeout(3500)
    await pg.eval_on_selector("#player", "a => a.pause()")
    await scroll_to(pg, "#cardReply", "center")
    await pg.wait_for_timeout(2000)
    await pg.evaluate("window.scrollTo({top: 0, behavior: 'smooth'})")
    await pg.wait_for_timeout(900)
    await pg.click("[data-lang=ru]")
    await pg.wait_for_timeout(900)
    await pg.click("[data-sample=planning-ru]")
    await pg.wait_for_timeout(900)
    await scroll_to(pg, "#results")
    await pg.wait_for_timeout(3200)
    await scroll_to(pg, "#cardTasks", "center")
    await pg.wait_for_timeout(2500)
    await pg.evaluate("window.scrollTo({top: 0, behavior: 'smooth'})")
    await pg.wait_for_timeout(900)
    await pg.click("[data-sample=voice-note-ru]")
    await pg.wait_for_timeout(900)
    await scroll_to(pg, "#results")
    await pg.wait_for_timeout(3500)
    await pg.evaluate("window.scrollTo({top: 0, behavior: 'smooth'})")
    await pg.wait_for_timeout(800)
    await pg.click("[data-tab=upload]")
    await pg.wait_for_timeout(1500)
    await pg.click("[data-tab=record]")
    await pg.wait_for_timeout(2000)
    v = pg.video
    await ctx.close()
    await b.close()
    return Path(await v.path())


def ff(*args: str) -> None:
    subprocess.run(["ffmpeg", "-y", "-loglevel", "error", *args], check=True)


async def main() -> None:
    OUT.mkdir(exist_ok=True)
    async with async_playwright() as p:
        await shots(p)
        with tempfile.TemporaryDirectory() as tmp:
            webm = await video(p, Path(tmp))
            if shutil.which("ffmpeg"):
                ff("-i", str(webm), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "26", "-preset", "slow",
                   "-movflags", "+faststart", "-an", str(OUT / "demo.mp4"))
                ff("-i", str(webm), "-vf", "fps=7,scale=720:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle",
                   str(OUT / "demo.gif"))
            else:
                shutil.copy(webm, OUT / "demo.webm")
    for f in sorted(OUT.iterdir()):
        print(f"{f.name}: {f.stat().st_size // 1024} KiB")


asyncio.run(main())
