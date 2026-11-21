# STATUS — S3 booking (2026-11-21)

## Done
- Go backend (SQLite, ~10 MB RAM, 14 MB image) + Svelte Mini App, owner admin, browser Telegram emulator, mock checkout page.
- Payments: Telegram Stars (telegram-core create_invoice_link / pre-checkout / refunds; emulator → mock_stars_payment)
  and card prepayment via payments (YooKassa; mock with persistent state). Holds 15 min, refunds on cancel ≥24 h.
- Notifications through notify (confirmation with Mini App button, owner alerts, reminders 24 h / 2 h, once each).
- Max: `server/messenger_max.go` on max-core (launch data, sessions, webhook `/max/webhook`, open_app button);
  emulator switch Telegram / Max; card-only prepayment on Max. All messages (both messengers) go through notify.
- Tests: `server` go test (slots, deposits, YooKassa flow, Stars flow, hold expiry, reminders, auth/roles, webhook secret+dedupe);
  `e2e/flow.mjs` Playwright run through the emulator (Stars EN, card RU, Max RU card, reminder, admin). Go: + Max flow test.
- Media: `media/01–06*.png`, `demo.mp4` / `demo.gif` (~50 s), `07-max-confirm-ru.png`. README (EN/RU), portfolio.md (EN/RU).
- CI: `.github/workflows/build-booking.yml` (tests → private GHCR image). Compose: `infra/compose/booking`.

- **Deployed:** https://booking.demo.sedlab.dev (sha-1c3850e, freelance-1, ~8 MB RAM, mem_limit 96m, uid 1000, read-only FS).
  check-demos green; browser e2e against prod green twice (Telegram Stars, Telegram card, Max card, reminder, admin).
- Personal data: the public demo stores no phone/username/real names; daily reset (DEMO_RESET). README: 152-FZ note
  (RU clients → Russian VPS + Russian S3).

## Left
- Optional live bots: Telegram (BotFather token) and Max (token) via orchestrator / `sec`.
- Real Max client: verify the Max bridge script for Mini Apps (window.WebApp) before a live Max launch.
