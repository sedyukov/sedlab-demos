# auth — единый аккаунт sedlab demo

Один аккаунт на все `*.demo.sedlab.dev`: без входа демо работают на заглушках, с аккаунтом — настоящие провайдеры
с жёсткими лимитами. Сервис живёт на российском хосте (152-ФЗ), демо проверяют токен офлайн по публичному ключу.

- **Вход** — только российские сервисы: Max (бот по deep link, Mini App), Яндекс ID, VK ID (OAuth, модуль `accounts`).
  Список и порядок — `AUTH_LOGIN_PROVIDERS_RU/_EN`.
- **Согласия** — три отдельных чекбокса (ПД, соглашение демо, необязательная связь), версии и время в БД.
- **Токен** — JWS EdDSA (Ed25519) в cookie `sl_auth` на `.demo.sedlab.dev`, без персональных данных.
- **Лимиты** — на человека в день, глобально в день, бюджет в месяц (`limits.json`); `consume` атомарен и идемпотентен.
- **Аккаунт** — экспорт данных JSON, отзыв согласия на связь, удаление по кнопке.
- **Лиды** — уведомление владельцу в Max без ПД в тексте, экспорт JSONL.
- **SQLite** через `Store` на переносимом SQL (Postgres — второй драйвер), миграции в `migrations/`.

## Запуск локально
```bash
AUTH_DEV=1 go run .            # http://localhost:8080/login → «DEV: войти как тестовый пользователь»
go test ./...                  # весь поток: вход Max, OAuth (фейковый Яндекс), согласия, лимиты, удаление
go run . keygen                # AUTH_SIGNING_KEY + ACCOUNTS_PUBLIC_KEY для демо
```

## API для демо
`POST /internal/v1/{check|consume|refund|remaining|visit}` с `X-Service-Key` (ключ на демо, `AUTH_SVC_<DEMO>`).
Клиент — модуль `accounts` (`mode()` → live/mock, `live_call` с возвратом единиц при ошибке).
Виджет: `<script src="https://auth.demo.sedlab.dev/widget.js" data-demo="voice" data-resource="yandex-stt" data-providers="Yandex SpeechKit (Россия)" defer></script>`.

Юридические тексты в `web/legal` генерируются из приватного пакета с плейсхолдерами `{{X}}`; значения — из env `AUTH_X` при деплое.
