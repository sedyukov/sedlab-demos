# Voice AI Assistant · Голосовой AI-ассистент

**Live demo:** https://voice.demo.sedlab.dev · [EN](#en) · [RU](#ru)

![demo](media/demo.gif)

---

<a id="en"></a>
## EN: voice messages and calls → summary, action items and a voice reply

Your team or your clients send voice notes and record calls, and nobody wants to listen to them twice.
This assistant transcribes the audio, writes a 2–3 sentence summary, lists the action items as **who / what / due date**,
and answers with a voice. It works on a web page (upload a file or use the mic) and as a Telegram bot (send it a voice note
and get the summary, tasks and a voice reply back).

### What it does
- **Speech-to-text** for voice notes, call recordings and video files (mp3, m4a, ogg/opus from Telegram, wav, webm, mp4/mov), with timestamps for each phrase. Click a phrase and playback jumps to that moment.
- **Summary**: 1–3 sentences, in the language of the recording.
- **Action items as JSON** (`title`, `owner`, `due`), ready to send to a CRM, Notion, Jira, Trello or a Google Sheet. The model is told not to invent owners or dates.
- **Voice reply (TTS)**: a short spoken recap. In Telegram it arrives as a voice message.
- **Choice of LLM**: OpenAI, Anthropic (Claude), GigaChat, YandexGPT, or any OpenAI-compatible endpoint such as a local model (Ollama, vLLM). You switch it in config without code changes.
- **Data residency**: for Russian clients the whole pipeline can run on a Russian stack (GigaChat / YandexGPT for the LLM, Yandex SpeechKit for speech, hosting and storage in Russia) to comply with Federal Law 152-FZ on personal data. For international clients it runs on OpenAI / Anthropic using the client's own keys, or on a self-hosted model.
- **Cost control**: rate limit per visitor or Telegram user, a daily budget for all users, a maximum audio length and file size. Requests over the limits are rejected before any paid API call is made.
- **Works without keys**: the public demo runs on three pre-computed samples (an EN stand-up, an RU video call, an RU Telegram voice note) and on mocks. The live mode starts as soon as the API keys are added.

### Screenshots
| | |
|---|---|
| ![home](media/01-home-en.png) | ![meeting](media/02-meeting-result-en.png) |
| ![video call, dark](media/03-video-call-ru-dark.png) | ![record](media/04-record-ru.png) |
| ![mobile](media/05-mobile-voice-note-ru.png) | Video: [media/demo.mp4](media/demo.mp4) (40 s) |

### Stack
Python 3.12 · FastAPI · vanilla JS (no build step; in-browser audio decoding and 16 kHz WAV encoding, MediaRecorder, Web Speech API fallback) ·
OpenAI Whisper / gpt-4o-transcribe and TTS · Anthropic, GigaChat, YandexGPT · Telegram Bot API · Docker (about 85 MB RAM) · Caddy · GitHub Actions.

Under the hood it uses two of my reusable modules. Each has the same interface in Go and Python and is covered by contract tests:
- **ai**: one interface for `chat`, `summarize`, `extract_tasks`, `transcribe` and `synthesize`, with a provider per slot (LLM / STT / TTS), unified errors, retries with backoff and rate limiting.
- **telegram-core**: Bot API client with retries and rate limits, idempotent webhooks, Mini App auth and Telegram Stars.

I also run my own production speech-recognition service: a Go server with workers (GigaAM on CPU, Whisper on GPU), Opus encoding in the browser and chunking by pauses. It transcribes about 1 hour of audio in ~30 s. When cloud STT is too expensive or not allowed, I can move a project onto self-hosted recognition.

### Price and timeline
| Scope | Timeline | Price |
|---|---|---|
| Telegram bot: voice note → transcript + summary | 3–5 days | from $600 |
| This demo's scope: calls and files, tasks, voice reply, LLM switch, limits | 5–10 days | $900–1,500 |
| + CRM / Notion / Google Sheets export, team accounts, self-hosted STT | from 2 weeks | quote |

API usage is billed separately at the provider's price. Typical cost: about $0.006 per minute of audio for Whisper, plus a few cents per summary.

### Run locally
```bash
pip install -e ../../path/to/modules/ai/python -e ../../path/to/modules/telegram-core/python -e '.[test]'
uvicorn voice_demo.main:app --port 8080     # demo mode, no keys needed
pytest -q                                    # 25 tests: demo mode, live mode with fakes, limits, Telegram bot
```
Config: see [.env.example](.env.example). `AI_OPENAI_API_KEY` turns on live STT/TTS. Each LLM goes live once its key is set.
`VOICE_TELEGRAM_TOKEN` starts the bot (long polling).
The `ai` and `telegram-core` modules are private and come with client projects. This folder holds the thin demo layer only.

