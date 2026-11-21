# Тексты для портфолио на биржах (без ссылок и контактов)

## RU — заголовок
Заявка с сайта → amoCRM → Google Таблица → Telegram + оплата ЮKassa с автообновлением сделки

## RU — описание
Демо-проект интеграции «под ключ». Форма на сайте (встраиваемый JS-виджет) создаёт контакт и сделку в amoCRM,
добавляет строку в Google Таблицу и присылает уведомление в Telegram отделу продаж. Клиент получает ссылку на оплату ЮKassa.
После оплаты вебхук переводит сделку в «Успешно», добавляет в неё примечание и обновляет строку в таблице.

Что внутри:
• живой лог событий на странице: видно каждый шаг в реальном времени;
• ретраи с backoff при сбоях API (503, 429 с Retry-After, таймауты): заявка не теряется;
• идемпотентность: двойной клик или повтор запроса не создают дублей, повторный вебхук игнорируется;
• проверка подписи вебхуков: поддельное «оплачено» отклоняется;
• антиспам: лимиты по IP, honeypot, ловушка для ботов;
• второй вариант связки для зарубежных клиентов: HubSpot + Stripe + email.

Стек: Go, Server-Sent Events, Docker, Caddy. Модули CRM, платежей, таблиц и уведомлений переиспользуются,
поэтому вместо amoCRM можно подключить Битрикс24, а вместо ЮKassa — Т-Банк, Робокассу или CloudPayments.
Срок типового проекта — 2–5 дней.

## EN — title
Website lead form → HubSpot → Google Sheets → Telegram, plus Stripe payments that update the deal

## EN — description
A demo of a turnkey integration. A website form (embeddable JS widget) creates a contact and a deal in HubSpot, appends a row to
Google Sheets and notifies the sales team in Telegram and by email. The customer gets a Stripe payment link. When they pay, the
webhook moves the deal to Won, adds a note and updates the sheet row.

Inside:
• a live event log on the page that shows every step in real time;
• retries with backoff on API failures (503, 429 with Retry-After, timeouts), so no lead is lost;
• idempotency: double clicks and network retries never create duplicates, and a replayed webhook is ignored;
• webhook signature checks, so a forged "paid" event is rejected;
• anti-spam: per-IP rate limits, a honeypot field and a bot trap;
• the same flow for Russian clients: amoCRM + YooKassa.

Stack: Go, Server-Sent Events, Docker, Caddy. The CRM, payments, sheets and notification modules are reusable, so other CRMs
(e.g. Pipedrive) and payment providers can be added to the same flow. Typical delivery time: 2–5 days.

## Медиа для загрузки
- Обложка: `media/05-deal-won-ru.png` (RU) / `media/05-deal-won-intl.png` (EN)
- Ещё: `01-lead-flow-*`, `02-retries-*`, `03-sheet-*`, `06-webhooks-*`
- Видео: `media/demo-ru.mp4` / `media/demo-intl.mp4` (~40 с), GIF: `media/demo-*.gif`
