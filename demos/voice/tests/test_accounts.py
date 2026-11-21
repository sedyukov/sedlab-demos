"""RU edition: live calls only for signed-in users, under per-user limits of the accounts service (mock provider)."""
from __future__ import annotations

import pytest

from sedlab_ai import Client
from test_app import FakeLLM, FakeSpeech, make, wav

RU_KEYS = {"AI_YANDEX_API_KEY": "y", "AI_YANDEX_FOLDER_ID": "f", "ACCOUNTS_MOCK_ALLOW_LIVE": "1"}  # YandexGPT + SpeechKit


class FailingLLM(FakeLLM):
    def chat(self, req):
        raise RuntimeError("provider down")


def ru_app(env=None, llm=None):
    c = make({**RU_KEYS, **(env or {})}, edition="ru")
    sp = FakeSpeech()
    c.llm, c.sp = llm or FakeLLM(), sp
    c.app.state.providers.client = lambda name: Client(c.llm, sp, sp)
    c.acc = c.app.state.accounts.client
    return c


def token(c, user="u1"):
    return {"Authorization": "Bearer " + c.acc.mock.issue_token(user, "s-" + user, "yandex").token}


def process(c, headers=None, seconds=2):
    return c.post("/api/process", files={"file": ("a.wav", wav(seconds), "audio/wav")}, data={"consent": "voice-v1"},
                  headers=headers or {})


def test_config_exposes_accounts_only_in_ru():
    j = make(edition="ru").get("/api/config").json()["accounts"]
    assert j == {"enabled": True, "widget": False, "auth_url": "https://auth.demo.sedlab.dev"}
    assert make(edition="intl").get("/api/config").json()["accounts"]["enabled"] is False
    w = make({"VOICE_ACCOUNTS_WIDGET": "1", "DEMO_DOMAIN": "demo.example"}, edition="ru").get("/api/config").json()
    assert w["accounts"]["widget"] is True and w["accounts"]["auth_url"] == "https://auth.demo.example"


def test_not_signed_in_gets_mock_and_login_notice():
    c = ru_app()
    j = process(c).json()
    assert j["mode"] == {"stt": "mock", "llm": "mock", "tts": "browser"}
    assert j["notices"][0] == "acc_login" and c.llm.calls == 0 and c.sp.last_filename == ""


def test_signed_in_goes_live_until_the_daily_limit():
    c = ru_app()
    h = token(c)
    for left in (2, 1, 0):  # mock limits: 3 paid runs per user per day
        j = process(c, h).json()
        assert j["mode"] == {"stt": "live", "llm": "live", "tts": "live"}, j
        assert j["summary"] == "Live summary." and j["account"]["user_left"] == left and j["notices"] == []
    calls = c.llm.calls
    j = process(c, h).json()
    assert j["mode"]["llm"] == "mock" and "acc_limit" in j["notices"] and c.llm.calls == calls
    assert process(c, token(c, "u2")).json()["mode"]["llm"] == "live"  # limits are per user


def test_mock_accounts_never_live_without_explicit_allow():
    c = ru_app({"ACCOUNTS_MOCK_ALLOW_LIVE": "0"})
    j = process(c, token(c)).json()
    assert j["mode"]["llm"] == "mock" and c.llm.calls == 0


def test_provider_failure_refunds_the_unit():
    c = ru_app(llm=FailingLLM())
    h = token(c)
    before = {r.resource: r.user_left for r in c.acc.remaining(h["Authorization"][7:]).resources}
    with pytest.raises(RuntimeError):
        c.post("/api/analyze", json={"text": "Обсудили релиз."}, headers=h)
    after = {r.resource: r.user_left for r in c.acc.remaining(h["Authorization"][7:]).resources}
    assert after["yandex-llm"] == before["yandex-llm"]


def test_auth_down_falls_back_to_mock():
    c = ru_app()
    h = token(c)
    c.acc.mock.set_unavailable(True)
    j = process(c, h).json()
    assert j["mode"]["llm"] == "mock" and "acc_down" in j["notices"] and c.llm.calls == 0


def test_cookie_token_is_accepted():
    c = ru_app()
    c.cookies.set("sl_auth", token(c)["Authorization"][7:])
    assert process(c).json()["mode"]["llm"] == "live"


def test_live_sample_uses_ogg_copy_and_refused_sample_replays():
    c = ru_app()
    r = c.post("/api/process", data={"sample": "planning-ru"}, headers=token(c))
    assert r.status_code == 200 and c.sp.last_filename == "stt.ogg" and r.json()["mode"]["stt"] == "live"
    j = ru_app().post("/api/process", data={"sample": "planning-ru"}).json()  # not signed in
    assert j["notices"] == ["acc_login", "sample"] and j["transcript"]["provider"] == "sample"


def test_stt_units_are_audio_seconds_capped_at_60():
    from voice_demo.account import stt_units
    assert stt_units(2.2) == 3 and stt_units(None) == 1 and stt_units(170) == 60


def test_speak_requires_sign_in_in_ru():
    c = ru_app()
    assert c.post("/api/speak", json={"text": "привет"}).status_code == 409
    assert c.post("/api/speak", json={"text": "привет"}, headers=token(c)).status_code == 200


def test_bad_accounts_config_fails_closed_not_crashes():
    c = make({**RU_KEYS, "ACCOUNTS_PROVIDER": "http"}, edition="ru")  # no service key, no demo
    assert c.app.state.accounts.config_error == "config"
    c.app.state.providers.client = lambda name: Client(FakeLLM(), FakeSpeech(), FakeSpeech())
    j = c.post("/api/process", files={"file": ("a.wav", wav(1), "audio/wav")}, data={"consent": "voice-v1"}).json()
    assert j["mode"]["llm"] == "mock"


class SpyGigaChat(FakeLLM):
    """Looks like GigaChat to the ai module (PII masking on) and records what it was sent."""
    name = "gigachat"

    def __init__(self):
        super().__init__()
        self.seen = []

    def chat(self, req):
        self.seen.append(" ".join(m.content for m in req.messages))
        return super().chat(req)


def test_gigachat_gets_masked_names_for_samples():
    from sedlab_ai import PIIConfig
    c = make({**RU_KEYS, "AI_GIGACHAT_AUTH_KEY": "g"}, edition="ru")
    spy, sp = SpyGigaChat(), FakeSpeech()
    sp.transcribe = lambda req: __import__("sedlab_ai").Transcript(
        text="Сергей проверит возвраты до 27 ноября, Ольга подготовит тексты.", language="ru", provider="fake")
    c.app.state.providers.client = lambda name: Client(spy, sp, sp, pii=PIIConfig())
    r = c.post("/api/process", data={"sample": "planning-ru", "llm": "gigachat"},
               headers={"Authorization": "Bearer " + c.app.state.accounts.client.mock.issue_token("u", "s", "vk").token})
    assert r.status_code == 200 and r.json()["mode"]["llm"] == "live"
    assert spy.seen and all("Сергей" not in x and "Ольга" not in x for x in spy.seen)
