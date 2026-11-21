# STATUS: S4 «voice» (AI-бот для голосовых и созвонов), 2026-11-21

## Две редакции (решение владельца 2026-11-21)
Одна кодовая база, редакция задаётся `EDITION=ru|intl` (по умолчанию intl).

| | RU (`EDITION=ru`) | INTL (`EDITION=intl`) |
|---|---|---|
| Адрес | https://voice.demo.sedlab.dev | https://voice-en.demo.sedlab.dev |
| Compose | `infra/compose/voice-ru` (`x-fl-host: freelance-ru`, `x-fl-pd: true`) | `infra/compose/voice-en` (`x-fl-host: freelance-1`) |
| Язык | только русский, ссылка «English version» | только английский, ссылка «Русская версия» |
| LLM | YandexGPT (по умолчанию), GigaChat (чужие ключи игнорируются) | OpenAI, Anthropic |
| STT/TTS | Yandex SpeechKit | OpenAI |
| Живой режим | включён по умолчанию (`VOICE_LIVE`), работает при наличии ключей | выключен (`VOICE_LIVE=0`), только mock |
| Примеры | planning-ru (mp4), voice-note-ru (ogg) | standup-en (mp3), client-call-en (mp4), voice-note-en (ogg) |
| Вход и лимиты | accounts: живой вызов только после входа, `live_call` (списание → вызов → возврат при ошибке); ресурсы gigachat, yandex-llm, yandex-stt (секунды, до 60), yandex-tts. Кнопка входа: заглушка, пока `VOICE_ACCOUNTS_WIDGET=0`; `1` подключает `widget.js` с auth | нет (локальный лимитер по IP, если включить `VOICE_LIVE=1`) |

Состояние на 2026-11-21, ночь:
- **voice-en**: https://voice-en.demo.sedlab.dev, freelance-1, `sha-82bb5aa`, check-demos зелёный.
- **voice-ru**: задеплоен на freelance-ru (5.35.4.25), `sha-82bb5aa`, healthy, около 48 МБ. Проверено изнутри хоста: конфиг RU, оба примера проходят цепочку, TLS до GigaChat/OAuth Сбера и SpeechKit/YandexGPT работает (CA-бандл хоста), auth доступен по сети edge. Снаружи HTTPS появится после A-записи `voice.demo → 5.35.4.25`: до неё сертификат на RU-хосте не выпускается.
- **voice** (старая смешанная сборка): работает на freelance-1 на voice.demo до переключения DNS; потом `deploy.sh rm voice` и удалить `infra/compose/voice`.

**Решение владельца (2026-11-21): на INTL-редакцию ресурсы не тратим.** voice-en заморожен на `sha-ae7866a`: не передеплоить, медиа, README EN и EN-тексты не обновлять, e2e не гонять. Все изменения дальше только для RU (`deploy.sh voice-ru --host freelance-ru`).

