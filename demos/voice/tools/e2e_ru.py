"""End-to-end check of the RU edition as a visitor sees it (Playwright, headless Chromium, its own browser).

    uv run --with playwright python tools/e2e_ru.py [URL]      # default https://voice.demo.sedlab.dev

Checks: no developer words on screen, both samples, own upload behind the consent box (imitation is labelled),
specialist details folded, phone width 390 px without horizontal scroll, sign-in button visible, result scrolled into view.
Exit code 1 on the first failed check.
"""
from __future__ import annotations

import asyncio
import re
import struct
import sys
import tempfile
from pathlib import Path

from playwright.async_api import async_playwright

URL = sys.argv[1] if len(sys.argv) > 1 else "https://voice.demo.sedlab.dev"
JARGON = re.compile(r"\b(мок|mock|LLM|STT|TTS|live|API-ключ|English version)\b|—", re.I)
fails: list[str] = []


def check(ok: bool, what: str) -> None:
    print(("ok    " if ok else "FAIL  ") + what)
    if not ok:
        fails.append(what)


def wav(sec: int) -> bytes:
    n = 16000 * sec
    return (b"RIFF" + struct.pack("<I", 36 + n * 2) + b"WAVEfmt " + struct.pack("<IHHIIHH", 16, 1, 1, 16000, 32000, 2, 16)
            + b"data" + struct.pack("<I", n * 2) + b"\0" * (n * 2))


async def visible_text(pg) -> str:
    return await pg.evaluate("document.body.innerText")


async def desktop(b) -> None:
    pg = await b.new_page(viewport={"width": 1366, "height": 800})
    await pg.goto(URL)
    await pg.wait_for_selector(".sample")
    txt = await visible_text(pg)
    check(not JARGON.search(txt), f"home: no developer words ({(JARGON.search(txt) or [''])[0]!r})")
    check(await pg.locator("#llmBox").is_hidden(), "home: no model switch")
    check(await pg.locator(".sample").count() == 2, "home: two samples")
    for sid in ("planning-ru", "voice-note-ru"):
        await pg.click(f"[data-sample={sid}]")
        await pg.wait_for_function("document.querySelectorAll('#tasks tr td').length > 1 && !document.querySelector('#tasks .skeleton')", timeout=15000)
        await pg.wait_for_timeout(2500)
        chips = await pg.locator("#trMeta, #llmMeta, #ttsMeta").all_inner_texts()
        check(all("готовый пример" in c for c in chips), f"{sid}: every card says «готовый пример»")
        check(await pg.locator("#tasks tr").count() >= 2, f"{sid}: tasks listed")
        txt = await visible_text(pg)
        check(not JARGON.search(txt), f"{sid}: no developer words ({(JARGON.search(txt) or [''])[0]!r})")
    await pg.evaluate("scrollTo(0, 0)")
    await pg.click("[data-tab=upload]")
    check(await pg.locator("#lockHint").is_visible(), "upload: consent hint shown while unchecked")
    check(await pg.locator("#fileIn").is_disabled(), "upload: file input locked before consent")
    await pg.check("#consentCb")
    check(await pg.locator("#lockHint").is_hidden(), "upload: hint gone after consent")
    with tempfile.TemporaryDirectory() as d:
        f = Path(d) / "test.wav"
        f.write_bytes(wav(2))
        await pg.set_input_files("#fileIn", str(f))
        await pg.wait_for_function("document.querySelector('#summary').innerText.trim().length > 0 && !document.querySelector('#summary .skeleton')", timeout=20000)
        await pg.wait_for_timeout(2500)
    tr = await pg.locator("#segments").inner_text()
    chips = " ".join(await pg.locator("#trMeta, #llmMeta").all_inner_texts())
    note = await pg.locator("#notice").inner_text()
    live = "вживую" in chips
    if live:
        check(True, "upload: processed live")
    else:
        check(tr.strip().startswith("Это имитация"), "upload: imitation transcript is plain Russian")
        check(chips.count("имитация") >= 2, "upload: transcript and summary labelled «имитация»")
        check("Имитация" in note, "upload: notice says it is an imitation")
    await pg.locator("#techBox summary").click()
    items = await pg.locator("#techList li").count()
    check(items >= 5, "details for specialists open with the stack and current status")
    await pg.close()


async def phone(b) -> None:
    m = await b.new_page(viewport={"width": 390, "height": 844}, device_scale_factor=2, is_mobile=True, has_touch=True)
    await m.goto(URL)
    await m.wait_for_selector(".sample")
    over = await m.evaluate("document.documentElement.scrollWidth - innerWidth")
    check(over <= 0, f"phone: no horizontal scroll ({over}px)")
    check(await m.locator("#sl-account button, #sl-account a").first.is_visible(), "phone: sign-in button visible")
    await m.click("[data-sample=voice-note-ru]")
    await m.wait_for_timeout(1500)
    top = await m.evaluate("document.querySelector('#results').getBoundingClientRect().top")
    check(top < 844 * 0.75, f"phone: result scrolled into view (top {round(top)}px)")
    await m.close()


async def main() -> None:
    async with async_playwright() as p:
        b = await p.chromium.launch()
        await desktop(b)
        await phone(b)
        await b.close()
    print(f"\n{'FAILED: ' + str(len(fails)) if fails else 'all green'}  ({URL})")
    sys.exit(1 if fails else 0)


asyncio.run(main())
