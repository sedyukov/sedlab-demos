from __future__ import annotations

import struct
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from sedlab_ai import Audio, ChatInput, ChatResult, Client, Segment, Task, Transcript, Usage, silence_wav
from voice_demo.limits import Limits, wav_duration
from voice_demo.main import create_app
from voice_demo.pipeline import guess_lang, template_reply
from voice_demo.providers import Providers, default_llm
from voice_demo.settings import Settings

SAMPLES = Path(__file__).resolve().parent.parent / "samples"
RU_SAMPLES = ["planning-ru", "voice-note-ru"]
EN_SAMPLES = ["standup-en", "client-call-en", "voice-note-en"]


def wav(seconds: float, rate: int = 16000) -> bytes:
    n = int(seconds * rate)
    return (b"RIFF" + struct.pack("<I", 36 + 2 * n) + b"WAVEfmt " + struct.pack("<IHHIIHH", 16, 1, 1, rate, rate * 2, 2, 16)
            + b"data" + struct.pack("<I", 2 * n) + b"\0\0" * n)


def make(env: dict[str, str] | None = None, edition: str = "ru", **kw) -> TestClient:
    s = Settings.from_env({"VOICE_TRUST_PROXY": "1", "EDITION": edition, **(env or {})})
    for k, v in kw.items():
        setattr(s, k, v)
    return TestClient(create_app(s))


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
    last_filename = ""

    def transcribe(self, req):
        self.last_filename = req.filename
        return Transcript(text="Kate sends the invoice by December second.", language="en", provider="fake",
                          duration_sec=2.0, segments=[Segment(0.0, 2.0, "Kate sends the invoice by December second.")])

    def synthesize(self, req):
        return Audio(bytes=silence_wav(10), mime="audio/mpeg", provider="fake")


# ---------- editions ----------

def test_edition_settings_and_defaults():
    assert Settings.from_env({}).edition == "intl"
    ru = Settings.from_env({"EDITION": "ru", "DEMO_DOMAIN": "demo.example"})
    assert ru.lang == "ru" and ru.live_allowed and ru.other_url == "https://voice-en.demo.example"
    en = Settings.from_env({"EDITION": "intl", "DEMO_DOMAIN": "demo.example"})
    assert en.lang == "en" and not en.live_allowed and en.other_url == "https://voice.demo.example"
    assert Settings.from_env({"EDITION": "intl", "VOICE_LIVE": "1"}).live_allowed
    assert Settings.from_env({"EDITION": "ru", "VOICE_LIVE": ""}).live_allowed          # empty = edition default
    assert not Settings.from_env({"EDITION": "ru", "VOICE_LIVE": "0"}).live_allowed
    with pytest.raises(ValueError):
        Settings.from_env({"EDITION": "de"})


def test_config_ru_edition():
    j = make(edition="ru").get("/api/config").json()
    assert j["edition"] == "ru" and j["lang"] == "ru" and j["other_url"].startswith("https://voice-en.")
    assert j["providers"] == {"llm": {"yandexgpt": "demo", "gigachat": "demo"}, "stt": "demo", "tts": "demo",
                              "speech": "mock"}
    assert list(j["providers"]["llm"]) == ["yandexgpt", "gigachat"] and j["default_llm"] == "yandexgpt"
    assert [s["id"] for s in j["samples"]] == RU_SAMPLES
    assert j["telegram"] is False


def test_config_intl_edition():
    j = make(edition="intl").get("/api/config").json()
    assert j["edition"] == "intl" and j["lang"] == "en" and j["other_url"].startswith("https://voice.")
    assert j["providers"]["llm"] == {"openai": "demo", "anthropic": "demo"} and j["default_llm"] == "openai"
    assert [s["id"] for s in j["samples"]] == EN_SAMPLES


