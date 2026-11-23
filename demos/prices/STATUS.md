# STATUS: S6 «Мониторинг цен WB и Ozon» (app: prices) · 2026-11-23

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
- **legal-ru (38ed7ad):** имена авторов отзывов не загружаются (на странице «Покупатель»), тексты и ответы обезличиваются при загрузке
  (`ai.MaskPII`, имя автора как известное имя, на странице «[скрыто]»); модель получает только обезличенный текст, модуль ai маскирует ещё раз
  (`AI_PII_MASKING=on` по умолчанию). «Вставить свой отзыв»: две строки §4б + обязательный чекбокс `prices-v1` (абсолютные ссылки https://auth.demo.sedlab.dev/consent/prices
  и https://auth.demo.sedlab.dev/terms#3a; при отметке чекбокса `POST /api/consent` → отметка в сеансе, для вошедших ещё `accounts.RecordConsent` (kind prices) в журнал auth;
  недоступность auth загрузку не блокирует), сервер без согласия отвечает 400, версия и время согласия хранятся в сессии (в журнал пишется только факт и версия, без текста).
  Подпись черновика: реальный провайдер («тестовая модель демо» в mock). GigaChat разрешён только с маскированием (`AI_GIGACHAT_PII_MASKING=off` → отказ запуска).
- **Без ПД:** песочница своя у каждого посетителя (cookie, в памяти, LRU 2000), простой > 6 ч удаляется, полная очистка раз в сутки.
  Лимиты: API 4/с на IP, сбор 6 подряд и дальше 1 в 2 с на IP, 60 сборов на сессию.
- **Тесты:** `go test -race` 17 (журнал согласий, обезличивание и согласие, сбор совпадает с витриной, отказы, рекомендации и применение, добавление карточек, отчёт без «—»,
  черновики, HTTP-поток, изоляция сессий, лимит, LRU, статика). `e2e/e2e.mjs` (Playwright): 29 проверок, включая 390 px без горизонтальной прокрутки.
- **Медиа:** `media/01…08-*-ru.png`, `demo-ru.mp4` (~37 с), `demo-ru.gif`. README.md (для заказчика, RU), portfolio.md (RU).
- **CI:** `.github/workflows/build-prices.yml` (копия build-leads: vet+race → образ `ghcr.io/sedyukov/prices:sha-xxxxxxx`, guard приватности).
- **Infra:** `infra/compose/prices-ru` (compose, site.caddy, health.url, env.keys).

- **CI 21.11 (1835679):** job test зелёный (vet + race на CI). Job build остановился на «GHCR visibility guard (before push)»:
  новый пакет `ghcr.io/sedyukov/prices` создан пустым пробником и публичен по умолчанию, код модулей в него не попал.

- **Деплой 21.11 (до GHCR):** `prices-ru` на freelance-ru, образ `ghcr.io/sedyukov/prices:sha-ae317af` собран на Маке
  (Docker Hub не отдал golang-образ: бинарник кросс-компилирован `GOOS=linux GOARCH=amd64`, образ на distroless, 10,4 МБ),
  `docker save | ssh docker load`, `deploy.sh prices-ru --host freelance-ru --tag sha-ae317af --no-pull`. check-demos --pre-dns зелёное;
  e2e 29/29 против контейнера через SSH-туннель; память 2 МБ в простое, 8 МБ после e2e. accounts: provider http, demo prices.
  Снаружи по `--resolve …:443:5.35.4.25` TLS не отвечает (сертификат выпустится после A-записи; у auth так же).

- **23.11: прод.** Пакет GHCR приватный, CI build-prices зелёный (run 44951526297, `sha-e910a49`). A-запись `prices.demo → 5.35.4.25`.
  Обычный деплой `infra/deploy.sh prices-ru --host freelance-ru --tag sha-e910a49` (pull из GHCR), check-demos зелёное, сертификат 89 дней.
  e2e по https://prices.demo.sedlab.dev: 29/29. Ссылки согласия (auth.demo/consent/prices, /terms) отвечают 200. Память 6,6 МБ.

## Осталось
- Сценарий со входом: войти через виджет, отметить чекбокс, в логе prices `journal=auth` (после OAuth-приложений владельца; виджет: раскомментировать `AUTH_WIDGET_URL` в compose).
- Живая YandexGPT: ресурс `yandex-llm` для prices в auth есть (3 вызова в день на человека, бюджет общий с voice). Нужны ключи
  `AI_YANDEX_API_KEY` + `AI_YANDEX_FOLDER_ID` в sec, затем `AI_LLM_PROVIDER: yandexgpt` в compose.
- Цены в README: на утверждении у владельца.
- Живой сбор (по желанию): WB через `marketplaces.CompetitorPrices`, Ozon через scraping http; в публичном демо не включаем.
