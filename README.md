# sedlab demos

Live, clickable demos of what I build: Telegram bots & Mini Apps, payments, CRM integrations, AI/voice, scrapers, backends.
Each demo works without real keys (sandbox/mock providers) — try it, then imagine it with your stack.

| Demo | What it shows | Status |
|---|---|---|
| _coming soon_ | Booking bot + Mini App with prepayment (YooKassa / Stripe) | 🛠 |
| _coming soon_ | AI voice bot: transcription, summaries, TTS replies | 🛠 |
| _coming soon_ | Lead form → CRM (amoCRM / HubSpot) → Google Sheets → Telegram | 🛠 |

**Built from reusable modules** with one contract and implementations in Go and Python
(telegram-core, payments, crm, ai, sheets-storage). Unified errors, retries, rate limiting and idempotent webhooks are built in.
The module source is private; its contract tests run in this repo's CI.

Layout: `demos/<name>/` — demo apps · `site/` — showcase site · `.github/workflows/` — CI.

---

**По-русски:** живые демо — ТГ-боты и мини-аппы, платежи (ЮKassa/Stripe), CRM (amoCRM/HubSpot), AI и голос, интеграции.
Работают без ключей (sandbox/mock). Собраны из переиспользуемых модулей на Go и Python.
