# STATUS: S5 «Заявка → CRM» (app: leads), 2026-11-21

**Демо:** RU-версия https://leads.demo.sedlab.dev (пока объединённая страница на freelance-1, до переезда на RU-хост),
EN-версия https://leads-en.demo.sedlab.dev (EDITION=intl, freelance-1).

## Сделано
- **Две редакции** (решение владельца): одна кодовая база, `EDITION=ru|intl` (пусто = старая объединённая страница).
  ru: только русский язык (в т.ч. лог, бейджи, уведомления), amoCRM или Битрикс24 (переключатель, профиль `ru_b24`, env `RU_B24_*`),
  ЮKassa, таблица с подписью «Яндекс Диск» (mock до провайдера s2), уведомления Telegram + Max, ссылка «English version».
  intl: только английский, HubSpot + Stripe + Google Sheets + Telegram/email, ссылка «Русская версия». Чужой профиль → 400.
  Compose: `infra/compose/leads-en` (freelance-1) и `infra/compose/leads-ru` (freelance-ru, `x-fl-pd: true`, accounts env).
  Тесты: `TestEditionRU`, `TestEditionIntl`, `TestRUEditionLogIsRussian`; e2e: `EDITION=ru STACKS=ru,ru_b24`, `EDITION=intl STACKS=intl`.
- **Приложение** (Go, stdlib + модули crm, payments, sheets-storage, notify). Цепочка: форма → CRM (upsert контакта + сделка) →
  Google Sheet → Telegram/email → ссылка на оплату → подписанный вебхук → сделка Won/Lost, примечание, обновлённая строка, сообщение «оплачено».
  Вебхуки CRM (deal.created, deal.updated) тоже доставляются и проверяются.
- **Живой лог** через SSE (backlog, Last-Event-ID, пинги). У каждого посетителя своя песочница: свои mock-CRM, таблица, платежи
  и уведомления, LRU-лимит 2000 сессий.
- **Переключатели:** RU (amoCRM + ЮKassa + Sheets + Telegram, ₽) и International (HubSpot + Stripe + Sheets + Telegram + email, $); язык RU/EN.
- **Видимая надёжность:** режим «сбои API» (CRM 503, Sheets 429 с Retry-After, Telegram 502 и таймаут через `metadata.mock_fail` модуля notify)
  показывает ретраи с backoff. Идемпотентность: submission_id, ключи на сделку, строку, платёж и уведомление. «Повторить вебхук» даёт duplicate,
  «поддельный вебхук» получает 401.
- **Антиспам:** лимит по IP (3 сразу, затем 5 в минуту, `SUBMIT_PER_MIN`), глобальный лимит, honeypot, ловушка по времени заполнения,
  25 заявок на сессию, лимит размера тела. **Автоочистка:** сессии без активности больше 6 ч удаляются каждые 10 мин, полная очистка
  раз в сутки (`WIPE_HOUR_UTC`, по умолчанию 03:00).
- **Виджет** `web/widget.js`: без зависимостей, Shadow DOM, автомонтирование по `data-leads-widget`, CORS через `ALLOWED_ORIGINS`. Готов для S7.
- **Live-режим** через env на каждый сервис (`RU_CRM_PROVIDER=amocrm`, `INTL_PAYMENTS_PROVIDER=stripe`, `RU_SHEETS_PROVIDER=google_sheets`
  + `RU_SHEETS_SHEET_ID`, `RU_NOTIFY_TELEGRAM_PROVIDER=telegram` + `RU_NOTIFY_TELEGRAM_TO` и т. д.). Live-провайдеры общие, mock создаётся на сессию.
- **Без «контактов» на экране** (модерация Kwork и FL.ru): страница и SSE видят email и телефоны только как псевдонимы
  `client-N (email|тел.)`, получатель уведомлений показан как «Менеджер (Telegram)», заглушка сайта `example-shop`, тестовая карта `•••• 4242`,
  id шагов выводятся как `crm › create_deal`. Тест `TestPageShowsNoContacts`.
