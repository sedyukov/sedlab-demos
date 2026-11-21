from __future__ import annotations

import httpx

from sedlab_ai import Client
from sedlab_telegram_core import Bot
from sedlab_telegram_core.api import MockAPI
from voice_demo.limits import Limits
from voice_demo.providers import Providers
from voice_demo.settings import Settings
from voice_demo.tgbot import VoiceBot, format_result

from test_app import FakeLLM, FakeSpeech


class RecordingAPI(MockAPI):
    """telegram-core mock + canned getUpdates/getFile/sendChatAction."""

    def __init__(self, updates):
        super().__init__()
        self.updates, self.extra = updates, []

    def call(self, method, params):
        if method == "getUpdates":
            u, self.updates = self.updates, []
            return u
        if method == "getFile":
            return {"file_id": params["file_id"], "file_path": "voice/file_1.oga"}
        if method == "sendChatAction":
            self.extra.append(method)
            return True
        return super().call(method, params)


def make_bot(updates, live=True):
    env = {"AI_OPENAI_API_KEY": "k"} if live else {}
    s = Settings.from_env({**env, "VOICE_TELEGRAM_TOKEN": "123:abc", "VOICE_MAX_AUDIO_SEC": "60"})
    prov = Providers(s.environ)
    sp, llm = FakeSpeech(), FakeLLM()
    prov.client = lambda name: Client(llm, sp, sp)  # type: ignore[method-assign]
    http_calls = []

    def handler(req: httpx.Request) -> httpx.Response:
        http_calls.append((req.method, req.url.path))
        if req.method == "GET":
            return httpx.Response(200, content=b"OggS-fake")
        return httpx.Response(200, json={"ok": True, "result": {}})

    api = RecordingAPI(updates)
    vb = VoiceBot(s, prov, Limits(10, 100), http=httpx.Client(transport=httpx.MockTransport(handler)),
                  bot=Bot(api, token="123:abc"), api_base="https://tg.test")
    return vb, api, http_calls


def msg(uid, **kw):
    return {"update_id": uid, "message": {"message_id": uid, "chat": {"id": 42}, "date": 0,
                                          "from": {"id": 7, "language_code": "ru"}, **kw}}


def sent_texts(api):
    return [c.params["text"] for c in api.calls("sendMessage")]


def test_voice_message_full_cycle():
    vb, api, http = make_bot([msg(1, voice={"file_id": "F", "duration": 3})])
    assert vb.poll_once() == 1 and vb.offset == 2
    texts = sent_texts(api)
    assert len(texts) == 1
    assert "Live summary." in texts[0] and "Send invoice" in texts[0] and "Kate" in texts[0]
    assert ("GET", "/file/bot123:abc/voice/file_1.oga") in http
    assert ("POST", "/bot123:abc/sendVoice") in http


def test_start_too_long_and_demo():
    vb, api, _ = make_bot([msg(1, text="/start"), msg(2, voice={"file_id": "F", "duration": 600})])
    vb.poll_once()
    t = sent_texts(api)
    assert "голосовое" in t[0] and "60" in t[1]
    vb, api, http = make_bot([msg(1, voice={"file_id": "F", "duration": 3})], live=False)
    vb.poll_once()
    assert "выключено" in sent_texts(api)[0] and not http


def test_text_message_is_analyzed():
    vb, api, http = make_bot([msg(1, text="Kate sends the invoice")])
    vb.poll_once()
    assert "Live summary." in sent_texts(api)[0]


def test_format_result_escapes_html():
    out = format_result("<b>x</b>", {"summary": "a & b", "tasks": [{"title": "<t>", "owner": None, "due": None}]}, "en")
    assert "&lt;b&gt;" in out and "a &amp; b" in out and "&lt;t&gt;" in out
