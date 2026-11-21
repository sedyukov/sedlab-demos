# STATUS: S4 «voice» (AI-бот для голосовых и созвонов), 2026-11-21

## Две редакции (решение владельца 2026-11-21)
Одна кодовая база, редакция задаётся `EDITION=ru|intl` (по умолчанию intl).

| | RU (`EDITION=ru`) | INTL (`EDITION=intl`) |
|---|---|---|
| Адрес | https://voice.demo.sedlab.dev | https://voice-en.demo.sedlab.dev |
| Compose | `infra/compose/voice-ru` (`x-fl-host: freelance-ru`, `x-fl-pd: true`) | `infra/compose/voice-en` (`x-fl-host: freelance-1`) |
| Язык | только русский, ссылка «English version» | только английский, ссылка «Русская версия» |
| LLM | GigaChat, YandexGPT (чужие ключи игнорируются) | OpenAI, Anthropic |
| STT/TTS | Yandex SpeechKit | OpenAI |
| Живой режим | включён по умолчанию (`VOICE_LIVE`), работает при наличии ключей | выключен (`VOICE_LIVE=0`), только mock |
| Примеры | planning-ru (mp4), voice-note-ru (ogg) | standup-en (mp3), client-call-en (mp4), voice-note-en (ogg) |
| Вход | accounts (Max, Яндекс ID, VK ID): подключается вместе с сессией accounts | нет |

Сейчас на freelance-1 работает прежний `compose/voice` (смешанная RU/EN-сборка, voice.demo) до переезда RU-версии.

## Сделано
- **Веб-демо** (FastAPI + чистый JS, светлая и тёмная тема, адаптив). Три способа подать запись: заготовленный пример, свой файл (аудио или видео) или микрофон. Дальше конвейер: расшифровка → саммари → задачи (кто / что / срок) → ответ голосом. Переключатель LLM в рамках редакции, правка текста с пересчётом, копирование задач в Markdown.
- **Демо-режим без ключей**: примеры с готовыми результатами, сгенерированы `tools/make_samples.py` (голоса macOS `say`, имена вымышленные). Свои файлы в этом режиме идут через mock модуля ai. Для записи с микрофона в Chrome и Safari работает распознавание речи браузера; ответ озвучивается голосом браузера (speechSynthesis).
- **Живой режим**: весь доступ к моделям идёт через модуль ai. Лимиты: `VOICE_IP_PER_HOUR=6` на IP, `VOICE_DAILY_LIVE=120` в день на всех, аудио до `VOICE_MAX_AUDIO_SEC=180` (браузер перекодирует в WAV 16 кГц, сервер проверяет длину по заголовку) и до `VOICE_MAX_UPLOAD_MB=10`. Запросы сверх лимитов и слишком длинное аудио отклоняются до платного вызова. Для примеров в живом режиме используются копии `samples/*/stt.ogg` (SpeechKit не принимает mp4). GigaChat работает через корневой сертификат Минцифры в образе (`certs/`, `AI_GIGACHAT_CA_FILE`).
- **ПД (R9/R2 legal-ru)**: демо ничего не хранит. Ключи лимитера (IP, chat_id) лежат только в памяти, в виде хэша с солью процесса, и сбрасываются раз в сутки. Страница предупреждает: записи не сохраняются, не загружайте ПД.
- **Telegram-бот** (по желанию, `VOICE_TELEGRAM_TOKEN`): long polling в том же процессе, язык по редакции. Принимает голосовое, аудио, кружок или документ-аудио; в ответ присылает текст (расшифровка, саммари, задачи) и голосовое (TTS в opus). Вся работа с Telegram идёт через telegram-core (`get_updates`, `download_file`, `send_voice`). При SpeechKit бот просит голосовое или mp3/ogg вместо кружков и m4a (иначе нужен ffmpeg в образе).
- **Тесты**: 46 штук, `pytest -q`. Покрывают обе редакции (провайдеры, языки, примеры, принудительный mock в intl, ссылки), демо- и живой режим с фейковыми провайдерами, лимиты, ПД в лимитере, парсер WAV и цикл бота на MockAPI.
- **CI**: `.github/workflows/build-voice.yml`: тесты, затем сборка `ghcr.io/sedyukov/voice:sha-xxxxxxx` (один образ на обе редакции). Пакет PRIVATE, guard-шаги ghcr-private-guard, без кэшей и артефактов.
- **Образ**: multi-stage, python:3.12-slim, около 140 МБ, в работе около 45 МБ RAM, `mem_limit 160m`.
- **Медиа**: `media/en-*.png` и `media/ru-*.png` (по 4 скриншота), `demo-en.mp4`/`.gif` (37 с), `demo-ru.mp4`/`.gif` (33 с). README с двумя ссылками и таблицей отличий (RU и EN, цены утверждены владельцем), `portfolio.md` (RU-текст про российский стек, EN-текст про OpenAI/Claude).
- **Стиль**: в текстах для людей нет длинного тире (правило владельца).

## Деплой
- INTL: `~/freelance/infra/deploy.sh voice-en --tag sha-<…>` → https://voice-en.demo.sedlab.dev.
- RU (когда будет RU-VPS): `deploy.sh voice-ru --host freelance-ru --tag sha-<…>` → A-запись `voice.demo` → IP RU-хоста → `deploy.sh rm voice` на freelance-1 и удалить `infra/compose/voice`.
- RU-ключи (раскомментировать в `infra/compose/voice-ru/env.keys`, когда появятся в sec): `AI_GIGACHAT_AUTH_KEY`, `AI_YANDEX_API_KEY`, `AI_YANDEX_FOLDER_ID`, `AUTH_SVC_VOICE:ACCOUNTS_SERVICE_KEY`, `ACCOUNTS_PUBLIC_KEY`, `VOICE_TELEGRAM_TOKEN`.
- Через ru-relay (если нужен): `AI_GIGACHAT_BASE_URL=https://<relay>/…/api/v1`, `AI_GIGACHAT_AUTH_URL=<полный URL OAuth>`, `AI_YANDEX_BASE_URL=https://<relay>/<префикс>`, `AI_YANDEX_STT_BASE_URL=$R/yandex/stt`, `AI_YANDEX_TTS_BASE_URL=$R/yandex/tts`, `AI_YANDEX_OPERATION_BASE_URL=$R/yandex/operation`. Определение живых провайдеров смотрит только на ключи.

## Осталось
- accounts (модуль a027917): вход в RU-редакции (живой режим только после входа), вместе с сессией accounts. Env уже в `compose/voice-ru`: `ACCOUNTS_PROVIDER=http`, `ACCOUNTS_AUTH_URL=http://auth:8080`; секреты `ACCOUNTS_SERVICE_KEY` (sec `AUTH_SVC_VOICE`), `ACCOUNTS_PUBLIC_KEY` (или JWKS), `ACCOUNTS_DEMO`. На mock живой режим принудительно выключен. INTL к auth не подключается.
- После запуска живого режима на RU-VPS: пересчитать результаты примеров настоящими моделями (сейчас они составлены вручную по сценарию) и снять видео в живом режиме.
