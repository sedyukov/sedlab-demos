from __future__ import annotations

import struct

import pytest
from fastapi.testclient import TestClient

from sedlab_ai import Audio, ChatInput, ChatResult, Client, Segment, Transcript, Usage, silence_wav
from voice_demo.limits import Limits, wav_duration
from voice_demo.main import create_app
from voice_demo.pipeline import guess_lang, template_reply
from voice_demo.settings import Settings
from sedlab_ai import Task


def wav(seconds: float, rate: int = 16000) -> bytes:
    n = int(seconds * rate)
    return (b"RIFF" + struct.pack("<I", 36 + 2 * n) + b"WAVEfmt " + struct.pack("<IHHIIHH", 16, 1, 1, rate, rate * 2, 2, 16)
            + b"data" + struct.pack("<I", 2 * n) + b"\0\0" * n)


def make(env: dict[str, str] | None = None, **kw) -> TestClient:
    s = Settings.from_env({"VOICE_TRUST_PROXY": "1", **(env or {})})
    for k, v in kw.items():
        setattr(s, k, v)
    return TestClient(create_app(s))


# ---------- demo mode (no keys) ----------

def test_health_and_index():
    c = make()
    assert c.get("/health").json() == {"ok": True}
    r = c.get("/")
    assert r.status_code == 200 and "Voice AI Assistant" in r.text


def test_config_demo_mode():
    j = make().get("/api/config").json()
    assert j["providers"] == {"llm": {"openai": "demo", "anthropic": "demo", "gigachat": "demo", "yandexgpt": "demo"},
                              "stt": "demo", "tts": "demo"}
    assert [s["id"] for s in j["samples"]] == ["standup-en", "planning-ru", "voice-note-ru"]
    assert j["telegram"] is False


@pytest.mark.parametrize("sid", ["standup-en", "planning-ru", "voice-note-ru"])
def test_samples_are_complete_and_served(sid):
    c = make()
    r = c.get(f"/api/samples/{sid}").json()
    assert r["transcript"]["segments"] and r["summary"] and r["tasks"] and r["reply"]["text"]
    assert all(t["title"] for t in r["tasks"])
    assert c.get(r["file"]).status_code == 200
    assert c.get(r["reply"]["audio"]).status_code == 200
    # process with a sample id in demo mode replays the same pre-computed result
    p = c.post("/api/process", data={"sample": sid, "llm": "anthropic"}).json()
    assert p["summary"] == r["summary"] and p["notices"] == ["sample"]


def test_unknown_sample_and_bad_llm():
    c = make()
    assert c.get("/api/samples/nope").status_code == 404
    assert c.post("/api/process", data={"sample": "nope"}).status_code == 404
    assert c.post("/api/analyze", json={"text": "hi", "llm": "gpt5"}).status_code == 400


def test_upload_demo_mode_uses_mock_and_browser_voice():
    c = make()
    r = c.post("/api/process", files={"file": ("a.wav", wav(2), "audio/wav")}, data={"llm": "openai", "lang": "en"})
    j = r.json()
    assert r.status_code == 200
    assert j["transcript"]["provider"] == "mock" and j["duration_sec"] == pytest.approx(2.0)
    assert j["reply"]["audio"] is None and j["mode"]["tts"] == "browser"
    assert "stt_mock" in j["notices"] and "llm_mock" in j["notices"]


def test_upload_demo_mode_with_browser_transcript():
    c = make()
    text = "Обсудили релиз.\n- Подготовить отчёт @Анна 2026-12-01"
    j = c.post("/api/process", files={"file": ("a.webm", b"xx", "audio/webm")},
               data={"llm": "gigachat", "lang": "en", "browser_transcript": text}).json()
    assert j["transcript"]["provider"] == "browser" and j["lang"] == "ru"
    assert j["summary"] == "Обсудили релиз."
    assert j["tasks"] == [{"title": "Подготовить отчёт", "owner": "Анна", "due": "2026-12-01"}]
    assert "llm_mock" in j["notices"] and "stt_browser" in j["notices"]
    assert j["reply"]["text"].startswith("Коротко:")


def test_upload_limits_any_mode():
    c = make(max_upload_mb=1)
    assert c.post("/api/process", files={"file": ("a.wav", b"\0" * (1024 * 1024 + 10), "audio/wav")}).status_code == 413
    assert c.post("/api/process", files={"file": ("a.wav", b"", "audio/wav")}).status_code == 400
    assert c.post("/api/process", data={"llm": "openai"}).status_code == 400


def test_analyze_and_speak_demo():
    c = make()
    j = c.post("/api/analyze", json={"text": "Release sync.\n- Ship it @Liam 2026-11-30", "llm": "openai"}).json()
    assert j["summary"] == "Release sync." and j["tasks"][0]["owner"] == "Liam" and j["mode"]["llm"] == "mock"
    assert c.post("/api/speak", json={"text": "hi"}).status_code == 409


def test_index_has_no_inline_secrets():
    html = make().get("/").text
    assert "sk-" not in html and "API_KEY" not in html


# ---------- live mode (fake providers behind the same Client) ----------

