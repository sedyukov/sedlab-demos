"""voice demo, FastAPI app: upload or record audio → transcript → summary → tasks → spoken reply.

Without keys everything runs on the ai module's mock + pre-computed samples; with AI_OPENAI_API_KEY /
AI_ANTHROPIC_API_KEY the same endpoints go live behind per-IP and daily limits.
"""
from __future__ import annotations

import json
import logging
import re
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

from fastapi import FastAPI, File, Form, Request, UploadFile
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from sedlab_ai import Client as AIClient
from sedlab_ai import Code, ModuleError
from sedlab_crm import ModuleError as CrmError

from . import pipeline
from .account import LLM_RESOURCE, STT_RESOURCE, TTS_RESOURCE, Accounts, Gate, stt_units
from .crm_export import CrmExport
from .limits import Limits, wav_duration
from .providers import Providers, default_llm
from .settings import Settings

log = logging.getLogger("voice")

STATUS = {Code.INVALID_ARGUMENT: 400, Code.RATE_LIMITED: 429, Code.NOT_IMPLEMENTED: 501,
          Code.UNAUTHENTICATED: 502, Code.CONFIG: 500}


CONSENT_VERSION = "voice-v1"  # legal: /consent/voice + terms 3a; sent by the page with every own recording (RU)

RU_MOCK_TRANSCRIPT = ("Это имитация: живое распознавание для этой записи не запускалось, поэтому здесь не текст вашей записи. "
                      "Нажмите «Править текст», впишите свою фразу, например: «Анна пришлёт отчёт до пятницы», "
                      "и нажмите «Пересчитать».")

RU_META = [
    ('<html lang="en">', '<html lang="ru">'),
    ('hreflang="ru"', 'hreflang="en"'),
    ("<title>Voice AI Assistant</title>", "<title>Голосовой ИИ-ассистент</title>"),
    ('content="Demo: transcribe voice messages and calls, get a summary, action items (who / what / when) and a spoken '
     'reply. Switch LLM: OpenAI, Anthropic or a local model."',
     'content="Демо: голосовые и созвоны в текст, кратко о главном, задачи (кто, что, к какому сроку) и ответ голосом. '
     'Российские сервисы распознавания и нейросети."'),
    ('content="Voice AI Assistant: demo"', 'content="Голосовой ИИ-ассистент: демо"'),
    ('content="Voice messages and calls → transcript → summary → action items → voice reply."',
     'content="Голосовые и созвоны: текст, кратко, задачи и ответ голосом."'),
]


class ApiError(Exception):
    def __init__(self, status: int, code: str, message: str = "") -> None:
        self.status, self.code, self.message = status, code, message


class AnalyzeIn(BaseModel):
    text: str = Field(min_length=1, max_length=20000)
    llm: str = ""
    lang: str = ""


class CrmIn(BaseModel):
    sample: str = Field(min_length=1, max_length=64)


class ConsentIn(BaseModel):
    version: str = Field(min_length=1, max_length=32)


class SpeakIn(BaseModel):
    text: str = Field(min_length=1, max_length=4000)


def load_samples(d: Path) -> dict[str, dict[str, Any]]:
    rs = [json.loads(f.read_text()) for f in d.glob("*/result.json")]
    return {r["id"]: r for r in sorted(rs, key=lambda r: (r.get("order", 99), r["id"]))}