def test_index_language_per_edition():
    ru = make(edition="ru").get("/").text
    en = make(edition="intl").get("/").text
    assert '<html lang="ru">' in ru and "<title>Голосовой AI-ассистент</title>" in ru and "GigaChat" in ru
    assert '<html lang="en">' in en and "<title>Voice AI Assistant</title>" in en
    assert "GigaChat" not in en and "YandexGPT" not in en
    assert "sk-" not in en and "API_KEY" not in en


def test_foreign_keys_ignored_in_ru_and_russian_in_intl():
    env = {"AI_OPENAI_API_KEY": "k", "AI_ANTHROPIC_API_KEY": "a", "AI_GIGACHAT_AUTH_KEY": "g",
           "AI_YANDEX_API_KEY": "y", "AI_YANDEX_FOLDER_ID": "f", "VOICE_LIVE": "1"}
    ru = make(env, edition="ru").get("/api/config").json()["providers"]
    assert ru == {"llm": {"yandexgpt": "live", "gigachat": "live"}, "stt": "live", "tts": "live",
                  "speech": "yandex_speechkit"}
    en = make(env, edition="intl").get("/api/config").json()["providers"]
    assert en == {"llm": {"openai": "live", "anthropic": "live"}, "stt": "live", "tts": "live", "speech": "openai"}


def test_intl_live_is_forced_off_by_default():
    j = make({"AI_OPENAI_API_KEY": "k", "AI_ANTHROPIC_API_KEY": "a"}, edition="intl").get("/api/config").json()
    assert j["providers"] == {"llm": {"openai": "demo", "anthropic": "demo"}, "stt": "demo", "tts": "demo",
                              "speech": "mock"}


def test_llm_outside_edition_is_rejected():
    assert make(edition="ru").post("/api/analyze", json={"text": "hi", "llm": "openai"}).status_code == 400
    assert make(edition="intl").post("/api/analyze", json={"text": "hi", "llm": "gigachat"}).status_code == 400
    assert make(edition="ru").post("/api/analyze", json={"text": "hi", "llm": "gpt5"}).status_code == 400


# ---------- demo mode (no keys) ----------

@pytest.mark.parametrize("edition", ["ru", "intl"])
def test_health(edition):
    assert make(edition=edition).get("/health").json() == {"ok": True}


@pytest.mark.parametrize("edition,sid", [("ru", x) for x in RU_SAMPLES] + [("intl", x) for x in EN_SAMPLES])
def test_samples_are_complete_and_served(edition, sid):
    c = make(edition=edition)
    r = c.get(f"/api/samples/{sid}").json()
    assert r["transcript"]["segments"] and r["summary"] and r["tasks"] and r["reply"]["text"]
    assert all(t["title"] for t in r["tasks"])
    assert c.get(r["file"]).status_code == 200
    assert c.get(r["reply"]["audio"]).status_code == 200
    p = c.post("/api/process", data={"sample": sid}).json()  # demo mode replays the pre-computed result
    assert p["summary"] == r["summary"] and p["notices"] == ["sample"]


def test_samples_of_other_edition_are_hidden():
    assert make(edition="ru").get("/api/samples/standup-en").status_code == 404
    assert make(edition="intl").get("/api/samples/planning-ru").status_code == 404
    assert make(edition="ru").post("/api/process", data={"sample": "nope"}).status_code == 404


@pytest.mark.parametrize("sid", RU_SAMPLES + EN_SAMPLES)
def test_sample_texts_have_no_em_dash_and_an_ogg_copy(sid):
    assert "—" not in (SAMPLES / sid / "result.json").read_text()
    assert (SAMPLES / sid / "stt.ogg").read_bytes()[:4] == b"OggS"


def test_upload_demo_mode_uses_mock_and_browser_voice():
    j = make(edition="intl").post("/api/process", files={"file": ("a.wav", wav(2), "audio/wav")}).json()
    assert j["transcript"]["provider"] == "mock" and j["duration_sec"] == pytest.approx(2.0)
    assert j["reply"]["audio"] is None and j["mode"]["tts"] == "browser"
    assert "stt_mock" in j["notices"] and "llm_mock" in j["notices"] and j["lang"] == "en"


