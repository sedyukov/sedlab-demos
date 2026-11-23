from __future__ import annotations

import json

from sedlab_ai import Client
from sedlab_telegram_core import Bot
from sedlab_telegram_core.api import MockAPI
from voice_demo.limits import Limits
from voice_demo.providers import Providers
from voice_demo.settings import Settings
from voice_demo.tgbot import VoiceBot, format_result

from test_app import FakeLLM, FakeSpeech


def make_bot(live=True, env_extra=None, consent=True):
    env = {"EDITION": "ru", **({"AI_GIGACHAT_AUTH_KEY": "g", "AI_YANDEX_API_KEY": "y"} if live else {})}
    env.update(env_extra or {})
    s = Settings.from_env({**env, "VOICE_TELEGRAM_TOKEN": "123:abc", "VOICE_MAX_AUDIO_SEC": "60",
                           "VOICE_MAX_UPLOAD_MB": "1"})
    prov = Providers(s.environ)
    sp, llm = FakeSpeech(), FakeLLM()
    prov.client = lambda name: Client(llm, sp, sp)  # type: ignore[method-assign]
    api = MockAPI()
    vb = VoiceBot(s, prov, Limits(10, 100), bot=Bot(api, token="123:abc"))
    if consent and vb.consents is not None:
        vb.consents.give(42)
    return vb, api


def push(api, uid, **msg):
    api.push_update(json.dumps({"update_id": uid, "message": {
        "message_id": uid, "chat": {"id": 42}, "date": 0, "from": {"id": 7, "language_code": "ru"}, **msg}}))


def sent_texts(api):
    return [c.params["text"] for c in api.calls("sendMessage")]


def test_voice_message_full_cycle():
    vb, api = make_bot()
    api.put_file("F", b"OggS-fake-voice")
    push(api, 1, voice={"file_id": "F", "duration": 3, "mime_type": "audio/ogg", "file_size": 15})
    assert vb.poll_once() == 1 and vb.offset == 2
    texts = sent_texts(api)
    assert len(texts) == 1
    assert "Live summary." in texts[0] and "Send invoice" in texts[0] and "Kate" in texts[0]
    assert [c.method for c in api.calls("getFile")] == ["getFile"]
    v = api.calls("sendVoice")
    assert len(v) == 1 and v[0].params["chat_id"] == 42 and v[0].params["voice_size"] > 0
    assert vb.poll_once() == 0  # offset moved past the handled update


def test_start_limits_and_demo_mode():
    vb, api = make_bot()
    push(api, 1, text="/start")
    push(api, 2, voice={"file_id": "F", "duration": 600})
    push(api, 3, audio={"file_id": "G", "duration": 5, "file_size": 5 * 1024 * 1024})
    push(api, 4, document={"file_id": "D", "mime_type": "application/pdf", "file_name": "x.pdf"})
    vb.poll_once()
    t = sent_texts(api)
    assert "голосовое" in t[0] and "60" in t[1] and "1 МБ" in t[2] and "Пришлите" in t[3]
    assert not api.calls("getFile")

    vb, api = make_bot(live=False)
    push(api, 1, voice={"file_id": "F", "duration": 3})
    vb.poll_once()
    assert "не подключено" in sent_texts(api)[0] and not api.calls("getFile")


def test_text_message_is_analyzed():
    vb, api = make_bot()
    push(api, 1, text="Kate sends the invoice")
    vb.poll_once()
    assert "Live summary." in sent_texts(api)[0]


def test_handler_error_is_reported_not_raised():
    vb, api = make_bot()
    push(api, 1, voice={"file_id": "missing", "duration": 3})  # mock has no such file → getFile fails
    assert vb.poll_once() == 1
    assert "Что-то пошло не так" in sent_texts(api)[-1]


def test_format_result_escapes_html():
    out = format_result("<b>x</b>", {"summary": "a & b", "tasks": [{"title": "<t>", "owner": None, "due": None}]}, "en")
    assert "&lt;b&gt;" in out and "a &amp; b" in out and "&lt;t&gt;" in out


