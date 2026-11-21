"""Which providers are live, per edition. All construction goes through the ai module's own from_env factory:
the demo only flips AI_LLM_PROVIDER / AI_STT_PROVIDER / AI_TTS_PROVIDER per request.

Editions (EDITION=ru|intl):
- ru:   GigaChat / YandexGPT + Yandex SpeechKit only (152-FZ); foreign keys are ignored.
- intl: OpenAI / Anthropic + OpenAI speech; live mode is off unless VOICE_LIVE=1 (mock only for now).
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping

from sedlab_ai import Client, Env, from_env

EDITION_LLMS = {"ru": ("gigachat", "yandexgpt"), "intl": ("openai", "anthropic")}
EDITION_SPEECH = {"ru": "yandex_speechkit", "intl": "openai"}
# legal-ru: only depersonalised text may go to GigaChat. The ai module masks PII inside Client.chat for GigaChat
# (AI_GIGACHAT_PII_MASKING=on by default); if someone turns it off, GigaChat never goes live here.
# Hold: the module's masker misses known names ending in "-ей" (e.g. "Сергей"), see test_gigachat_gets_masked_names.
# Flip to True once that test passes (it is a strict xfail and will fail loudly when the module is fixed).
GIGACHAT_LIVE_ALLOWED = False


def default_llm(p: "Providers") -> str:
    """First live provider of the edition, else its first one."""
    return next((n for n in p.llms if p.llm_live(n)), p.llms[0])


@dataclass
class Providers:
    environ: Mapping[str, str]
    edition: str = "ru"
    live_allowed: bool = True

    @property
    def llms(self) -> tuple[str, ...]:
        return EDITION_LLMS[self.edition]

    def _has(self, *keys: str) -> bool:
        return self.live_allowed and any((self.environ.get(k) or "").strip() for k in keys)

    @property
    def openai(self) -> bool:  # key, or an OpenAI-compatible endpoint (Ollama, vLLM...)
        return self._has("AI_OPENAI_API_KEY", "AI_OPENAI_BASE_URL")

    @property
    def anthropic(self) -> bool:
        return self._has("AI_ANTHROPIC_API_KEY")

    @property
    def gigachat(self) -> bool:
        masked = (self.environ.get("AI_GIGACHAT_PII_MASKING") or "on").strip().lower() != "off"
        return GIGACHAT_LIVE_ALLOWED and masked and self._has("AI_GIGACHAT_AUTH_KEY", "AI_GIGACHAT_API_KEY")

    @property
    def yandexgpt(self) -> bool:
        return self._has("AI_YANDEX_API_KEY", "AI_YANDEX_IAM_TOKEN") and self._has("AI_YANDEX_FOLDER_ID")

    @property
    def speechkit(self) -> bool:  # Yandex SpeechKit: API key or IAM token (folder id optional)
        return self._has("AI_YANDEX_API_KEY", "AI_YANDEX_IAM_TOKEN")

    def llm_live(self, name: str) -> bool:
        if name not in self.llms:
            return False
        return {"openai": self.openai, "anthropic": self.anthropic, "gigachat": self.gigachat,
                "yandexgpt": self.yandexgpt}[name]

    @property
    def speech(self) -> str:
        """STT/TTS provider of the edition if its key is set, else mock."""
        name = EDITION_SPEECH[self.edition]
        live = self.speechkit if name == "yandex_speechkit" else self.openai
        return name if live else "mock"

    @property
    def stt_live(self) -> bool:
        return self.speech != "mock"

    @property
    def tts_live(self) -> bool:
        return self.speech != "mock"

    def status(self) -> dict:
        llm = {n: "live" if self.llm_live(n) else "demo" for n in self.llms}
        return {"llm": llm, "stt": "live" if self.stt_live else "demo", "tts": "live" if self.tts_live else "demo",
                "speech": self.speech}

    def client(self, llm: str) -> Client:
        """Client for one request: chosen LLM if live (else mock), the edition's STT+TTS if live (else mock)."""
        env = dict(self.environ)
        env["AI_LLM_PROVIDER"] = llm if self.llm_live(llm) else "mock"
        env["AI_STT_PROVIDER"] = env["AI_TTS_PROVIDER"] = self.speech
        return from_env(Env("AI_", env))