def test_upload_demo_mode_with_browser_transcript_ru():
    text = "Обсудили релиз.\n- Подготовить отчёт @Анна 2026-12-01"
    j = make(edition="ru").post("/api/process", files={"file": ("a.webm", b"xx", "audio/webm")},
                                data={"browser_transcript": text}).json()
    assert j["transcript"]["provider"] == "browser" and j["lang"] == "ru"
    assert j["summary"] == "Обсудили релиз."
    assert j["tasks"] == [{"title": "Подготовить отчёт", "owner": "Анна", "due": "2026-12-01"}]
    assert "llm_mock" in j["notices"] and "stt_browser" in j["notices"]
    assert j["reply"]["text"].startswith("Коротко:")


def test_upload_limits_any_mode():
    c = make(max_upload_mb=1)
    assert c.post("/api/process", files={"file": ("a.wav", b"\0" * (1024 * 1024 + 10), "audio/wav")}).status_code == 413
    assert c.post("/api/process", files={"file": ("a.wav", b"", "audio/wav")}).status_code == 400
    assert c.post("/api/process", data={}).status_code == 400


def test_analyze_and_speak_demo():
    c = make(edition="intl")
    j = c.post("/api/analyze", json={"text": "Release sync.\n- Ship it @Liam 2026-11-30"}).json()
    assert j["summary"] == "Release sync." and j["tasks"][0]["owner"] == "Liam" and j["mode"]["llm"] == "mock"
    assert c.post("/api/speak", json={"text": "hi"}).status_code == 409


# ---------- live mode (fake providers behind the same Client) ----------

@pytest.fixture
def live():
    """intl edition with live forced on: the local per-IP / daily limiter guards spend (RU: see test_accounts.py)."""
    c = make({"AI_OPENAI_API_KEY": "k", "VOICE_LIVE": "1", "VOICE_IP_PER_HOUR": "2", "VOICE_DAILY_LIVE": "3",
              "VOICE_MAX_AUDIO_SEC": "5"}, edition="intl")
    llm, sp = FakeLLM(), FakeSpeech()
    c.app.state.providers.client = lambda name: Client(llm, sp, sp)
    c.llm, c.sp = llm, sp
    return c


def test_live_config(live):
    j = live.get("/api/config").json()
    assert j["providers"]["stt"] == "live" and j["default_llm"] == "openai"
    assert j["providers"]["llm"][j["default_llm"]] == "live"
    assert j["limits"]["daily_left"] == 3


def test_live_pipeline(live):
    r = live.post("/api/process", files={"file": ("a.wav", wav(2), "audio/wav")}, headers={"X-Forwarded-For": "1.1.1.1"})
    j = r.json()
    assert r.status_code == 200, j
    assert j["transcript"]["provider"] == "fake" and j["transcript"]["segments"]
    assert j["summary"] == "Live summary."
    assert j["tasks"] == [{"title": "Send invoice", "owner": "Kate", "due": "2026-12-02"}]
    assert j["reply"]["text"] == "Live spoken reply." and j["reply"]["audio"].startswith("data:audio/mpeg;base64,")
    assert j["mode"] == {"stt": "live", "llm": "live", "tts": "live"} and j["notices"] == []


def test_live_requires_wav_and_max_length(live):
    r = live.post("/api/process", files={"file": ("a.mp3", b"ID3notwav", "audio/mpeg")})
    assert r.status_code == 400 and r.json()["error"] == "need_wav"
    r = live.post("/api/process", files={"file": ("a.wav", wav(6), "audio/wav")})
    assert r.status_code == 400 and r.json()["error"] == "too_long"
    assert live.llm.calls == 0  # rejected before any paid call


