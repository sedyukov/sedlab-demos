"""Optional live Telegram bot (long polling in a background thread), enabled by VOICE_TELEGRAM_TOKEN.

Voice / audio / video note → transcript, summary, tasks as text + the reply as a voice message.
Text messages → summary and tasks. Same pipeline and limits as the web demo.
Sending uses the telegram-core module; file download and multipart sendVoice are thin helpers here
(TODO: move into telegram-core as download_file / send_voice).
"""
from __future__ import annotations

import html
import logging
import threading
from typing import Any

import httpx

from sedlab_ai import ModuleError
from sedlab_telegram_core import Bot
from sedlab_telegram_core.api import HTTPAPI

from . import pipeline
from .limits import Limits
from .providers import Providers
from .settings import Settings

log = logging.getLogger("voice.tg")
API = "https://api.telegram.org"

TEXT = {
    "start": {
        "en": "Send me a voice message, an audio file or a round video — I'll transcribe it, summarize it, list the "
              "action items (who / what / when) and answer with a voice message. Text works too.",
        "ru": "Пришлите голосовое, аудиофайл или кружок — расшифрую, сделаю саммари, выпишу задачи (кто / что / срок) "
              "и отвечу голосом. Можно и текстом.",
    },
    "too_long": {"en": "Audio is limited to {n} s in this demo.", "ru": "В демо аудио — не длиннее {n} с."},
    "limit": {"en": "Demo limit reached, try again later.", "ru": "Лимит демо исчерпан, попробуйте позже."},
    "demo": {"en": "Live speech recognition is off in this demo build.", "ru": "Живое распознавание в этой сборке выключено."},
    "error": {"en": "Something went wrong, try again.", "ru": "Что-то пошло не так, попробуйте ещё раз."},
    "summary": {"en": "Summary", "ru": "Саммари"},
    "tasks": {"en": "Action items", "ru": "Задачи"},
    "none": {"en": "no action items", "ru": "задач нет"},
    "transcript": {"en": "Transcript", "ru": "Расшифровка"},
}


def t(key: str, lang: str, **kw: Any) -> str:
    return TEXT[key]["ru" if lang == "ru" else "en"].format(**kw)


def format_result(transcript: str, r: dict[str, Any], lang: str) -> str:
    e = html.escape
    lines = []
    if transcript:
        tr = transcript if len(transcript) < 1500 else transcript[:1500] + "…"
        lines += [f"<b>{t('transcript', lang)}</b>", f"<i>{e(tr)}</i>", ""]
    lines += [f"<b>{t('summary', lang)}</b>", e(r["summary"]), "", f"<b>{t('tasks', lang)}</b>"]
    if not r["tasks"]:
        lines.append(t("none", lang))
    for x in r["tasks"]:
        meta = " · ".join(v for v in (x.get("owner"), x.get("due")) if v)
        lines.append("• " + e(x["title"]) + (f" — <i>{e(meta)}</i>" if meta else ""))
    return "\n".join(lines)[:4000]


class VoiceBot:
    def __init__(self, s: Settings, prov: Providers, limits: Limits, http: httpx.Client | None = None,
                 bot: Bot | None = None, api_base: str = API) -> None:
        self.s, self.prov, self.limits = s, prov, limits
        self.base = api_base.rstrip("/")
        self.http = http or httpx.Client(timeout=60)
        self.bot = bot or Bot(HTTPAPI(s.telegram_token, api_base), token=s.telegram_token)
        self.llm = s.environ.get("VOICE_TG_LLM") or ("anthropic" if prov.anthropic else "openai")
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self.offset = 0

    # --- lifecycle -------------------------------------------------------------------------------------------
    def start(self) -> None:
        self._thread = threading.Thread(target=self._loop, name="tg-poll", daemon=True)
        self._thread.start()
        log.info("telegram bot polling started")

    def stop(self) -> None:
        self._stop.set()

    def _loop(self) -> None:
        while not self._stop.is_set():
            try:
                self.poll_once(timeout=25)
            except Exception as e:  # keep polling; never log tokens or content
                log.warning("poll error: %s", type(e).__name__)
                self._stop.wait(5)

    def poll_once(self, timeout: int = 0) -> int:
        updates = self.bot.call("getUpdates", {"offset": self.offset, "timeout": timeout,
                                               "allowed_updates": ["message"]}) or []
        for u in updates:
            self.offset = max(self.offset, u["update_id"] + 1)
            m = u.get("message")
            if m:
                try:
                    self.handle(m)
                except Exception as e:
                    log.warning("handle error: %s", type(e).__name__)
                    self._send(m["chat"]["id"], t("error", self._lang(m)))
        return len(updates)

    # --- handling --------------------------------------------------------------------------------------------
    @staticmethod
    def _lang(m: dict[str, Any]) -> str:
        return "ru" if ((m.get("from") or {}).get("language_code") or "").startswith(("ru", "uk", "be", "kk")) else "en"

    def _send(self, chat_id: int, text: str) -> None:
        self.bot.send_message(chat_id, text, parse_mode="HTML")

    def handle(self, m: dict[str, Any]) -> None:
        chat, lang = m["chat"]["id"], self._lang(m)
        text = (m.get("text") or "").strip()
        media = m.get("voice") or m.get("audio") or m.get("video_note") or m.get("video")
        if text.startswith("/"):
            self._send(chat, t("start", lang))
            return
        if not media and not text:
            return
        if media and (media.get("duration") or 0) > self.s.max_audio_sec:
            self._send(chat, t("too_long", lang, n=self.s.max_audio_sec))
            return
        if media and not self.prov.stt_live:
            self._send(chat, t("demo", lang))
            return
        if self.limits.check(f"tg:{m['chat']['id']}"):
            self._send(chat, t("limit", lang))
            return
        self.bot.call("sendChatAction", {"chat_id": chat, "action": "typing"})
        client = self.prov.client(self.llm)
        transcript = ""
        if media:
            data = self.download(media["file_id"])
            name = "voice.ogg" if m.get("voice") else ("video.mp4" if m.get("video_note") or m.get("video") else
                                                       media.get("file_name") or "audio.mp3")
            tr = client.transcribe(data, filename=name)
            transcript = tr.text
            text = tr.text
        out_lang = pipeline.guess_lang(text, lang)
        r = pipeline.analyze(client, text, out_lang, live_llm=self.prov.llm_live(self.llm))
        self._send(chat, format_result(transcript, r, lang))
        if self.prov.tts_live:
            try:
                a = client.synthesize(r["reply"]["text"][:4000], format="opus")
                self.send_voice(chat, a.bytes)
            except ModuleError as e:
                log.warning("tts error: %s", e.code.value)

    # --- file helpers (candidates for telegram-core) --------------------------------------------------------
    def download(self, file_id: str) -> bytes:
        f = self.bot.call("getFile", {"file_id": file_id})
        r = self.http.get(f"{self.base}/file/bot{self.s.telegram_token}/{f['file_path']}")
        r.raise_for_status()
        return r.content

    def send_voice(self, chat_id: int, ogg: bytes) -> None:
        r = self.http.post(f"{self.base}/bot{self.s.telegram_token}/sendVoice", data={"chat_id": str(chat_id)},
                           files={"voice": ("reply.ogg", ogg, "audio/ogg")})
        r.raise_for_status()
