# STATUS — demos/auth (2026-11-21)

Готово (код + тесты, `go test ./...` зелёный): вход Max (deep link + подтверждение в боте + Mini App initData),
Яндекс ID / VK ID через `accounts/go/oauth`, экран согласия по `legal/ui-texts.md` (3 чекбокса), страницы
/privacy /terms /consent/demo /consent/contact, токены Ed25519, лимиты check/consume/refund/remaining/visit,
аккаунт (экспорт, отзыв согласия на связь, удаление), лиды + уведомление в Max, экспорт JSONL, admin, widget.js,
Store на переносимом SQL + миграции, CI build-auth, Dockerfile, infra/compose/auth (x-fl-host: freelance-ru, x-fl-pd).

Ждёт:
- RU-VPS (infra) и DNS `auth.demo.sedlab.dev → RU IP`; бэкап — VK Cloud Hotbox (ключи VKS3_*, infra).
- Бот Max, приложения Яндекс ID / VK ID, AUTH_PD_EMAIL и прочие плейсхолдеры политики (владелец).
- Модуль `accounts` целиком (Store/limits клиента) — сейчас Store свой, в сервисе; переключить, когда s2 выпустит.
- Регистрация webhook Max (`POST /subscriptions` на `https://auth…/max/webhook` с секретом) — после токена.
- VK ID — проверить на реальном приложении.
