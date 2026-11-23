# STATUS: S3 booking (2026-11-21)

## Editions (owner's decision): one code base, `EDITION=intl|ru`
| | intl → https://booking-en.demo.sedlab.dev (freelance-1) | ru → https://booking.demo.sedlab.dev (freelance-ru, after the move) |
|---|---|---|
| UI | English | Russian |
| Messengers | Telegram | Telegram + Max |
| Deposit | Stars + card (Stripe, mock) | card (YooKassa, mock) |
| PD | `PD_ALLOWED=false`: no phone/names, live bots refused | `PD_ALLOWED=true` on the RU host |
| compose | `infra/compose/booking-en` | `infra/compose/booking-ru` (`x-fl-pd: true`) |
Until the RU host exists, `infra/compose/booking` (DO) serves the RU edition with `PD_ALLOWED=false`.

## Done
- Go backend (SQLite, ~10 MB RAM, 14 MB image) + Svelte Mini App, owner admin, browser Telegram emulator, mock checkout page.
- Payments: Telegram Stars (telegram-core create_invoice_link / pre-checkout / refunds; emulator → mock_stars_payment)
  and card prepayment via payments (YooKassa; mock with persistent state). Holds 15 min, refunds on cancel ≥24 h.
- Notifications through notify (confirmation with Mini App button, owner alerts, reminders 24 h / 2 h, once each).
- Max: `server/messenger_max.go` on max-core (launch data, sessions, webhook `/max/webhook`, open_app button);
  emulator switch Telegram / Max; card-only prepayment on Max. All messages (both messengers) go through notify.
- Tests: `server` go test (slots, deposits, YooKassa flow, Stars flow, hold expiry, reminders, auth/roles, webhook secret+dedupe);
  `e2e/flow.mjs` Playwright run through the emulator (Stars EN, card RU, Max RU card, reminder, admin). Go: + Max flow test.
- Media: `media/01–06*.png`, per edition: `*-en.png` + `demo-en.mp4/gif`, `*-ru.png` + `demo-ru.mp4/gif`. README (EN/RU), portfolio.md (EN/RU).
- CI: `.github/workflows/build-booking.yml` (tests → private GHCR image). Compose: `infra/compose/booking`.

- **Deployed:** https://booking.demo.sedlab.dev (sha-1c3850e, freelance-1, ~8 MB RAM, mem_limit 96m, uid 1000, read-only FS).
  check-demos green; browser e2e against prod green twice (Telegram Stars, Telegram card, Max card, reminder, admin).
- Personal data: the public demo stores no phone/username/real names; daily reset (DEMO_RESET). README: 152-FZ note
  (RU clients → Russian VPS + Russian S3).

- Legal v2 (2a131ec): RU bots ask consent bot-v1 ("Согласен, начать", callback button) before storing anything, the Mini App
  API returns 403 consent_required until then; /delete wipes the chat's data; background purge ≤ 7 days (both editions);
  real Telegram users' names are never stored; INTL welcome carries "Demo: use test data only…". Tests: TestConsentDeleteRetention.

- 2026-11-21 night: sha-74ce01a deployed. booking-ru on freelance-ru (5.35.4.25), checked from inside (pre-dns): /health 200,
  config ru, consent 403 → 200, /delete, ~4 MB RAM; accounts http to auth on the host. booking (DO) = interim RU edition.
  booking-en stays as deployed (sha-214b615): the owner decided not to spend effort on INTL editions.

## Left
- RU edition moved: booking-ru on freelance-ru, A record booking.demo → RU host (23.11); interim DO copy removed.
- Live integrations wait for keys/owner: see «Чек-лист включения живых интеграций» below.
- Real Max client: verify the Max bridge script for Mini Apps (window.WebApp) on the first live Max launch.