class FakeLLM:
    name = "fake"

    def __init__(self) -> None:
        self.calls = 0

    def chat(self, req: ChatInput) -> ChatResult:
        self.calls += 1
        sys = req.messages[0].content
        if sys.startswith("You summarize"):
            text = "Live summary."
        elif sys.startswith("You extract"):
            text = '{"tasks":[{"title":"Send invoice","owner":"Kate","due":"2026-12-02"}]}'
        else:
            text = "Live spoken reply."
        return ChatResult(text=text, provider="fake", model="fake-1", usage=Usage(10, 5), finish_reason="stop")


class FakeSpeech:
    name = "fake"

    def transcribe(self, req):
        return Transcript(text="Kate sends the invoice by December second.", language="en", provider="fake",
                          duration_sec=2.0, segments=[Segment(0.0, 2.0, "Kate sends the invoice by December second.")])

    def synthesize(self, req):
        return Audio(bytes=silence_wav(10), mime="audio/mpeg", provider="fake")


@pytest.fixture
def live():
    env = {"AI_OPENAI_API_KEY": "test-key", "VOICE_IP_PER_HOUR": "2", "VOICE_DAILY_LIVE": "3",
           "VOICE_MAX_AUDIO_SEC": "5"}
    c = make(env)
    llm, sp = FakeLLM(), FakeSpeech()
    c.app.state.providers.client = lambda name: Client(llm, sp, sp)
    c.llm = llm
    return c


def test_gigachat_yandex_live_detection():
    j = make({"AI_GIGACHAT_AUTH_KEY": "x", "AI_YANDEX_API_KEY": "y"}).get("/api/config").json()
    assert j["providers"]["llm"]["gigachat"] == "live"
    assert j["providers"]["llm"]["yandexgpt"] == "demo"  # folder id is required too
    j = make({"AI_YANDEX_API_KEY": "y", "AI_YANDEX_FOLDER_ID": "f"}).get("/api/config").json()
    assert j["providers"]["llm"]["yandexgpt"] == "live"


def test_live_config(live):
    j = live.get("/api/config").json()
    assert j["providers"]["stt"] == "live" and j["providers"]["llm"]["openai"] == "live"
    assert j["providers"]["llm"]["anthropic"] == "demo"
    assert j["limits"]["daily_left"] == 3


def test_live_pipeline(live):
    r = live.post("/api/process", files={"file": ("a.wav", wav(2), "audio/wav")}, data={"llm": "openai"},
                  headers={"X-Forwarded-For": "1.1.1.1"})
    j = r.json()
    assert r.status_code == 200, j
    assert j["transcript"]["provider"] == "fake" and j["transcript"]["segments"]
    assert j["summary"] == "Live summary."
    assert j["tasks"] == [{"title": "Send invoice", "owner": "Kate", "due": "2026-12-02"}]
    assert j["reply"]["text"] == "Live spoken reply." and j["reply"]["audio"].startswith("data:audio/mpeg;base64,")
    assert j["mode"] == {"stt": "live", "llm": "live", "tts": "live"} and j["notices"] == []


def test_live_requires_wav_and_max_length(live):
    r = live.post("/api/process", files={"file": ("a.mp3", b"ID3notwav", "audio/mpeg")}, data={"llm": "openai"})
    assert r.status_code == 400 and r.json()["error"] == "need_wav"
    r = live.post("/api/process", files={"file": ("a.wav", wav(6), "audio/wav")}, data={"llm": "openai"})
    assert r.status_code == 400 and r.json()["error"] == "too_long"
    assert live.llm.calls == 0  # rejected before any paid call


def test_live_rate_limit_per_ip_and_daily(live):
    def run(ip):
        return live.post("/api/process", files={"file": ("a.wav", wav(1), "audio/wav")}, data={"llm": "openai"},
                         headers={"X-Forwarded-For": f"9.9.9.9, {ip}"})
    assert run("1.1.1.1").status_code == 200
    assert run("1.1.1.1").status_code == 200
    r = run("1.1.1.1")
    assert r.status_code == 429 and r.json()["error"] == "limit_ip"
    assert run("2.2.2.2").status_code == 200          # 3rd live run today
    r = run("3.3.3.3")
    assert r.status_code == 429 and r.json()["error"] == "limit_daily"


def test_live_analyze_and_speak_count_against_limits(live):
    h = {"X-Forwarded-For": "5.5.5.5"}
    assert live.post("/api/analyze", json={"text": "x", "llm": "openai"}, headers=h).status_code == 200
    assert live.post("/api/speak", json={"text": "x"}, headers=h).status_code == 200
    assert live.post("/api/speak", json={"text": "x"}, headers=h).status_code == 429


# ---------- units ----------

def test_wav_duration():
    assert wav_duration(wav(1.5)) == pytest.approx(1.5)
    assert wav_duration(silence_wav(8000)) == pytest.approx(1.0)
    assert wav_duration(b"OggS....") is None
    assert wav_duration(b"RIFF\0\0\0\0WAVE") is None


def test_limits_reset_next_day():
    day = ["2026-11-21"]
    lim = Limits(ip_per_hour=100, daily=1, today=lambda: day[0])
    assert lim.check("a") is None and lim.check("b") == "daily"
    day[0] = "2026-11-22"
    assert lim.check("b") is None


def test_guess_lang_and_template():
    assert guess_lang("Привет, мир") == "ru" and guess_lang("Hello") == "en" and guess_lang("123", "ru") == "ru"
    r = template_reply("Done.", [Task("Ship", "Liam", "2026-11-30")], "en")
    assert r == "In short: Done. Action items: 1. Liam: Ship — by 2026-11-30."