- **152-ФЗ и юрисдикция:** публичное демо (DO, Франкфурт) не принимает реальные контакты. `Guard.SandboxContacts` включён по умолчанию
  и выключается через `ACCEPT_REAL_CONTACTS=1` для клиентских установок. Полей email и телефона нет, присланное отбрасывается,
  галочки согласия нет (обрабатывать нечего). Тест `TestSandboxContactsDiscardTypedContacts`.
- **Тесты:** `go test -race`, 18 сценариев (полный поток RU и INTL, дубль формы, повтор и подделка вебхука, отмена оплаты, ретраи и Retry-After,
  honeypot, rate limit, валидация, изоляция сессий, SSE, сброс и очистка, LRU, виджет и CORS, отсутствие контактов, режим песочницы).
  `e2e/e2e.mjs` (Playwright): 22 проверки в браузере для обоих стеков, включая ширину 390 px. Всё зелёное, в том числе на проде.
- **Образ:** distroless, около 9 МБ, около 30 МБ RAM под нагрузкой e2e, `mem_limit 128m`, GOMEMLIMIT 90MiB.
- **CI:** `.github/workflows/build-leads.yml`: test (vet + race), затем build в `ghcr.io/sedyukov/leads:sha-xxxxxxx` (private,
  с проверками видимости пакета от s2-modules).
- **Infra:** `~/freelance/infra/compose/leads/` (compose, site.caddy с `flush_interval -1` для SSE, health.url, env.keys с перечнем live-ключей).
- **Деплой:** `infra/deploy.sh leads --tag sha-<7>`. e2e на проде запускать по одному стеку: `BASE=https://leads.demo.sedlab.dev STACKS=ru node e2e/e2e.mjs`,
  потом `STACKS=intl` (лимит прода: 3 заявки сразу, дальше 1 в 12 с с одного IP).
- **Документы и медиа:** README.md (EN), README.ru.md с утверждёнными ценами, portfolio.md (RU и EN), 12 скриншотов, 2 видео MP4 по 35–40 с
  и 2 GIF (RU и EN). Тексты без длинного тире.

## Осталось
- Переезд RU: `infra/deploy.sh leads-ru --host freelance-ru --tag sha-<7>`, затем A-запись `leads.demo → IP freelance-ru` в Cloudflare,
  `check-demos.sh leads-ru --host freelance-ru`, потом `deploy.sh rm leads` на freelance-1 и удалить `infra/compose/leads`.
- Таблица RU: провайдер Яндекс Диск (XLSX) от s2 → подключить в профилях `ru` и `ru_b24` (env `RU_SHEETS_*`, `RU_B24_SHEETS_*`).
- Битрикс24 live (crm 1fcaa49) готов к включению: `RU_B24_CRM_PROVIDER=bitrix24`, `RU_B24_CRM_BITRIX24_WEBHOOK_URL` (секрет), `..._ENTITY`, `..._CATEGORY_ID`, `..._APP_TOKEN`; тест `TestBitrix24LiveByEnv`. По умолчанию mock.
- Модуль accounts (sedlab-modules a027917): интеграция позже, вместе с сессией accounts. **На проде обязательно `ACCOUNTS_PROVIDER=http`**,
  на mock живой режим принудительно выключен.
- После подъёма RU-VPS: RU-провайдеры через ru-relay, `RU_PAYMENTS_YOOKASSA_BASE_URL=https://<relay>/yookassa/v3`,
  `RU_CRM_AMOCRM_BASE_URL=https://<relay>/amocrm/api/v4` (+ `RU_CRM_AMOCRM_SUBDOMAIN`). Код менять не нужно, модули читают эти env. Пока не включено.
- Остаточный риск: имя, компания и текст задачи остаются свободными полями, посетитель может вписать туда реальные данные. Они живут только
  в памяти до суточной очистки, в форме написано не вводить реальные данные. Если нужно строже, в демо можно убрать свободный текст.
- Живые sandbox-ключи (amoCRM trial, HubSpot dev, ЮKassa test, Stripe test, Google SA, TG-бот) по желанию; перечень env в `infra/compose/leads/env.keys`.
