"""Body and query validation: the same rules and the same order of problems as go/validate.go (BEHAVIOR.md)."""
from __future__ import annotations

import json
import re
from datetime import date
from typing import Any, Mapping

from .core import validation_error

MAX_INT64 = 9223372036854775807


def _reject_constant(name: str) -> Any:
    raise ValueError(name)


def parse_object(body: bytes) -> dict:
    try:
        v = json.loads(body.decode("utf-8"), parse_constant=_reject_constant)
    except (ValueError, UnicodeDecodeError):
        raise validation_error([("body", "invalid_json")])
    if not isinstance(v, dict):
        raise validation_error([("body", "invalid_type")])
    return v


_DATE = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}")
_TAG = re.compile(r"[a-z0-9-]{1,24}")
_INT = re.compile(r"-?[0-9]+")

_ABSENT = object()


def valid_date(s: str) -> bool:
    if not _DATE.fullmatch(s):
        return False
    try:
        date(int(s[0:4]), int(s[5:7]), int(s[8:10]))
    except ValueError:
        return False
    return True


class Validator:
    def __init__(self, obj: dict, patch: bool):
        self.obj, self.patch = obj, patch
        self.seen: set[str] = set()
        self.out: dict[str, Any] = {}
        self.details: list[tuple[str, str]] = []

    def add(self, field: str, issue: str) -> None:
        self.details.append((field, issue))

    def _get(self, name: str, required: bool) -> Any:
        self.seen.add(name)
        if name not in self.obj:
            if required and not self.patch:
                self.add(name, "required")
            return _ABSENT
        return self.obj[name]

    def _null(self, name: str, val: Any, nullable: bool) -> bool:
        if val is not None:
            return False
        if nullable:
            self.out[name] = None
        else:
            self.add(name, "invalid_type")
        return True

    def str(self, name: str, required: bool, nullable: bool, lo: int, hi: int) -> None:
        val = self._get(name, required)
        if val is _ABSENT or self._null(name, val, nullable):
            return
        if not isinstance(val, str):
            self.add(name, "invalid_type")
        elif len(val) < lo:
            self.add(name, "too_short")
        elif len(val) > hi:
            self.add(name, "too_long")
        else:
            self.out[name] = val

    def integer(self, name: str, required: bool, lo: int, hi: int) -> None:
        val = self._get(name, required)
        if val is _ABSENT or self._null(name, val, False):
            return
        if type(val) is not int:
            self.add(name, "invalid_type")
        elif val < lo or val > hi:
            self.add(name, "out_of_range")
        else:
            self.out[name] = val

    def enum(self, name: str, required: bool, *allowed: str) -> None:
        val = self._get(name, required)
        if val is _ABSENT or self._null(name, val, False):
            return
        if not isinstance(val, str):
            self.add(name, "invalid_type")
        elif val not in allowed:
            self.add(name, "invalid")
        else:
            self.out[name] = val

    def date(self, name: str, nullable: bool) -> None:
        val = self._get(name, False)
        if val is _ABSENT or self._null(name, val, nullable):
            return
        if not isinstance(val, str):
            self.add(name, "invalid_type")
        elif not valid_date(val):
            self.add(name, "invalid")
        else:
            self.out[name] = val

    def tags(self, name: str) -> None:
        val = self._get(name, False)
        if val is _ABSENT or self._null(name, val, False):
            return
        if not isinstance(val, list):
            self.add(name, "invalid_type")
            return
        if len(val) > 5:
            self.add(name, "too_long")
            return
        out: list[str] = []
        bad = False
        for i, it in enumerate(val):
            f = f"{name}[{i}]"
            if not isinstance(it, str):
                self.add(f, "invalid_type")
                bad = True
            elif not _TAG.fullmatch(it):
                self.add(f, "invalid")
                bad = True
            elif it not in out:
                out.append(it)
        if not bad:
            self.out[name] = out

    def unknown(self) -> None:
        for k in sorted(k for k in self.obj if k not in self.seen):
            self.add(k, "unknown")


class Query:
    """Query parameters: the first value wins (like Go's url.Values.Get)."""

    def __init__(self, params: Mapping[str, list[str]]):
        self.params = params
        self.details: list[tuple[str, str]] = []

    def add(self, field: str, issue: str) -> None:
        self.details.append((field, issue))

    def has(self, name: str) -> bool:
        return name in self.params

    def str(self, name: str) -> str | None:
        vals = self.params.get(name)
        return vals[0] if vals else None

    def int_param(self, name: str, lo: int, hi: int, default: int) -> int:
        s = self.str(name)
        if s is None:
            return default
        if not _INT.fullmatch(s):
            self.add(name, "invalid_type")
            return default
        i = int(s)
        if i < lo or i > hi:
            self.add(name, "out_of_range")
            return default
        return i

    def search(self, name: str) -> str:
        s = self.str(name)
        if s is None:
            return ""
        if len(s) > 100:
            self.add(name, "too_long")
            return ""
        return s.lower()

    def date(self, name: str) -> str:
        s = self.str(name)
        if s is None:
            return ""
        if not valid_date(s):
            self.add(name, "invalid")
            return ""
        return s

    def enum(self, name: str, default: str, *allowed: str) -> str:
        s = self.str(name)
        if s is None:
            return default
        if s in allowed:
            return s
        self.add(name, "invalid")
        return default
