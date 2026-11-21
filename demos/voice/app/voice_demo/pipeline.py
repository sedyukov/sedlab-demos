"""Glue: transcript → summary → tasks → spoken reply. All model calls go through the ai module Client."""
from __future__ import annotations

import base64
import re
from dataclasses import asdict
from typing import Any

from sedlab_ai import Client, Task

REPLY_PROMPT = (
    "You are a voice assistant. Write a short spoken reply (at most 3 sentences, plain text, no markdown, "
    "no lists) that recaps the key point and the action items with owners and deadlines. "
    "Write it in this language: {lang}."
)
LANG_NAMES = {"ru": "Russian", "en": "English"}


def guess_lang(text: str, fallback: str = "en") -> str:
    cyr = len(re.findall(r"[а-яА-ЯёЁ]", text or ""))
    lat = len(re.findall(r"[a-zA-Z]", text or ""))
    if not cyr and not lat:
        return fallback
    return "ru" if cyr >= lat else "en"


def _task_phrase(t: Task, lang: str) -> str:
    s = t.title
    if t.owner:
        s = f"{t.owner}: {s}"
    if t.due:
        s += (" — до " if lang == "ru" else " — by ") + t.due
    return s


def template_reply(summary: str, tasks: list[Task], lang: str) -> str:
    """Key-free reply used when the LLM is the mock."""
    if lang == "ru":
        head = f"Коротко: {summary}" if summary else "Сообщение получил."
        if not tasks:
            return head + " Задач не нашёл."
        return head + f" Задач: {len(tasks)}. " + "; ".join(_task_phrase(t, lang) for t in tasks) + "."
    head = f"In short: {summary}" if summary else "Got your message."
    if not tasks:
        return head + " No action items found."
    return head + f" Action items: {len(tasks)}. " + "; ".join(_task_phrase(t, lang) for t in tasks) + "."


def analyze(client: Client, text: str, lang: str, *, live_llm: bool) -> dict[str, Any]:
    s = client.summarize(text, language=LANG_NAMES.get(lang))
    t = client.extract_tasks(text)
    if live_llm:
        brief = s.text + "\n\nAction items:\n" + "\n".join("- " + _task_phrase(x, lang) for x in t.tasks)
        r = client.chat([{"role": "system", "content": REPLY_PROMPT.format(lang=LANG_NAMES.get(lang, lang))},
                         {"role": "user", "content": brief}], max_tokens=300)
        reply = r.text.strip()
    else:
        reply = template_reply(s.text, t.tasks, lang)
    usage = {"input_tokens": s.usage.input_tokens + t.usage.input_tokens,
             "output_tokens": s.usage.output_tokens + t.usage.output_tokens}
    return {
        "summary": s.text,
        "tasks": [asdict(x) for x in t.tasks],
        "reply": {"text": reply},
        "llm": {"provider": s.provider, "model": s.model, "usage": usage},
    }


def speak(client: Client, text: str) -> dict[str, str]:
    """TTS → {"audio": data URL, "provider": ...}. Caller decides whether TTS is live."""
    a = client.synthesize(text[:4000], format="mp3")
    return {"audio": f"data:{a.mime};base64," + base64.b64encode(a.bytes).decode(), "provider": a.provider}
