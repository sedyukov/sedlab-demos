# STATUS: demos/auth (2026-11-21)

Готово, `go test ./...` зелёный:
- вход Max (deep link, подтверждение в боте, Mini App initData), Яндекс ID и VK ID через `accounts/go/oauth`;
- экран согласия по `legal/ui-texts.md` (3 чекбокса), страницы /privacy, /terms, /consent/demo, /consent/contact;
- токены Ed25519 и хранилище через модуль `accounts` (Store, миграции, storetest 19/19 на modernc);
- лимиты check/consume/refund/remaining/visit, возврат только своим демо;
- аккаунт (экспорт, отзыв согласия на связь, удаление), лиды с уведомлением в Max, экспорт JSONL, admin, widget.js;
- CI build-auth, Dockerfile, `infra/compose/auth` (x-fl-host: freelance-ru, x-fl-pd).

Ждёт:
- RU-VPS (infra) и DNS `auth.demo.sedlab.dev` на его IP; бэкап в VK Cloud Hotbox (ключи VKS3_*, infra);
- бот Max, приложения Яндекс ID и VK ID, AUTH_PD_EMAIL и прочие плейсхолдеры политики (владелец, legal-ru);
- регистрация webhook Max (`POST /subscriptions` на `https://auth…/max/webhook` с секретом) после выдачи токена;
- проверка VK ID на реальном приложении.