---

<a id="ru"></a>
## RU: голосовые и созвоны → саммари, задачи и ответ голосом

Сотрудники и клиенты присылают голосовые и записывают созвоны, а переслушивать их никто не хочет. Ассистент расшифровывает
запись, пишет саммари в 2–3 предложения, выписывает задачи в формате **кто / что / срок** и отвечает голосом. Работает на
веб-странице (загрузить файл или записать с микрофона) и как Telegram-бот: пришлите голосовое, в ответ придут саммари, задачи
и голосовой ответ.

### Что умеет
- **Расшифровка** голосовых, записей созвонов и видеофайлов (mp3, m4a, ogg/opus из Telegram, wav, webm, mp4/mov) с таймкодами по фразам. Клик по фразе перематывает запись на это место.
- **Саммари** в 1–3 предложения на языке записи.
- **Задачи в JSON** (`title`, `owner`, `due`): их можно сразу отправить в amoCRM, Битрикс24, Notion, Jira или Google Таблицу. Модели запрещено придумывать ответственных и сроки.
- **Ответ голосом (TTS)**: короткое резюме. В Telegram приходит голосовым сообщением.
- **Выбор LLM**: OpenAI, Anthropic (Claude), GigaChat, YandexGPT или любая OpenAI-совместимая модель, в том числе локальная (Ollama, vLLM). Переключается конфигом, без правок кода.
- **Персональные данные и 152-ФЗ**: для российских заказчиков весь конвейер можно собрать на российском стеке: GigaChat или YandexGPT для LLM, Yandex SpeechKit для распознавания и озвучки, хостинг и хранилище в РФ. Записи граждан РФ при этом не уходят за границу. Для зарубежных заказчиков — OpenAI или Anthropic на ключах заказчика либо своя модель.
- **Контроль расходов**: лимит запусков на посетителя или пользователя Telegram, дневной бюджет на всех, максимальная длина и размер аудио. Запрос сверх лимита отклоняется до платного вызова API.
- **Работает без ключей**: в публичном демо три заранее посчитанных примера (стендап на EN, видеосозвон на RU, голосовое из Telegram на RU) и моки. Живой режим включается добавлением ключей.

### Стек
Python 3.12 · FastAPI · чистый JS (без сборки; декодирование аудио и перекодирование в WAV 16 кГц в браузере, MediaRecorder, Web Speech API) ·
OpenAI Whisper / gpt-4o-transcribe и TTS · Anthropic, GigaChat, YandexGPT · Telegram Bot API · Docker (~85 МБ RAM) · Caddy · GitHub Actions.

Внутри два моих переиспользуемых модуля. У каждого одинаковый интерфейс на Go и Python, оба покрыты контрактными тестами:
**ai** (LLM, STT и TTS за одним интерфейсом; ретраи, лимиты, единые ошибки) и **telegram-core** (Bot API с ретраями и лимитами, идемпотентные вебхуки, авторизация мини-аппов, Telegram Stars).

Кроме того, у меня есть свой сервис распознавания речи в проде: Go-сервер и воркеры (GigaAM на CPU, Whisper на GPU), кодирование Opus в браузере, нарезка по паузам. Час аудио он расшифровывает примерно за 30 секунд. Если облачное распознавание дорого или запрещено политикой, проект можно перевести на своё.

### Цена и сроки
| Объём | Срок | Цена |
|---|---|---|
| Telegram-бот: голосовое → расшифровка и саммари | 3–5 дней | от 40 000 ₽ |
| Как в демо: созвоны и файлы, задачи, ответ голосом, выбор LLM, лимиты | 5–10 дней | 80 000–150 000 ₽ |
| + выгрузка в CRM, Notion или Google Таблицу, командные аккаунты, своё распознавание речи | от 2 недель | по оценке |

Расходы на API оплачиваются отдельно по тарифу провайдера и считаются заранее. Ориентир: Whisper стоит около $0,006 за минуту аудио, саммари — несколько центов.

---

<sub>The samples use synthetic voices (macOS TTS) and fictional names. No personal data. Samples are regenerated with `python3 tools/make_samples.py`; screenshots and video with `tools/shoot_media.py`.</sub>
