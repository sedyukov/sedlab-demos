"""Optional live Telegram bot (long polling in a background thread), enabled by VOICE_TELEGRAM_TOKEN.

Voice / audio / round video / audio document → transcript, summary, tasks as text + the reply as a voice message.
Text messages → summary and tasks. Same pipeline and limits as the web demo. All Telegram I/O is telegram-core.
"""
from __future__ import annotations

import html
import logging
import threading
from typing import Any

from sedlab_ai import ModuleError
from sedlab_telegram_core import Bot, Update
from sedlab_telegram_core.api import HTTPAPI

from . import pipeline
from .limits import Limits
from .providers import Providers, default_llm
from .settings import Settings

log = logging.getLogger("voice.tg")
MEDIA_KINDS = ("voice", "audio", "video_note", "document")
DEFAULT_NAMES = {"voice": "voice.ogg", "audio": "audio.mp3", "video_note": "video.mp4", "document": "file.bin"}

TEXT = {
    "start": {
        "en": "Send me a voice message, an audio file or a round video. I'll transcribe it, summarize it, list the "
              "action items (who / what / when) and answer with a voice message. Text works too.",
        "ru": "Пришлите голосовое, аудиофайл или кружок: расшифрую, сделаю саммари, выпишу задачи (кто / что / срок) "
              "и отвечу голосом. Можно и текстом.",
    },
    "too_long": {"en": "Audio is limited to {n} s in this demo.", "ru": "В демо аудио не длиннее {n} с."},
    "too_big": {"en": "The file is too large for this demo ({n} MB max).", "ru": "Файл слишком большой для демо (до {n} МБ)."},
    "speechkit_fmt": {"en": "Please send a voice message or an mp3/ogg file: this format isn't supported by the "
                            "speech service.", "ru": "Пришлите, пожалуйста, голосовое или файл mp3/ogg: этот формат "
                                                     "сервис распознавания не принимает."},
    "unsupported": {"en": "Send a voice message, audio or video, please.", "ru": "Пришлите, пожалуйста, голосовое, аудио или видео."},
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
        lines.append("• " + e(x["title"]) + (f" <i>({e(meta)})</i>" if meta else ""))
    return "\n".join(lines)[:4000]


def speechkit_ok(f: Any) -> bool:
    """SpeechKit accepts OGG/Opus, WAV and MP3; round videos, m4a, webm etc. would need transcoding."""
    if f.kind == "voice":
        return True
    name = (f.file_name or "").lower()
    return f.mime_type in ("audio/ogg", "audio/opus", "audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav") or \
        name.endswith((".ogg", ".oga", ".opus", ".mp3", ".wav"))


def user_lang(u: Update) -> str:
    code = (((u.raw.get("message") or {}).get("from") or {}).get("language_code") or "")
    return "ru" if code.startswith(("ru", "uk", "be", "kk")) else "en"


class VoiceBot:
    def __init__(self, s: Settings, prov: Providers, limits: Limits, bot: Bot | None = None) -> None:
        self.s, self.prov, self.limits = s, prov, limits
        self.bot = bot or Bot(HTTPAPI(s.telegram_token), token=s.telegram_token)
        self.llm = s.environ.get("VOICE_TG_LLM") or default_llm(prov)
        self.max_bytes = s.max_upload_mb * 1024 * 1024
        self._stop = threading.Event()
        self.offset = 0

    # --- lifecycle -------------------------------------------------------------------------------------------
    def start(self) -> None:
        threading.Thread(target=self._loop, name="tg-poll", daemon=True).start()
        log.info("telegram bot polling started")

    def stop(self) -> None:
        self._stop.set()

    def _loop(self) -> None:
        while not self._stop.is_set():
            try:
                self.poll_once(timeout=25)
            except Exception as e:  # keep polling; never log tokens or content
                log.warning("poll error: %s", getattr(e, "code", type(e).__name__))
                self._stop.wait(5)

    def poll_once(self, timeout: int = 0) -> int:
        updates = self.bot.get_updates(offset=self.offset, timeout=timeout)
        for u in updates:
            self.offset = max(self.offset, u.update_id + 1)
            if not u.chat_id:
                continue
            try:
                self.handle(u)
            except Exception as e:
                log.warning("handle error: %s", getattr(e, "code", type(e).__name__))
                self._send(u.chat_id, t("error", user_lang(u)))
        return len(updates)

    # --- handling --------------------------------------------------------------------------------------------
    def _send(self, chat_id: int, text: str) -> None:
        self.bot.send_message(chat_id, text, parse_mode="HTML")

    def handle(self, u: Update) -> None:
        chat, lang = u.chat_id, user_lang(u)
        f = u.file
        if u.kind == "command":
            self._send(chat, t("start", lang))
            return
        if f and (f.kind not in MEDIA_KINDS or
                  (f.kind == "document" and not f.mime_type.startswith(("audio/", "video/")))):
            self._send(chat, t("unsupported", lang))
            return
        text = "" if f else u.text.strip()
        if not f and not text:
            return
        if f and f.duration > self.s.max_audio_sec:
            self._send(chat, t("too_long", lang, n=self.s.max_audio_sec))
            return
        if f and f.file_size > self.max_bytes:
            self._send(chat, t("too_big", lang, n=self.s.max_upload_mb))
            return
        if f and self.prov.speech == "yandex_speechkit" and not speechkit_ok(f):
            self._send(chat, t("speechkit_fmt", lang))
            return
        if f and not self.prov.stt_live:
            self._send(chat, t("demo", lang))
            return
        if self.limits.check(f"tg:{chat}"):
            self._send(chat, t("limit", lang))
            return
        self.bot.call("sendChatAction", {"chat_id": chat, "action": "typing"})
        client = self.prov.client(self.llm)
        transcript = ""
        if f:
            data = self.bot.download_file(f.file_id, max_bytes=self.max_bytes).content
            tr = client.transcribe(data, filename=f.file_name or DEFAULT_NAMES[f.kind])
            transcript = text = tr.text
        out_lang = pipeline.guess_lang(text, lang)
        r = pipeline.analyze(client, text, out_lang, live_llm=self.prov.llm_live(self.llm))
        self._send(chat, format_result(transcript, r, lang))
        if self.prov.tts_live:
            try:
                a = client.synthesize(r["reply"]["text"][:4000], format="opus")
                self.bot.send_voice(chat, a.bytes, filename="reply.ogg")
            except ModuleError as e:
                log.warning("tts error: %s", e.code.value)