def test_speechkit_asks_for_supported_format():
    vb, api = make_bot(env_extra={"AI_YANDEX_API_KEY": "y"})
    api.put_file("V", b"OggS-voice")
    push(api, 1, video_note={"file_id": "R", "duration": 5})
    push(api, 2, document={"file_id": "M", "mime_type": "audio/mp4", "file_name": "call.m4a"})
    push(api, 3, voice={"file_id": "V", "duration": 3})
    vb.poll_once()
    t = sent_texts(api)
    assert "mp3/ogg" in t[0] and "mp3/ogg" in t[1] and "Live summary." in t[2]
    assert [c.params["file_id"] for c in api.calls("getFile")] == ["V"]


def push_callback(api, uid, data):
    api.push_update(json.dumps({"update_id": uid, "callback_query": {
        "id": f"cq{uid}", "data": data, "from": {"id": 7}, "message": {"message_id": 1, "chat": {"id": 42}, "date": 0}}}))


def test_ru_bot_asks_consent_before_anything():
    from voice_demo.tgbot import CONSENT_BUTTON, CONSENT_DATA
    vb, api = make_bot(consent=False)
    api.put_file("F", b"OggS-voice")
    push(api, 1, voice={"file_id": "F", "duration": 3})
    vb.poll_once()
    first = api.calls("sendMessage")[0].params
    assert "тестовые данные" in first["text"] and "/delete" in first["text"] and "—" not in first["text"]
    assert first["reply_markup"]["inline_keyboard"][0][0] == {"text": CONSENT_BUTTON, "callback_data": CONSENT_DATA}
    assert not api.calls("getFile")  # nothing downloaded or processed before consent
    push_callback(api, 2, CONSENT_DATA)
    vb.poll_once()
    assert api.calls("answerCallbackQuery") and vb.consents.has(42)
    push(api, 3, voice={"file_id": "F", "duration": 3})
    vb.poll_once()
    assert "Live summary." in sent_texts(api)[-1]


def test_ru_bot_delete_forgets_consent():
    vb, api = make_bot()
    push(api, 1, text="/delete")
    vb.poll_once()
    assert sent_texts(api)[-1] == "Готово, данные этого чата удалены." and not vb.consents.has(42)


def test_consent_expires_after_7_days_and_is_hashed():
    from voice_demo.tgbot import Consents
    now = [1000.0]
    c = Consents(clock=lambda: now[0])
    c.give(123456789)
    assert c.has(123456789) and all("123456789" not in k for k in c._m)
    now[0] += 7 * 24 * 3600 + 1
    assert not c.has(123456789) and not c._m


def test_intl_bot_has_no_consent_step_and_shows_test_data_note():
    vb, api = make_bot(env_extra={"EDITION": "intl"}, live=False)
    assert vb.consents is None
    push(api, 1, text="/start")
    vb.poll_once()
    assert "Demo: use test data only. Nothing is stored after the session." in sent_texts(api)[0]


def test_ru_bot_text_in_imitation_is_labelled():
    vb, api = make_bot(live=False)
    vb.prov.client = lambda name: Client.mock()  # type: ignore[method-assign]
    push(api, 1, text="Анна пришлёт отчёт до пятницы.")
    vb.poll_once()
    out = sent_texts(api)[-1]
    assert out.startswith("<i>Имитация") and "Кратко" in out and "Саммари" not in out


def test_bot_uses_tg_relay_base_url():
    s = Settings.from_env({"EDITION": "ru", "VOICE_TELEGRAM_TOKEN": "1:a", "TELEGRAM_API_BASE": "https://tg-relay.example/"})
    assert s.telegram_api_base == "https://tg-relay.example"
    vb = VoiceBot(s, Providers(s.environ), Limits(1, 1))
    assert str(vb.bot.api._client.base_url).rstrip("/") == "https://tg-relay.example"
    assert Settings.from_env({}).telegram_api_base == "https://api.telegram.org"
