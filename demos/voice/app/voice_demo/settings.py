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
    environ: Mapping[str, str] = field(default_factory=lambda: dict(os.environ))

    @classmethod
    def from_env(cls, env: Mapping[str, str] | None = None) -> "Settings":
        env = dict(os.environ if env is None else env)
        return cls(
            max_audio_sec=_int(env, "VOICE_MAX_AUDIO_SEC", 180),
            max_upload_mb=_int(env, "VOICE_MAX_UPLOAD_MB", 10),
            ip_per_hour=_int(env, "VOICE_IP_PER_HOUR", 6),
            daily_live=_int(env, "VOICE_DAILY_LIVE", 120),
            trust_proxy=env.get("VOICE_TRUST_PROXY", "1") not in ("0", "false", "no"),
            samples_dir=Path(env.get("VOICE_SAMPLES_DIR") or DEFAULT_SAMPLES),
            static_dir=Path(env.get("VOICE_STATIC_DIR") or DEFAULT_STATIC),
            telegram_token=env.get("VOICE_TELEGRAM_TOKEN", "").strip(),
            environ=env,
        )
