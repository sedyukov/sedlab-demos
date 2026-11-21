# auth: единый аккаунт sedlab demo

Один аккаунт на все `*.demo.sedlab.dev`. Без входа демо работают на заглушках, с аккаунтом подключаются настоящие
провайдеры с жёсткими лимитами. Сервис живёт на российском хосте (152-ФЗ), демо проверяют токен офлайн по публичному ключу.

- **Вход.** Только российские сервисы: Max (бот по deep link, Mini App), Яндекс ID, VK ID (OAuth, модуль `accounts`).
  Список и порядок задаются в `AUTH_LOGIN_PROVIDERS_RU/_EN`.
- **Согласия.** Три отдельных чекбокса: ПД, соглашение демо, необязательная связь. Версии и время хранятся в БД.
- **Токен.** JWS EdDSA (Ed25519) в cookie `sl_auth` на `.demo.sedlab.dev`, без персональных данных.
- **Лимиты.** На человека в день, глобально в день, бюджет в месяц (`limits.json`). `consume` атомарен и идемпотентен.
- **Аккаунт.** Экспорт данных в JSON, отзыв согласия на связь, удаление по кнопке.
- **Лиды.** Уведомление владельцу в Max без ПД в тексте, экспорт JSONL.
- **Хранилище.** `Store` модуля `accounts` (SQLite через modernc, переносимый SQL, Postgres как второй драйвер).

## Запуск локально
```bash
AUTH_DEV=1 go run .            # http://localhost:8080/login, кнопка «DEV: войти как тестовый пользователь»
go test ./...                  # весь поток: вход Max, OAuth (фейковый Яндекс), согласия, лимиты, удаление, storetest
go run . keygen                # AUTH_SIGNING_KEY и ACCOUNTS_PUBLIC_KEY для демо
```

## API для демо
`POST /internal/v1/{check|consume|refund|remaining|visit}` с `X-Service-Key` (свой ключ у каждого демо, `AUTH_SVC_<DEMO>`).
Клиент: модуль `accounts` (`mode()` выбирает live или mock, `live_call` возвращает единицы при ошибке).
Виджет: `<script src="https://auth.demo.sedlab.dev/widget.js" data-demo="voice" data-resource="yandex-stt" data-providers="Yandex SpeechKit (Россия)" defer></script>`.

Юридические тексты в `web/legal` генерируются из приватного пакета с плейсхолдерами `{{X}}`. Значения подставляются из env `AUTH_X` при деплое.
