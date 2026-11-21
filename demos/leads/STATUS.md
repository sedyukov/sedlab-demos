# STATUS — S5 «Заявка → CRM» (app: leads) · 2026-11-21

**Демо: https://leads.demo.sedlab.dev** — задеплоено `sha-52926b8`, e2e на проде зелёный (RU и International).

## Сделано
- **Деплой:** `infra/deploy.sh leads --tag sha-52926b8`, контейнер ~3 МБ RAM в простое, SSE через Caddy без буферизации. e2e на проде: `BASE=https://leads.demo.sedlab.dev STACKS=ru node e2e/e2e.mjs`, затем `STACKS=intl` (лимит прода — 3 заявки сразу, дальше 1 в 12 с с одного IP, поэтому стеки гоняются по отдельности).
- **Приложение** (Go, stdlib + модули crm / payments / sheets-storage / notify): форма → CRM (upsert контакта + сделка) →
  Google Sheet → Telegram/email → ссылка на оплату → подписанный вебхук → сделка Won/Lost + примечание + строка в таблице + «оплачено».
  Вебхуки CRM (deal.created/updated) тоже доставляются и проверяются.
- **Живой лог** через SSE (backlog + Last-Event-ID, пинги), у каждого посетителя изолированная песочница (свои mock-CRM,
  таблица, платежи, уведомления; LRU-лимит 2000 сессий).
- **Переключатель** RU (amoCRM + ЮKassa + Sheets + Telegram, ₽) / International (HubSpot + Stripe + Sheets + Telegram + email, $).
  Интерфейс RU/EN.
- **Видимая надёжность:** режим «сбои API» (CRM 503, Sheets 429 + Retry-After, Telegram 502/timeout через `metadata.mock_fail` модуля notify) → ретраи с backoff;
  идемпотентность (submission_id, ключи на сделку/строку/платёж/уведомление); «повторить вебхук» → duplicate; «поддельный вебхук» → 401.
- **Антиспам:** лимит по IP (3 сразу, потом 5/мин; `SUBMIT_PER_MIN`), глобальный лимит, honeypot, ловушка по времени заполнения, 25 заявок на сессию,
  лимит размера тела. **Автоочистка:** простаивающие сессии >6 ч каждые 10 мин, полная очистка раз в сутки (`WIPE_HOUR_UTC`, по умолчанию 03:00).
- **Виджет** `web/widget.js` — самостоятельный, без зависимостей, Shadow DOM, автомонтирование по `data-leads-widget`, CORS (`ALLOWED_ORIGINS`) — готов для S7.
- **Live-режим** через env на каждый сервис (`RU_CRM_PROVIDER=amocrm`, `INTL_PAYMENTS_PROVIDER=stripe`, `RU_SHEETS_PROVIDER=google_sheets` + `RU_SHEETS_SHEET_ID`,
  `RU_NOTIFY_TELEGRAM_PROVIDER=telegram` + `RU_NOTIFY_TELEGRAM_TO`…); live-провайдеры общие, mock — на сессию.
- **Тесты:** `go test -race` — 16 сценариев (полный поток RU/INTL, дубль формы, повтор/подделка вебхука, отмена оплаты, ретраи и Retry-After,
  honeypot, rate limit, валидация, изоляция сессий, SSE, сброс/очистка, LRU, виджет+CORS) — зелёные.
  `e2e/e2e.mjs` (Playwright) — 22 проверки в браузере для обоих стеков, в том числе против Docker-образа, и проверка ширины 390 px — зелёные.
- **Образ:** distroless, ~9 МБ, ~30 МБ RAM под нагрузкой e2e, `mem_limit 128m`, GOMEMLIMIT 90MiB.
- **CI:** `.github/workflows/build-leads.yml` — test (vet + race) → build → `ghcr.io/sedyukov/leads:sha-xxxxxxx` (private), по правилам SECURITY.md.
- **Infra:** `~/freelance/infra/compose/leads/` (compose, site.caddy с `flush_interval -1` для SSE, health.url, env.keys с перечнем live-ключей).
- **Документы и медиа:** README.md (EN) и README.ru.md с ценами, portfolio.md (RU/EN), 12 скриншотов, 2 видео MP4 по ~40 с и 2 GIF (RU и EN).

- **Без «контактов» на экране** (модерация Kwork/FL.ru): страница и SSE получают email/телефоны только как псевдонимы `client-N (email|тел.)`, получатели уведомлений — «Менеджер (Telegram)»; «Заполнить тестовыми данными» не вставляет email/телефон (сервер сам заводит sandbox-контакт, `demo_contact`); заглушка сайта — `example-shop`; точечные id шагов показаны как `crm › create_deal`; тестовая карта — `•••• 4242`. Тест `TestPageShowsNoContacts`. Медиа пересняты.

- **152-ФЗ / юрисдикция:** публичное демо (DO, Франкфурт) не принимает реальные контакты: `Guard.SandboxContacts` (по умолчанию включён,
  отключается `ACCEPT_REAL_CONTACTS=1` для клиентских установок). Поля email/телефон скрыты, присланное отбрасывается, галочки согласия нет —
  обрабатывать нечего. Тест `TestSandboxContactsDiscardTypedContacts`.

## Осталось
- Модуль accounts (sedlab-modules a027917): интеграция позже, вместе с сессией accounts. **На проде обязательно `ACCOUNTS_PROVIDER=http`**, на mock живой режим принудительно выключен.
- После подъёма RU-VPS: RU-провайдеры через ru-relay — `RU_PAYMENTS_YOOKASSA_BASE_URL=https://<relay>/yookassa/v3`, `RU_CRM_AMOCRM_BASE_URL=https://<relay>/amocrm/api/v4` (+ `RU_CRM_AMOCRM_SUBDOMAIN`); код менять не нужно, модули читают эти env. Пока не включено.
- Остаточный риск: имя/компания/текст задачи — свободные поля, посетитель может вписать туда реальные данные. Они живут только в памяти
  до суточной очистки; в форме прямо написано не вводить реальные данные. Если нужно строже — можно вообще не принимать свободный текст в демо.
- Для S7: у s2-modules есть TS-часть `@sedlab/crm` (submitLead, validateLead). `widget.js` пока самостоятельный, без зависимостей; его можно перевести на неё при сборке S7.
- Проверить приватность пакета ghcr.io/sedyukov/leads (оркестратор, когда починят gh).
- Живые sandbox-ключи (amoCRM trial, HubSpot dev, ЮKassa test, Stripe test, Google SA, TG-бот) — опционально; перечень env — в `infra/compose/leads/env.keys`.
  Для live-вебхуков Stripe/ЮKassa указать в кабинете URL `https://leads.demo.sedlab.dev/webhooks/payments/{ru|intl}`.
- В mock-режиме состояние живёт только в памяти, рестарт контейнера = чистая песочница. Это осознанно: демо очищается раз в сутки.
