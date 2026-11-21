"""Screenshots + demo videos for README / portfolio, one set per edition (Playwright, Chromium).

    EDITION=intl uvicorn voice_demo.main:app --port 8791 &
    EDITION=ru   uvicorn voice_demo.main:app --port 8792 &
    uv run --with playwright python tools/shoot_media.py [EN_URL] [RU_URL]

Writes media/en-*.png, media/ru-*.png, media/demo-en.{mp4,gif}, media/demo-ru.{mp4,gif} (needs ffmpeg).
"""
from __future__ import annotations

import asyncio
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from playwright.async_api import async_playwright

EN = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8791"
RU = sys.argv[2] if len(sys.argv) > 2 else "http://localhost:8792"
OUT = Path(__file__).resolve().parent.parent / "media"
GIF = ("fps=7,scale=720:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];"
       "[b][p]paletteuse=dither=none:diff_mode=rectangle")


async def scroll_to(pg, sel: str, block: str = "start") -> None:
    await pg.eval_on_selector(sel, f"e => e.scrollIntoView({{behavior: 'smooth', block: '{block}'}})")
    await pg.wait_for_timeout(900)


async def shots(b, base: str, tag: str, meeting: str, video_call: str, voice_note: str) -> None:
    pg = await b.new_page(viewport={"width": 1280, "height": 860}, device_scale_factor=2)
    await pg.goto(base)
    await pg.wait_for_selector(".sample")
    await pg.screenshot(path=OUT / f"{tag}-1-home.png")
    await pg.click(f"[data-sample={meeting}]")
    await pg.wait_for_timeout(3500)
    await pg.screenshot(path=OUT / f"{tag}-2-meeting-result.png", full_page=True)

    dark = await b.new_page(viewport={"width": 1280, "height": 860}, device_scale_factor=2, color_scheme="dark")
    await dark.goto(f"{base}/?sample={video_call}")
    await dark.wait_for_timeout(3800)
    await scroll_to(dark, "#results")
    await dark.screenshot(path=OUT / f"{tag}-3-video-call-dark.png")

    m = await b.new_page(viewport={"width": 390, "height": 844}, device_scale_factor=3, is_mobile=True)
    await m.goto(f"{base}/?sample={voice_note}")
    await m.wait_for_timeout(3800)
    await scroll_to(m, "#cardTasks", "center")
    await m.screenshot(path=OUT / f"{tag}-4-mobile-voice-note.png")
    for p in (pg, dark, m):
        await p.close()


async def video(b, base: str, samples: list[str], tmp: Path) -> Path:
    ctx = await b.new_context(viewport={"width": 1280, "height": 800}, record_video_dir=str(tmp),
                              record_video_size={"width": 1280, "height": 800})
    pg = await ctx.new_page()
    await pg.goto(base)
    await pg.wait_for_selector(".sample")
    await pg.wait_for_timeout(1500)
    n = await pg.locator("#llmSeg button").count()
    for i in [*range(1, n), 0]:  # the LLM switch (buttons re-render on click → locate by index)
        await pg.locator("#llmSeg button").nth(i).click()
        await pg.wait_for_timeout(450)
    first = True
    for sid in samples:
        await pg.evaluate("window.scrollTo({top: 0, behavior: 'smooth'})")
        await pg.wait_for_timeout(800)
        await pg.click(f"[data-sample={sid}]")
        await pg.wait_for_timeout(900)
        await scroll_to(pg, "#results")
        await pg.wait_for_timeout(2600)
        if first:  # click a phrase → playback jumps there
            await pg.click(".seg-line[data-i='1']")
            await pg.wait_for_timeout(3200)
            await pg.eval_on_selector("#player", "a => a.pause()")
            first = False
        await scroll_to(pg, "#cardTasks", "center")
        await pg.wait_for_timeout(2000 if len(samples) > 2 else 4500)  # fewer samples → linger, keep ≥ 30 s
    await pg.evaluate("window.scrollTo({top: 0, behavior: 'smooth'})")
    await pg.wait_for_timeout(800)
    await pg.click("[data-tab=upload]")
    await pg.wait_for_timeout(1300)
    await pg.click("[data-tab=record]")
    await pg.wait_for_timeout(1800)
    v = pg.video
    await ctx.close()
    return Path(await v.path())


def ff(*args: str) -> None:
    subprocess.run(["ffmpeg", "-y", "-loglevel", "error", *args], check=True)


async def main() -> None:
    OUT.mkdir(exist_ok=True)
    editions = [("en", EN, "standup-en", "client-call-en", "voice-note-en"),
                ("ru", RU, "planning-ru", "planning-ru", "voice-note-ru")]
    async with async_playwright() as p:
        b = await p.chromium.launch()
        for tag, base, meeting, call, note in editions:
            await shots(b, base, tag, meeting, call, note)
            with tempfile.TemporaryDirectory() as tmp:
                webm = await video(b, base, list(dict.fromkeys([meeting, call, note])), Path(tmp))
                if shutil.which("ffmpeg"):
                    ff("-i", str(webm), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "26", "-preset", "slow",
                       "-movflags", "+faststart", "-an", str(OUT / f"demo-{tag}.mp4"))
                    ff("-i", str(OUT / f"demo-{tag}.mp4"), "-vf", GIF, str(OUT / f"demo-{tag}.gif"))
                else:
                    shutil.copy(webm, OUT / f"demo-{tag}.webm")
        await b.close()
    for f in sorted(OUT.iterdir()):
        print(f"{f.name}: {f.stat().st_size // 1024} KiB")


asyncio.run(main())
