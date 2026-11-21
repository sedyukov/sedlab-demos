# STATUS: S6 «Мониторинг цен WB и Ozon» (app: prices) · 2026-11-21

Только RU-редакция (решение владельца 22.11: INTL не делаем). Адрес: https://prices.demo.sedlab.dev (RU-хост freelance-ru).

## Сделано
- **Приложение** (Go, stdlib + модули marketplaces, scraping, sheets-storage, notify, ai, accounts):
  - 5 товаров магазина (каталог, цены, отзывы: mock модуля marketplaces), по 4 конкурента на WB и Ozon.
  - Сбор через модуль scraping: витрины смоделированы (WB: открытые данные карточки `card.wb.ru/cards/v2/detail`, Ozon: страница товара
    с разметкой schema.org) и отдаются через mock-транспорт модуля: robots.txt, лимит на сайт, повторы на 429/503, отказ после 3 попыток
    (в отчёте остаются вчерашние данные). Сбор = следующий день; история 30 дней предзагружена, ряды детерминированы.
  - Графики цен и остатков (SVG без библиотек, подсказка по наведению), рекомендация цены (−1% от самого дешёвого в наличии,
    не ниже 85% стартовой цены; при упоре в минимум: «не опускаем»), «Применить» → `UpdatePrices` (mock).
  - Добавление конкурента по ссылке/артикулу (`marketplaces.ParseProductRef`), до 8 на сессию, удаление своих.
  - Ежедневный отчёт → notify (Max и Telegram, mock), журнал отправленного. Строки в таблицу (sheets-storage mock), выгрузка CSV и XLSX (`EncodeXLSX`).
  - Отзывы: черновик через `ai.ReplyToReview` (mock по умолчанию), правка и публикация (`AnswerReview`).
  - accounts (RU): живой LLM только через `accounts.LiveCall` (ресурс `yandex-llm`), иначе mock; виджет входа по `AUTH_WIDGET_URL`.
    GigaChat в живом режиме выключен (legal-ru, как в voice). Сейчас у prices живых ресурсов нет: всё в mock.
- **Без ПД:** песочница своя у каждого посетителя (cookie, в памяти, LRU 2000), простой > 6 ч удаляется, полная очистка раз в сутки.
  Лимиты: API 4/с на IP, сбор 6 подряд и дальше 1 в 2 с на IP, 60 сборов на сессию.
- **Тесты:** `go test -race` 13 (сбор совпадает с витриной, отказы, рекомендации и применение, добавление карточек, отчёт без «—»,
  черновики, HTTP-поток, изоляция сессий, лимит, LRU, статика). `e2e/e2e.mjs` (Playwright): 25 проверок, включая 390 px без горизонтальной прокрутки.
- **Медиа:** `media/01…07-*-ru.png`, `demo-ru.mp4` (~33 с), `demo-ru.gif`. README.md (для заказчика, RU), portfolio.md (RU).
- **CI:** `.github/workflows/build-prices.yml` (копия build-leads: vet+race → образ `ghcr.io/sedyukov/prices:sha-xxxxxxx`, guard приватности).
- **Infra:** `infra/compose/prices-ru` (compose, site.caddy, health.url, env.keys).

- **CI 21.11 (1835679):** job test зелёный (vet + race на CI). Job build остановился на «GHCR visibility guard (before push)»:
  новый пакет `ghcr.io/sedyukov/prices` создан пустым пробником и публичен по умолчанию, код модулей в него не попал.

## Осталось
- **Владелец:** github.com/users/sedyukov/packages/container/prices/settings → Change visibility → Private, затем Re-run build-prices.
- После зелёного build: `infra/deploy.sh prices-ru --host freelance-ru --tag sha-<7>`, проверка `curl --resolve prices.demo.sedlab.dev:443:5.35.4.25 …/health`.
- `infra/demos.txt`: строке prices нужен флаг `ru-only` (зона infra), A-запись `prices.demo → 5.35.4.25`.
- Живые ресурсы (по желанию): `yandex-llm` для prices в limits.json auth + ключи YandexGPT; виджет входа после DNS auth.
- Живой сбор (по желанию): WB через `marketplaces.CompetitorPrices` (теперь с остатками), Ozon через scraping http; в публичном демо не включаем.
