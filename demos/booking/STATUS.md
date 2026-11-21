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
- RU edition move: `deploy.sh booking-ru --tag sha-…` on freelance-ru + A record `booking.demo` → RU host; then `deploy.sh rm booking` on DO.
- accounts (RU) is wired: `server/accounts.go` (mode → consume → live YooKassa TEST shop, refund on failure, fail-closed to
  the mock; webhook/refund routed by `pay_live`), stub sign-in button now (`ACCOUNTS_UI=stub`). To go live when auth is up:
  `ACCOUNTS_UI=widget`, `ACCOUNTS_PROVIDER=http`, keys `AUTH_PUBLIC_KEY`, `AUTH_SVC_BOOKING`, `YOOKASSA_TEST_SHOP_ID/SECRET_KEY`
  (see `infra/compose/booking-ru/env.keys`). Test: `TestRuLiveModeViaAccounts`.
- Optional live bots: Telegram (BotFather token) and Max (token) via orchestrator / `sec`.
- Real Max client: verify the Max bridge script for Mini Apps (window.WebApp) before a live Max launch.
