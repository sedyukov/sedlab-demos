"""Spend control for live mode: per-IP token bucket, a daily cap for everyone, audio length from the WAV header."""
from __future__ import annotations

import struct
import threading
import time
from datetime import datetime, timezone
from typing import Callable

from sedlab_ai.kit.ratelimit import KeyedLimiter


class Limits:
    def __init__(self, ip_per_hour: int, daily: int, clock: Callable[[], float] = time.monotonic,
                 today: Callable[[], str] = lambda: datetime.now(timezone.utc).strftime("%Y-%m-%d")) -> None:
        n = max(1, ip_per_hour)
        self._ip = KeyedLimiter(n / 3600.0, n, clock)
        self.daily = daily
        self._today = today
        self._day = ""
        self._used = 0
        self._lock = threading.Lock()

    def check(self, ip: str) -> str | None:
        """Consume one live run for ip. Returns None when allowed, else "ip" or "daily"."""
        with self._lock:
            d = self._today()
            if d != self._day:
                self._day, self._used = d, 0
            if self._used >= self.daily:
                return "daily"
        if not self._ip.allow(ip):
            return "ip"
        with self._lock:
            self._used += 1
        return None

    def remaining_today(self) -> int:
        with self._lock:
            return self.daily if self._today() != self._day else max(0, self.daily - self._used)


def wav_duration(data: bytes) -> float | None:
    """Duration of a RIFF/WAVE PCM file in seconds, or None if it is not a parseable WAV."""
    if len(data) < 12 or data[:4] != b"RIFF" or data[8:12] != b"WAVE":
        return None
    pos, byte_rate = 12, 0
    while pos + 8 <= len(data):
        cid, size = data[pos:pos + 4], struct.unpack("<I", data[pos + 4:pos + 8])[0]
        body = pos + 8
        if cid == b"fmt " and size >= 16:
            byte_rate = struct.unpack("<I", data[body + 8:body + 12])[0]
        elif cid == b"data":
            if not byte_rate:
                return None
            size = min(size, len(data) - body)  # streaming writers put 0xFFFFFFFF here
            return size / byte_rate
        pos = body + size + (size & 1)
    return None