## 2026-11-23: понятный интерфейс и честные интеграции (аудит, пункт А)
- Демо-страница без техники: заголовок и шаги для заказчика, блок «Что получает салон», детали в свёрнутом «Для специалистов».
- Честность: подпись «имитация окна мессенджера»; оплата названа «Учебная оплата (имитация)», без названий провайдеров и без карты.
- Убраны: значок sedlab · demo, стек-чипы, /start и /my (кнопки «Меню», «Мои записи»), заглушка входа (вернётся с виджетом).
- Админка RU: без полей EN, «Ждут оплаты». e2e RU и медиа *-ru обновлены. INTL не трогали.
- Тестовая ЮKassa открыта всем без входа (решение владельца 23.11): лимит 3 оплаты в день с IP и 60 в день всего
  (`LIVE_PER_IP_DAY`, `LIVE_PER_DAY`; IP только солёным хешем в памяти). Включится, когда в sec появятся
  `PAYMENTS_YOOKASSA_SHOP_ID/SECRET_KEY`. Адрес уведомлений: https://booking.demo.sedlab.dev/payments/webhook. Тест `TestRuLiveAnonymousByIP`.

## Чек-лист включения живых интеграций (booking-ru, https://booking.demo.sedlab.dev)
Ключи в `~/freelance/infra/compose/booking-ru/env.keys` уже прописаны как необязательные (`?`): нет в sec, значит
пропуск. Включение: положить ключ в sec (скоуп freelance), затем
`~/freelance/infra/deploy.sh booking-ru --host freelance-ru --tag sha-<последний образ>` (check-demos идёт сам).
Код готов, пересобирать образ не нужно. Проверка после каждого шага: `curl -s https://booking.demo.sedlab.dev/api/config`.

1. **Robokassa, тестовый магазин sedlabbooking** (пароли в sec уже есть: BOOKING_ROBOKASSA_PASSWORD1/2).
   - Владелец в ЛК: включить способы оплаты для тестовых платежей (сейчас страница оплаты «500 No payment methods
     available»). Уже вписано: SHA256, Result `https://booking.demo.sedlab.dev/payments/webhook` POST,
     Success `/payment/success` GET, Fail `/payment/fail` GET.
   - Включить: в compose.yml удалить строку `LIVE_PROVIDER: "off"`, передеплоить.
   - Проверка: `live_available: true, live_provider: Robokassa`; запись в эмуляторе, оплата на странице Robokassa,
     запись становится «Подтверждена» (по Result URL).
2. **ЮKassa, тестовый магазин.** sec: `PAYMENTS_YOOKASSA_SHOP_ID`, `PAYMENTS_YOOKASSA_SECRET_KEY`. В ЛК ЮKassa
   HTTP-уведомления на `https://booking.demo.sedlab.dev/payments/webhook` (payment.succeeded, payment.canceled,
   refund.succeeded). Если в sec есть и Robokassa, в compose.yml `LIVE_PROVIDER: yookassa`. Передеплоить; проверка:
   `live_provider: ЮKassa`, тестовая карта 5555 5555 5555 4477.
3. **Max-бот.** sec: `BOOKING_MAX_BOT_TOKEN`; compose.yml: раскомментировать `MAX_BOT_USERNAME: <имя бота>`.
   Передеплоить: токен сам включает бота, подписка на `/max/webhook` ставится при старте (CA Минцифры смонтирован).
   Проверка: /start → «Согласен, начать» → «Записаться» открывает мини-приложение в Max.
4. **Telegram-бот** через tg-relay (`TELEGRAM_API_BASE` и `NOTIFY_TELEGRAM_API_BASE` уже в compose.yml, релей
   отвечает с RU-хоста). BotFather: бот, токен в sec как `BOOKING_TELEGRAM_BOT_TOKEN`; Menu Button / Web App URL
   `https://booking.demo.sedlab.dev/app/`. Передеплоить: вебхук `/tg/webhook` ставится сам. Только тестовые данные,
   имена пользователей Telegram не сохраняются. Проверка: в логе `"telegram_live":true` и `webhook set`.
5. **Вход через accounts** (лимиты по аккаунту): когда auth.demo доступен снаружи, `ACCOUNTS_UI: widget`.

Не относится к booking: Яндекс Облако, Битрикс24 (в этом демо их нет).
