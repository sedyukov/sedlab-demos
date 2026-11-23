from __future__ import annotations

from sedlab_crm import Code, Mock, ModuleError
from test_app import make


def crm_client(env=None):
    c = make({"CRM_PROVIDER": "bitrix24", "CRM_BITRIX24_WEBHOOK_URL": "https://demo.bitrix24.ru/rest/1/x/", **(env or {})})
    c.app.state.crm.crm = Mock()  # same interface as the live Bitrix24 provider, no network
    return c


def test_crm_off_without_key_and_page_stays_honest():
    c = make({"CRM_PROVIDER": "bitrix24"})  # no webhook URL in sec → feature off
    assert c.get("/api/config").json()["crm"] is None
    assert c.post("/api/crm/tasks", json={"sample": "voice-note-ru"}).status_code == 404
    assert make().get("/api/config").json()["crm"] is None


def test_crm_sends_sample_tasks_with_deadline_and_idempotency():
    c = crm_client()
    assert c.get("/api/config").json()["crm"] == {"provider": "bitrix24", "name": "Битрикс24"}
    h = {"X-Forwarded-For": "5.5.5.5"}
    r = c.post("/api/crm/tasks", json={"sample": "planning-ru"}, headers=h)
    assert r.status_code == 200, r.json()
    tasks = r.json()["tasks"]
    assert len(tasks) == 4 and all(x["title"].startswith("[демо] ") for x in tasks)
    assert any(x["deadline"].endswith("+07:00") for x in tasks)
    again = c.post("/api/crm/tasks", json={"sample": "planning-ru"}, headers=h).json()["tasks"]
    assert [x["id"] for x in again] == [x["id"] for x in tasks]  # repeat click: no duplicates in the portal


def test_crm_only_samples_and_limited():
    c = crm_client()
    assert c.post("/api/crm/tasks", json={"sample": "nope"}).status_code == 400
    h = {"X-Forwarded-For": "6.6.6.6"}
    codes = [c.post("/api/crm/tasks", json={"sample": "voice-note-ru"}, headers=h).status_code for _ in range(4)]
    assert codes == [200, 200, 200, 429]


def test_crm_failure_is_a_plain_russian_message():
    c = crm_client()

    class Down:
        def create_task(self, **kw):
            raise ModuleError(Code.UNAVAILABLE, "down", "bitrix24")

    c.app.state.crm.crm = Down()
    r = c.post("/api/crm/tasks", json={"sample": "voice-note-ru"})
    assert r.status_code == 502 and "Битрикс24" in r.json()["message"]
