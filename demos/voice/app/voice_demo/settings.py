"""Demo settings from env (VOICE_*). Provider keys are read by the ai module itself (AI_*)."""
from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Mapping

HERE = Path(__file__).resolve().parent
DEFAULT_SAMPLES = HERE.parent.parent / "samples"
DEFAULT_STATIC = HERE.parent.parent / "static"


def _int(env: Mapping[str, str], key: str, default: int) -> int:
    try:
        return int(env.get(key, "") or default)
    except ValueError:
        return default


@dataclass
class Settings:
    max_audio_sec: int = 180          # live mode: longest accepted audio
    max_upload_mb: int = 10           # any mode: largest accepted upload
    ip_per_hour: int = 6              # live mode: processing runs per IP per hour
    daily_live: int = 120             # live mode: processing runs per day for everyone (spend cap)
    trust_proxy: bool = True          # take the client IP from X-Forwarded-For (behind Caddy)
    samples_dir: Path = DEFAULT_SAMPLES
    static_dir: Path = DEFAULT_STATIC
    telegram_token: str = ""          # optional live bot (long polling)
    telegram_api_base: str = "https://api.telegram.org"  # TELEGRAM_API_BASE: RU host reaches Telegram via tg-relay
    edition: str = "intl"             # EDITION=ru|intl: language, providers, samples, link to the other edition
    live_allowed: bool = False        # VOICE_LIVE: ru → on by default (Russian stack), intl → off for now
    other_url: str = ""               # link to the other edition (default from DEMO_DOMAIN)
    accounts_widget: bool = False     # VOICE_ACCOUNTS_WIDGET=1: load auth's widget.js (sign-in button); 0 = stub button
    auth_public_url: str = ""         # public auth URL for the widget (default https://auth.<DEMO_DOMAIN>)
    environ: Mapping[str, str] = field(default_factory=lambda: dict(os.environ))

    @property
    def lang(self) -> str:
        return "ru" if self.edition == "ru" else "en"

    @classmethod
    def from_env(cls, env: Mapping[str, str] | None = None) -> "Settings":
        env = dict(os.environ if env is None else env)
        edition = (env.get("EDITION") or "intl").strip().lower()
        if edition not in ("ru", "intl"):
            raise ValueError("EDITION must be ru or intl")
        live = (env.get("VOICE_LIVE") or "").strip().lower() or ("1" if edition == "ru" else "0")
        live = live in ("1", "true", "yes", "on")
        domain = (env.get("DEMO_DOMAIN") or "demo.sedlab.dev").strip()
        other = env.get("VOICE_OTHER_URL") or (f"https://voice-en.{domain}" if edition == "ru" else f"https://voice.{domain}")
        return cls(
            accounts_widget=(env.get("VOICE_ACCOUNTS_WIDGET") or "0").strip().lower() in ("1", "true", "yes", "on"),
            auth_public_url=(env.get("VOICE_AUTH_PUBLIC_URL") or f"https://auth.{domain}").rstrip("/"),
            edition=edition,
            live_allowed=live,
            other_url=other,
            max_audio_sec=_int(env, "VOICE_MAX_AUDIO_SEC", 180),
            max_upload_mb=_int(env, "VOICE_MAX_UPLOAD_MB", 10),
            ip_per_hour=_int(env, "VOICE_IP_PER_HOUR", 6),
            daily_live=_int(env, "VOICE_DAILY_LIVE", 120),
            trust_proxy=env.get("VOICE_TRUST_PROXY", "1") not in ("0", "false", "no"),
            samples_dir=Path(env.get("VOICE_SAMPLES_DIR") or DEFAULT_SAMPLES),
            static_dir=Path(env.get("VOICE_STATIC_DIR") or DEFAULT_STATIC),
            telegram_token=env.get("VOICE_TELEGRAM_TOKEN", "").strip(),
            telegram_api_base=(env.get("TELEGRAM_API_BASE") or "https://api.telegram.org").strip().rstrip("/"),
            environ=env,
        )
