"""RU edition: real "send the tasks to the CRM" (module sedlab_crm, create_task). Bitrix24 or amoCRM test portal.

Honest by construction: without CRM_PROVIDER + its key the feature is off and the page says the demo has no export.
Only the tasks of a ready-made sample go to the CRM (fictional names, no visitor text), with an idempotency key per
visitor, sample and day, under a small per-IP and daily limit, so the test portal does not fill up with spam.
"""
from __future__ import annotations

import logging
import time
from typing import Any, Mapping

from sedlab_crm import Env as CrmEnv
from sedlab_crm import ModuleError
from sedlab_crm import from_env as crm_from_env

from .limits import Limits

log = logging.getLogger("voice.crm")

CRM_NAMES = {"bitrix24": "Битрикс24", "amocrm": "amoCRM"}
TZ_OFFSET = "+07:00"  # deadlines at 18:00 Tomsk time (the portal owner's zone)


class CrmExport:
    def __init__(self, environ: Mapping[str, str], edition: str, per_hour: int = 3, daily: int = 40) -> None:
        self.provider, self.name, self.crm = "", "", None
        self.limits = Limits(per_hour, daily)
        p = (environ.get("CRM_PROVIDER") or "").strip().lower()
        if edition != "ru" or p not in CRM_NAMES:
            return
        try:
            self.crm = crm_from_env(CrmEnv("CRM_", dict(environ)))
        except ModuleError as e:  # a missing key keeps the feature off, the demo stays up
            log.warning("crm config error: %s; export is off", e.code.value)
            return
        self.provider, self.name = p, CRM_NAMES[p]

    @property
    def enabled(self) -> bool:
        return self.crm is not None

    def config(self) -> dict[str, str] | None:
        return {"provider": self.provider, "name": self.name} if self.enabled else None

    def send(self, visitor: str, sample_id: str, sample: dict[str, Any]) -> list[dict[str, str]]:
        """Creates one CRM task per sample task. Raises ModuleError from the CRM (caller maps it to a message)."""
        title = sample["title"].get("ru") or sample["title"].get("en") or sample_id
        day = time.strftime("%Y-%m-%d")
        out = []
        for i, x in enumerate(sample.get("tasks") or []):
            desc = (f"Задача из демо «Голосовой ИИ-ассистент» (пример «{title}»). Тестовые данные."
                    + (f" Ответственный в записи: {x['owner']}." if x.get("owner") else ""))
            t = self.crm.create_task(title="[демо] " + x["title"], description=desc, deadline=x.get("due") or "",
                                     tz_offset=TZ_OFFSET, idempotency_key=f"voice:{sample_id}:{day}:{visitor}:{i}")
            out.append({"id": t.id, "title": t.title, "deadline": t.deadline, "url": t.url})
        return out