def create_app(settings: Settings | None = None) -> FastAPI:
    s = settings or Settings.from_env()
    prov = Providers(s.environ, s.edition, s.live_allowed)
    limits = Limits(s.ip_per_hour, s.daily_live)
    accounts = Accounts(s.environ, enabled=s.edition == "ru")  # RU: live only for signed-in users, limits on auth
    mock = AIClient.mock()
    crm = CrmExport(s.environ, s.edition)
    samples = {k: r for k, r in load_samples(s.samples_dir).items() if r["lang"] == s.lang}
    max_bytes = s.max_upload_mb * 1024 * 1024

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        bot = None
        if s.telegram_token:
            from .tgbot import VoiceBot
            # RU: Telegram users carry no sign-in token, so the bot never spends live limits (mock only)
            bot_prov = prov if s.edition != "ru" else Providers(s.environ, s.edition, live_allowed=False)
            bot = VoiceBot(s, bot_prov, limits)
            bot.start()
            app.state.tg_username = bot.username
        yield
        if bot:
            bot.stop()

    app = FastAPI(title="voice demo", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
    app.state.settings, app.state.providers, app.state.limits, app.state.accounts = s, prov, limits, accounts
    app.state.crm = crm

    @app.exception_handler(ApiError)
    async def _api_error(_: Request, e: ApiError):
        return JSONResponse({"error": e.code, "message": e.message}, status_code=e.status)

    @app.exception_handler(ModuleError)
    async def _module_error(_: Request, e: ModuleError):
        log.warning("module error: %s (%s)", e.code.value, e.provider)  # never the request content
        msg = e.message if e.code == Code.INVALID_ARGUMENT else ""
        return JSONResponse({"error": e.code.value, "message": msg, "provider": e.provider},
                            status_code=STATUS.get(e.code, 502))

    def client_ip(req: Request) -> str:
        if s.trust_proxy:
            xff = req.headers.get("x-forwarded-for", "")
            if xff:  # Caddy appends the peer it saw: the last hop is the trustworthy one
                return xff.split(",")[-1].strip()
        return req.client.host if req.client else "?"

    def spend(req: Request) -> None:
        why = limits.check(client_ip(req))
        if why == "ip":
            raise ApiError(429, "limit_ip", f"Live mode allows {s.ip_per_hour} runs per hour per visitor.")
        if why == "daily":
            raise ApiError(429, "limit_daily", "The daily live budget is used up. Try the samples or come back tomorrow.")

    def gate_for(req: Request, live: bool) -> Gate | None:
        """RU: accounts gate (limits per user on auth). Otherwise the local per-IP / daily limiter."""
        g = accounts.gate(req.headers)
        if g is None and live:
            spend(req)
        return g

    def run(g: Gate | None, resource: str, units: int, fn):
        """Live call: under the user's accounts limit (RU) or directly (limits already spent). None → serve the mock."""
        return fn() if g is None else g.call(resource, units, fn)

    def check_llm(name: str) -> str:
        name = name or default_llm(prov)
        if name not in prov.llms:
            raise ApiError(400, "invalid_argument", f"llm must be one of {', '.join(prov.llms)}")
        return name

    @app.get("/health")
    def health():
        return {"ok": True}

    @app.get("/api/config")
    def config():
        return {
            "edition": s.edition,
            "lang": s.lang,
            "other_url": s.other_url,
            "providers": prov.status(),
            "default_llm": default_llm(prov),
            "limits": {"max_audio_sec": s.max_audio_sec, "max_upload_mb": s.max_upload_mb,
                       "ip_per_hour": s.ip_per_hour, "daily_left": limits.remaining_today()},
            "samples": [{"id": r["id"], "title": r["title"], "lang": r["lang"], "kind": r["kind"],
                         "file": f"/samples/{r['id']}/{r['file']}", "duration_sec": r["duration_sec"]}
                        for r in samples.values()],
            "crm": crm.config(),
            "telegram": bool(s.telegram_token),
            "telegram_username": getattr(app.state, "tg_username", ""),
            "accounts": {"enabled": accounts.enabled, "widget": s.accounts_widget, "auth_url": s.auth_public_url},
        }

    @app.get("/api/samples/{sid}")
    def sample(sid: str):
        if sid not in samples:
            raise ApiError(404, "not_found", "unknown sample")
        return sample_payload(sid)

    def sample_payload(sid: str) -> dict[str, Any]:
        r = samples[sid]
        out = dict(r)
        out["file"] = f"/samples/{sid}/{r['file']}"
        out["reply"] = {"text": r["reply"]["text"], "audio": f"/samples/{sid}/{r['reply']['audio']}"}
        out["mode"] = {"stt": "sample", "llm": "sample", "tts": "sample"}
        out["transcript"] = {**r["transcript"], "provider": "sample"}
        out["notices"] = ["sample"]
        return out

    @app.post("/api/process")
    async def process(req: Request, file: UploadFile | None = File(None), sample: str = Form(""),
                      llm: str = Form(""), lang: str = Form(""), browser_transcript: str = Form(""),
                      consent: str = Form("")):
        llm = check_llm(llm)
        lang = lang or s.lang
        if sample:
            r = samples.get(sample)
            if not r:
                raise ApiError(404, "not_found", "unknown sample")
            if not prov.stt_live:  # demo mode: replay the pre-computed result
                return sample_payload(sample)
            path = s.samples_dir / sample / "stt.ogg"  # OGG/Opus copy: accepted by SpeechKit and OpenAI alike
            data, filename, duration = path.read_bytes(), path.name, float(r["duration_sec"])
        elif file is not None:
            if s.edition == "ru" and consent != CONSENT_VERSION:  # legal ui-texts §4б: own recordings need consent
                raise ApiError(400, "consent_required", "Отметьте согласие над загрузкой, чтобы обработать свою запись.")
            data = await file.read(max_bytes + 1)
            if len(data) > max_bytes:
                raise ApiError(413, "too_large", f"Файл больше {s.max_upload_mb} МБ." if s.lang == "ru" else f"Max upload is {s.max_upload_mb} MB.")
            if not data:
                raise ApiError(400, "invalid_argument", "empty file")
            filename = file.filename or "audio.wav"
            duration = wav_duration(data)
        else:
            raise ApiError(400, "invalid_argument", "send a file or a sample id")

        live = prov.stt_live or prov.llm_live(llm) or prov.tts_live
        notices: list[str] = []
        if prov.stt_live:
            if duration is None:
                raise ApiError(400, "need_wav", "Не удалось прочитать запись. Попробуйте mp3, m4a или wav." if s.lang == "ru"
                               else "Live mode expects 16 kHz mono WAV (the page converts files automatically).")
            if duration > s.max_audio_sec:
                raise ApiError(400, "too_long", f"Запись длиннее {s.max_audio_sec} с: сократите её или откройте пример." if s.lang == "ru"
                               else f"Live mode accepts audio up to {s.max_audio_sec} s.")
        g = gate_for(req, live)
        client = prov.client(llm)

        t = None
        if prov.stt_live:
            t = run(g, STT_RESOURCE, stt_units(duration), lambda: client.transcribe(data, filename=filename))
        if t is not None:
            transcript = {"text": t.text, "language": t.language, "provider": t.provider,
                          "segments": [{"start": x.start, "end": x.end, "text": x.text} for x in t.segments]}
        elif sample:  # live refused for a sample: replay the pre-computed result
            out = sample_payload(sample)
            out["notices"] = (g.notices() if g else []) + out["notices"]
            return out
        elif browser_transcript.strip():
            text = browser_transcript.strip()[:20000]
            transcript = {"text": text, "language": pipeline.guess_lang(text, lang or "en"), "provider": "browser",
                          "segments": []}
            notices.append("stt_browser")
        else:
            t = mock.transcribe(data, filename=filename)
            text = RU_MOCK_TRANSCRIPT if s.lang == "ru" else t.text  # RU: a plain imitation text, not module internals
            transcript = {"text": text, "language": lang or t.language, "provider": t.provider, "segments": []}
            notices.append("stt_mock")
        out_lang = pipeline.guess_lang(transcript["text"], lang or "en") if transcript["provider"] != "mock" else (lang or "en")

        names = sorted({x["owner"] for x in (samples[sample]["tasks"] if sample else []) if x.get("owner")})
        result, llm_live = analyze_text(g, llm, client, transcript["text"], out_lang, names)
        if not llm_live:
            notices.append("llm_mock")
        audio = speak_text(g, client, result["reply"]["text"])
        result["reply"].update(audio or {"audio": None, "provider": "browser"})
        if g:
            notices = g.notices() + notices
        return {"transcript": transcript, "duration_sec": duration, "lang": out_lang, **result,
                "mode": {"stt": "live" if transcript["provider"] not in ("mock", "browser") else transcript["provider"],
                         "llm": "live" if llm_live else "mock",
                         "tts": "live" if audio else "browser"},
                "account": account_info(g),
                "notices": notices}

    def analyze_text(g: Gate | None, llm: str, client, text: str, lang: str,
                     names: list[str] | None = None) -> tuple[dict[str, Any], bool]:
        if prov.llm_live(llm):
            r = run(g, LLM_RESOURCE.get(llm, llm), 1,
                    lambda: pipeline.analyze(client, text, lang, live_llm=True, known_names=names))
            if r is not None:
                return r, True
        return pipeline.analyze(mock, text, lang, live_llm=False), False

    def speak_text(g: Gate | None, client, text: str) -> dict[str, str] | None:
        if not prov.tts_live:
            return None
        return run(g, TTS_RESOURCE if prov.speech == "yandex_speechkit" else prov.speech, 1,
                   lambda: pipeline.speak(client, text))

    def account_info(g: Gate | None) -> dict[str, Any] | None:
        return None if g is None else {"user_left": g.user_left}

    @app.post("/api/consent")
    def consent_mark(req: Request, body: ConsentIn):
        """RU: the page calls this when the consent box is ticked (before any upload); safe to repeat."""
        if s.edition != "ru":
            raise ApiError(404, "not_found", "no consent in this edition")
        if body.version != CONSENT_VERSION:
            raise ApiError(400, "invalid_argument", "unknown consent version")
        return {"recorded": accounts.record_consent(req.headers, "voice", body.version), "version": body.version}

    @app.post("/api/crm/tasks")
    def crm_tasks(req: Request, body: CrmIn):
        """RU: real tasks in the CRM test portal, only for a ready-made sample (fictional names, no visitor text)."""
        if not crm.enabled:
            raise ApiError(404, "not_found", "Отправка в CRM в этом демо не подключена.")
        if body.sample not in samples:
            raise ApiError(400, "invalid_argument", "Отправить в CRM можно задачи готового примера.")
        ip = client_ip(req)
        why = crm.limits.check(ip)
        if why:
            raise ApiError(429, "limit_" + why, "Лимит отправок на сегодня исчерпан, попробуйте позже."
                           if why == "daily" else "Не чаще трёх отправок в час, попробуйте позже.")
        try:
            tasks = crm.send(crm.limits.key(ip), body.sample, samples[body.sample])
        except CrmError as e:
            log.warning("crm error: %s (%s)", e.code.value, e.provider)
            raise ApiError(502, "crm_unavailable", f"{crm.name} сейчас не ответил, попробуйте позже.") from None
        return {"provider": crm.provider, "name": crm.name, "tasks": tasks}

    @app.post("/api/analyze")
    def analyze(req: Request, body: AnalyzeIn):
        llm = check_llm(body.llm)
        g = gate_for(req, prov.llm_live(llm))
        lang = pipeline.guess_lang(body.text, body.lang or s.lang)
        result, live = analyze_text(g, llm, prov.client(llm), body.text, lang)
        notices = (g.notices() if g else []) + ([] if live else ["llm_mock"])
        return {**result, "lang": lang, "mode": {"llm": "live" if live else "mock"}, "account": account_info(g),
                "notices": notices}

    @app.post("/api/speak")
    def speak(req: Request, body: SpeakIn):
        if not prov.tts_live:
            raise ApiError(409, "tts_demo", "Live TTS is off in demo mode; the page uses the browser voice.")
        g = gate_for(req, True)
        audio = speak_text(g, prov.client(default_llm(prov)), body.text)
        if audio is None:
            raise ApiError(409, "tts_refused", "Live voice is not available right now; the page uses the browser voice.")
        return audio

    app.mount("/samples", StaticFiles(directory=s.samples_dir), name="samples")
    app.mount("/static", StaticFiles(directory=s.static_dir), name="static")

    page = (s.static_dir / "index.html").read_text()
    if s.edition == "ru":  # crawlers and link previews see the edition's language without JS
        for a, b in RU_META:
            page = page.replace(a, b)
    else:  # RU-only blocks (consent above upload) are not part of the intl page
        page = re.sub(r"\s*<!--ru-only-->.*?<!--/ru-only-->", "", page, flags=re.S)

    @app.get("/")
    def index():
        return HTMLResponse(page, headers={"Cache-Control": "no-cache"})

    return app


app = create_app()