## Сделано
- **Веб-демо** (FastAPI + чистый JS, светлая и тёмная тема, адаптив). Три способа подать запись: заготовленный пример, свой файл (аудио или видео) или микрофон. Дальше конвейер: расшифровка → саммари → задачи (кто / что / срок) → ответ голосом. Переключатель LLM в рамках редакции, правка текста с пересчётом, копирование задач в Markdown.
- **Демо-режим без ключей**: примеры с готовыми результатами, сгенерированы `tools/make_samples.py` (голоса macOS `say`, имена вымышленные). Свои файлы в этом режиме идут через mock модуля ai. Для записи с микрофона в Chrome и Safari работает распознавание речи браузера; ответ озвучивается голосом браузера (speechSynthesis).
- **Живой режим**: весь доступ к моделям идёт через модуль ai. Лимиты: `VOICE_IP_PER_HOUR=6` на IP, `VOICE_DAILY_LIVE=120` в день на всех, аудио до `VOICE_MAX_AUDIO_SEC=180` (браузер перекодирует в WAV 16 кГц, сервер проверяет длину по заголовку) и до `VOICE_MAX_UPLOAD_MB=10`. Запросы сверх лимитов и слишком длинное аудио отклоняются до платного вызова. Для примеров в живом режиме используются копии `samples/*/stt.ogg` (SpeechKit не принимает mp4). GigaChat работает через корневой сертификат Минцифры в образе (`certs/`, `AI_GIGACHAT_CA_FILE`).
- **legal-ru (аудио и провайдеры)**: аудио не хранится, обрабатывается в памяти и отбрасывается сразу после расшифровки (на диск ничего не пишется, бот скачивает файлы в память). Голосовой идентификации и распознавания дикторов нет (572-ФЗ). Заголовок `x-data-logging-enabled: false` для YandexGPT/SpeechKit добавляет s2 в модуль ai. В GigaChat можно только обезличенный текст: модуль ai (79c785f) маскирует ПД внутри `Client.chat`, демо передаёт участников встречи в `known_names` и не пускает GigaChat вживую при `AI_GIGACHAT_PII_MASKING=off`; поддержан cloud.ru (`AI_GIGACHAT_MODE=cloudru`, `AI_GIGACHAT_API_KEY`). Маскировщик модуля исправлен (d0b514c, имена на «й»/«ь»), живой GigaChat разрешён (`GIGACHAT_LIVE_ALLOWED = True`, `False` = аварийный стоп); тест проверяет, что имена участников примера не уходят в GigaChat. Одиночные имена, которых нет в `known_names` (например, «Андрей» в своей записи), модуль не ловит; по решению владельца в обезличивание дальше не вкладываемся, оговорка над загрузкой просит загружать только тестовые записи. Заголовок `x-data-logging-enabled: false` на запросах Yandex Cloud ставит модуль. YandexGPT стоит первым и выбран по умолчанию в RU-редакции.
- **legal v3 (terms-v3, раздел 3а; ui-texts §4б)**: в RU над загрузкой и записью две строки оговорки и обязательный чекбокс согласия voice-v1 (ссылки на auth.demo.sedlab.dev/consent/voice и /terms#3a; для вошедших журнал согласий в auth: страница при отметке чекбокса зовёт `POST /api/consent`, тот вызывает `accounts.record_consent(token, "voice", "voice-v1")` (модуль d1923f7, ключ идемпотентности генерирует модуль); без метода, без входа или при недоступном auth остаётся отметка в сеансе, загрузка не блокируется); без отметки загрузка и микрофон неактивны, сервер отклоняет свою запись без `consent=voice-v1` (`consent_required`). Примеры открываются по умолчанию и согласия не требуют. В оговорке указаны только реально используемые провайдеры (Яндекс.Облако, плюс GigaChat, если он живой); в карточке саммари подпись с реальной моделью ответа (YandexGPT / GigaChat или «мок»). Версия согласия не сохраняется: демо не хранит данных; если юрист потребует журнал, писать версию и время к непрозрачному user_id в accounts.
- **ПД (R9/R2 legal-ru)**: демо ничего не хранит. Ключи лимитера (IP, chat_id) лежат только в памяти, в виде хэша с солью процесса, и сбрасываются раз в сутки. Страница предупреждает: записи не сохраняются, не загружайте ПД.
- **Telegram-бот** (по желанию, `VOICE_TELEGRAM_TOKEN`): long polling в том же процессе, язык по редакции. В RU-редакции бот только на mock: у пользователей Telegram нет токена входа, а с RU-хоста Telegram API недоступен. По документам legal v2 в RU-боте сначала текст согласия и кнопка «Согласен, начать» (bot-v1), до нажатия ничего не обрабатывается; /delete удаляет согласие; согласие хранится только в памяти, хэшем, не дольше 7 дней. Имя и username бот не сохраняет. В INTL-боте строка «Demo: use test data only. Nothing is stored after the session.» Принимает голосовое, аудио, кружок или документ-аудио; в ответ присылает текст (расшифровка, саммари, задачи) и голосовое (TTS в opus). Вся работа с Telegram идёт через telegram-core (`get_updates`, `download_file`, `send_voice`). При SpeechKit бот просит голосовое или mp3/ogg вместо кружков и m4a (иначе нужен ffmpeg в образе).
- **Тесты**: 63 штуки, `pytest -q`. Покрывают обе редакции (провайдеры, языки, примеры, принудительный mock в intl, ссылки), accounts в RU (без входа → mock, лимит 3 в день, возврат при ошибке провайдера, auth недоступен, cookie, ошибка конфига → mock), демо- и живой режим с фейковыми провайдерами, лимиты, ПД в лимитере, парсер WAV и цикл бота на MockAPI.
- **CI**: `.github/workflows/build-voice.yml`: тесты, затем сборка `ghcr.io/sedyukov/voice:sha-xxxxxxx` (один образ на обе редакции). Пакет PRIVATE, guard-шаги ghcr-private-guard, без кэшей и артефактов.
- **Образ**: multi-stage, python:3.12-slim, около 140 МБ, в работе около 45 МБ RAM, `mem_limit 160m`.
- **Медиа**: `media/en-*.png` и `media/ru-*.png` (по 4 скриншота), `demo-en.mp4`/`.gif` (37 с), `demo-ru.mp4`/`.gif` (33 с). README с двумя ссылками и таблицей отличий (RU и EN, цены утверждены владельцем), `portfolio.md` (RU-текст про российский стек, EN-текст про OpenAI/Claude).
- **Стиль**: в текстах для людей нет длинного тире (правило владельца).

## Деплой
- INTL: `~/freelance/infra/deploy.sh voice-en --tag sha-<…>` → https://voice-en.demo.sedlab.dev.
- RU (когда будет RU-VPS): `deploy.sh voice-ru --host freelance-ru --tag sha-<…>` → A-запись `voice.demo` → IP RU-хоста → `deploy.sh rm voice` на freelance-1 и удалить `infra/compose/voice`.
- RU-ключи: `AUTH_SVC_VOICE:ACCOUNTS_SERVICE_KEY` и `ACCOUNTS_PUBLIC_KEY` уже включены в `infra/compose/voice-ru/env.keys`; ключи моделей (`AI_GIGACHAT_AUTH_KEY`, `AI_YANDEX_API_KEY`, `AI_YANDEX_FOLDER_ID`) раскомментировать, когда появятся в sec. На RU-хосте смонтирован CA-бандл хоста (корень Минцифры для Сбера), `SSL_CERT_FILE` указывает на него.
- Через ru-relay (если нужен): `AI_GIGACHAT_BASE_URL=https://<relay>/…/api/v1`, `AI_GIGACHAT_AUTH_URL=<полный URL OAuth>`, `AI_YANDEX_BASE_URL=https://<relay>/<префикс>`, `AI_YANDEX_STT_BASE_URL=$R/yandex/stt`, `AI_YANDEX_TTS_BASE_URL=$R/yandex/tts`, `AI_YANDEX_OPERATION_BASE_URL=$R/yandex/operation`. Определение живых провайдеров смотрит только на ключи.

## Осталось
- Включить кнопку входа: после запуска auth снаружи (DNS auth.demo) поставить `VOICE_ACCOUNTS_WIDGET: "1"` в `infra/compose/voice-ru/compose.yml` и передеплоить.
- Живые ключи RU-стека (`AI_GIGACHAT_AUTH_KEY`, `AI_YANDEX_API_KEY`, `AI_YANDEX_FOLDER_ID`): раскомментировать в `env.keys` voice-ru, передеплоить, e2e «вход, 3 живых запуска, 4-й в mock».
- После запуска живого режима: пересчитать результаты примеров настоящими моделями (сейчас они составлены вручную по сценарию) и снять видео в живом режиме.