def test_live_rate_limit_per_ip_and_daily(live):
    def run(ip):
        return live.post("/api/process", files={"file": ("a.wav", wav(1), "audio/wav")},
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
    assert live.post("/api/analyze", json={"text": "x"}, headers=h).status_code == 200
    assert live.post("/api/speak", json={"text": "x"}, headers=h).status_code == 200
    assert live.post("/api/speak", json={"text": "x"}, headers=h).status_code == 429


# ---------- providers ----------

def test_provider_selection_per_edition():
    p = Providers({"AI_YANDEX_API_KEY": "y", "AI_OPENAI_API_KEY": "k"}, "ru")
    assert p.speech == "yandex_speechkit" and p.stt_live and p.tts_live
    c = p.client("gigachat")
    assert c.stt.name == "yandex_speechkit" and c.tts.name == "yandex_speechkit" and c.llm.name == "mock"
    assert Providers({"AI_OPENAI_API_KEY": "k"}, "ru").speech == "mock"          # no foreign speech in ru
    assert Providers({"AI_OPENAI_API_KEY": "k"}, "intl").speech == "openai"
    assert Providers({"AI_OPENAI_API_KEY": "k"}, "intl", live_allowed=False).speech == "mock"
    assert Providers({}, "ru").client("gigachat").stt.name == "mock"
    assert default_llm(Providers({}, "ru")) == "yandexgpt"
    assert default_llm(Providers({"AI_GIGACHAT_AUTH_KEY": "g"}, "ru")) == "gigachat"  # the only live one
    assert default_llm(Providers({"AI_YANDEX_API_KEY": "y", "AI_YANDEX_FOLDER_ID": "f"}, "ru")) == "yandexgpt"
    assert default_llm(Providers({"AI_ANTHROPIC_API_KEY": "a"}, "intl")) == "anthropic"
    assert Providers({"AI_YANDEX_API_KEY": "y"}, "ru").llm_live("yandexgpt") is False  # needs a folder id too


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


def test_limiter_keeps_only_hashed_keys_and_forgets_daily():
    day = ["2026-11-21"]
    lim = Limits(ip_per_hour=5, daily=100, today=lambda: day[0])
    lim.check("203.0.113.7")
    lim.check("tg:123456789")
    keys = list(lim._ip._m)
    assert len(keys) == 2
    assert all("203.0.113.7" not in k and "123456789" not in k for k in keys)
    assert lim.key("203.0.113.7") in keys
    day[0] = "2026-11-22"
    lim.check("x")
    assert lim.key("203.0.113.7") not in lim._ip._m and len(lim._ip._m) == 1


def test_guess_lang_and_template():
    assert guess_lang("Привет, мир") == "ru" and guess_lang("Hello") == "en" and guess_lang("123", "ru") == "ru"
    r = template_reply("Done.", [Task("Ship", "Liam", "2026-11-30")], "en")
    assert r == "In short: Done. Action items: 1. Liam: Ship, by 2026-11-30."


def test_gigachat_live_only_with_pii_masking(monkeypatch):
    import voice_demo.providers as pv
    assert Providers({"AI_GIGACHAT_AUTH_KEY": "g"}, "ru").llm_live("gigachat")
    assert Providers({"AI_GIGACHAT_MODE": "cloudru", "AI_GIGACHAT_API_KEY": "c"}, "ru").llm_live("gigachat")
    off = Providers({"AI_GIGACHAT_AUTH_KEY": "g", "AI_GIGACHAT_PII_MASKING": "off"}, "ru")
    assert not off.llm_live("gigachat") and off.client("gigachat").llm.name == "mock"
    c = Providers({"AI_GIGACHAT_AUTH_KEY": "g"}, "ru").client("gigachat")
    assert c.llm.name == "gigachat" and c.pii_masking_enabled()
    monkeypatch.setattr(pv, "GIGACHAT_LIVE_ALLOWED", False)  # emergency stop still works
    assert not Providers({"AI_GIGACHAT_AUTH_KEY": "g"}, "ru").llm_live("gigachat")
