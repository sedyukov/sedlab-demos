"""Which providers are live. All construction goes through the ai module's own from_env factory:
the demo only flips AI_LLM_PROVIDER / AI_STT_PROVIDER / AI_TTS_PROVIDER per request."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping

from sedlab_ai import Client, Env, from_env

LLMS = ("gigachat", "yandexgpt", "openai", "anthropic")  # RU providers first: they are the default (152-FZ)


def default_llm(p: "Providers") -> str:
    """First live provider, Russian ones preferred; gigachat when nothing is live."""
    return next((n for n in LLMS if p.llm_live(n)), LLMS[0])


@dataclass
class Providers:
    environ: Mapping[str, str]

    def _has(self, *keys: str) -> bool:
        return any((self.environ.get(k) or "").strip() for k in keys)

    @property
    def openai(self) -> bool:  # key, or an OpenAI-compatible endpoint (Ollama, vLLM...)
        return self._has("AI_OPENAI_API_KEY", "AI_OPENAI_BASE_URL")

    @property
    def anthropic(self) -> bool:
        return self._has("AI_ANTHROPIC_API_KEY")

    @property
    def gigachat(self) -> bool:
        return self._has("AI_GIGACHAT_AUTH_KEY")

    @property
    def yandexgpt(self) -> bool:
        return self._has("AI_YANDEX_API_KEY", "AI_YANDEX_IAM_TOKEN") and self._has("AI_YANDEX_FOLDER_ID")

    def llm_live(self, name: str) -> bool:
        return {"openai": self.openai, "anthropic": self.anthropic, "gigachat": self.gigachat,
                "yandexgpt": self.yandexgpt}.get(name, False)

    @property
    def speechkit(self) -> bool:  # Yandex SpeechKit: API key or IAM token (folder id optional)
        return self._has("AI_YANDEX_API_KEY", "AI_YANDEX_IAM_TOKEN")

    @property
    def speech(self) -> str:
        """STT/TTS provider: Russian SpeechKit first (152-FZ), then OpenAI, else mock."""
        return "yandex_speechkit" if self.speechkit else ("openai" if self.openai else "mock")

    @property
    def stt_live(self) -> bool:
        return self.speech != "mock"

    @property
    def tts_live(self) -> bool:
        return self.speech != "mock"

    def status(self) -> dict:
        llm = {}
        for n in LLMS:
            llm[n] = "live" if self.llm_live(n) else "demo"
        return {"llm": llm, "stt": "live" if self.stt_live else "demo", "tts": "live" if self.tts_live else "demo",
                "speech": self.speech}

    def client(self, llm: str) -> Client:
        """Client for one request: chosen LLM if live (else mock), SpeechKit / OpenAI STT+TTS if live (else mock)."""
        env = dict(self.environ)
        env["AI_LLM_PROVIDER"] = llm if self.llm_live(llm) else "mock"
        env["AI_STT_PROVIDER"] = env["AI_TTS_PROVIDER"] = self.speech
        return from_env(Env("AI_", env))
