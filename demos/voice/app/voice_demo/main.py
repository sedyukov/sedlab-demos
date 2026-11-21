"""voice demo — FastAPI app: upload or record audio → transcript → summary → tasks → spoken reply.

Without keys everything runs on the ai module's mock + pre-computed samples; with AI_OPENAI_API_KEY /
AI_ANTHROPIC_API_KEY the same endpoints go live behind per-IP and daily limits.
"""
from __future__ import annotations

import json
import logging
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

from fastapi import FastAPI, File, Form, Request, UploadFile
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

from sedlab_ai import Code, ModuleError

from . import pipeline
from .limits import Limits, wav_duration
from .providers import LLMS, Providers
from .settings import Settings

log = logging.getLogger("voice")

STATUS = {Code.INVALID_ARGUMENT: 400, Code.RATE_LIMITED: 429, Code.NOT_IMPLEMENTED: 501,
          Code.UNAUTHENTICATED: 502, Code.CONFIG: 500}


class ApiError(Exception):
    def __init__(self, status: int, code: str, message: str = "") -> None:
        self.status, self.code, self.message = status, code, message


class AnalyzeIn(BaseModel):
    text: str = Field(min_length=1, max_length=20000)
    llm: str = "openai"
    lang: str = ""


class SpeakIn(BaseModel):
    text: str = Field(min_length=1, max_length=4000)


def load_samples(d: Path) -> dict[str, dict[str, Any]]:
    rs = [json.loads(f.read_text()) for f in d.glob("*/result.json")]
    return {r["id"]: r for r in sorted(rs, key=lambda r: (r.get("order", 99), r["id"]))}


def create_app(settings: Settings | None = None) -> FastAPI:
    s = settings or Settings.from_env()
    prov = Providers(s.environ)
    limits = Limits(s.ip_per_hour, s.daily_live)
    samples = load_samples(s.samples_dir)
    max_bytes = s.max_upload_mb * 1024 * 1024

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        bot = None
        if s.telegram_token:
            from .tgbot import VoiceBot
            bot = VoiceBot(s, prov, limits)
            bot.start()
        yield
        if bot:
            bot.stop()

    app = FastAPI(title="voice demo", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
    app.state.settings, app.state.providers, app.state.limits = s, prov, limits

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

    def check_llm(name: str) -> str:
        if name not in LLMS:
            raise ApiError(400, "invalid_argument", f"llm must be one of {', '.join(LLMS)}")
        return name

    @app.get("/health")
    def health():
        return {"ok": True}

    @app.get("/api/config")
    def config():
        return {
            "providers": prov.status(),
            "limits": {"max_audio_sec": s.max_audio_sec, "max_upload_mb": s.max_upload_mb,
                       "ip_per_hour": s.ip_per_hour, "daily_left": limits.remaining_today()},
            "samples": [{"id": r["id"], "title": r["title"], "lang": r["lang"], "kind": r["kind"],
                         "file": f"/samples/{r['id']}/{r['file']}", "duration_sec": r["duration_sec"]}
                        for r in samples.values()],
            "telegram": bool(s.telegram_token),
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
                      llm: str = Form("openai"), lang: str = Form(""), browser_transcript: str = Form("")):
        llm = check_llm(llm)
        if sample:
            r = samples.get(sample)
            if not r:
                raise ApiError(404, "not_found", "unknown sample")
            if not prov.stt_live:  # demo mode: replay the pre-computed result
                return sample_payload(sample)
            path = s.samples_dir / sample / r["file"]
            data, filename, duration = path.read_bytes(), path.name, float(r["duration_sec"])
        elif file is not None:
            data = await file.read(max_bytes + 1)
            if len(data) > max_bytes:
                raise ApiError(413, "too_large", f"Max upload is {s.max_upload_mb} MB.")
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
                raise ApiError(400, "need_wav", "Live mode expects 16 kHz mono WAV (the page converts files automatically).")
            if duration > s.max_audio_sec:
                raise ApiError(400, "too_long", f"Live mode accepts audio up to {s.max_audio_sec} s.")
        if live:
            spend(req)

        client = prov.client(llm)
        if prov.stt_live:
            t = client.transcribe(data, filename=filename)
            transcript = {"text": t.text, "language": t.language, "provider": t.provider,
                          "segments": [{"start": x.start, "end": x.end, "text": x.text} for x in t.segments]}
        elif browser_transcript.strip():
            text = browser_transcript.strip()[:20000]
            transcript = {"text": text, "language": pipeline.guess_lang(text, lang or "en"), "provider": "browser",
                          "segments": []}
            notices.append("stt_browser")
        else:
            t = client.transcribe(data, filename=filename)
            transcript = {"text": t.text, "language": lang or t.language, "provider": t.provider, "segments": []}
            notices.append("stt_mock")
        out_lang = pipeline.guess_lang(transcript["text"], lang or "en") if transcript["provider"] != "mock" else (lang or "en")
        result = pipeline.analyze(client, transcript["text"], out_lang, live_llm=prov.llm_live(llm))
        if not prov.llm_live(llm):
            notices.append("llm_mock")
        result["reply"].update(pipeline.speak(client, result["reply"]["text"]) if prov.tts_live
                               else {"audio": None, "provider": "browser"})
        return {"transcript": transcript, "duration_sec": duration, "lang": out_lang, **result,
                "mode": {"stt": "live" if prov.stt_live else transcript["provider"],
                         "llm": "live" if prov.llm_live(llm) else "mock",
                         "tts": "live" if prov.tts_live else "browser"},
                "notices": notices}

    @app.post("/api/analyze")
    def analyze(req: Request, body: AnalyzeIn):
        llm = check_llm(body.llm)
        if prov.llm_live(llm):
            spend(req)
        lang = pipeline.guess_lang(body.text, body.lang or "en")
        result = pipeline.analyze(prov.client(llm), body.text, lang, live_llm=prov.llm_live(llm))
        notices = [] if prov.llm_live(llm) else ["llm_mock"]
        return {**result, "lang": lang, "mode": {"llm": "live" if prov.llm_live(llm) else "mock"}, "notices": notices}

    @app.post("/api/speak")
    def speak(req: Request, body: SpeakIn):
        if not prov.tts_live:
            raise ApiError(409, "tts_demo", "Live TTS is off in demo mode; the page uses the browser voice.")
        spend(req)
        return pipeline.speak(prov.client("openai"), body.text)

    app.mount("/samples", StaticFiles(directory=s.samples_dir), name="samples")
    app.mount("/static", StaticFiles(directory=s.static_dir), name="static")

    @app.get("/")
    def index():
        return FileResponse(s.static_dir / "index.html", headers={"Cache-Control": "no-cache"})

    return app


app = create_app()
