# Portfolio text: Voice AI Assistant

No links or contacts here, as marketplace rules require. RU text: attach `media/ru-*.png` and `media/demo-ru.mp4`; EN text: `media/en-*.png` and `media/demo-en.mp4`.

## RU

**Заголовок:** AI-ассистент для голосовых и созвонов: расшифровка, саммари, задачи, ответ голосом

**Описание:**
Бот и веб-сервис для голосовых сообщений и записей созвонов. На вход принимает голосовое из Telegram, аудио- или видеофайл
либо запись с микрофона. Выдаёт:
- расшифровку с таймкодами по фразам;
- саммари в 2–3 предложения;
- задачи в формате «кто / что / срок» (JSON, можно сразу выгрузить в CRM или таблицу);
- короткий ответ голосом. В Telegram он приходит голосовым сообщением.

Работает на российском стеке: GigaChat или YandexGPT, распознавание и озвучка Yandex SpeechKit, сервер и данные в РФ (152-ФЗ).
LLM переключается конфигом, можно подключить и локальную модель. Расходы под контролем: лимит на пользователя, дневной
бюджет, ограничение длины аудио. Без ключей работает в демо-режиме, поэтому заказчик может посмотреть результат ещё до оплаты API.

Стек: Python, FastAPI, Telegram Bot API, GigaChat, YandexGPT, Yandex SpeechKit, Docker.
Срок похожего проекта: 5–10 дней.

**Теги:** Telegram-бот, AI, GigaChat, YandexGPT, SpeechKit, распознавание речи, саммари, 152-ФЗ, автоматизация, Python

## EN

**Title:** AI voice & meeting assistant: transcription, summary, action items, voice reply

**Description:**
A bot and web service for voice notes and call recordings. Input: a Telegram voice note, an audio or video file, or the mic.
Output:
- a transcript with timestamps for each phrase;
- a 2–3 sentence summary;
- action items as who / what / due date (JSON, ready for a CRM, Notion or a spreadsheet);
- a short spoken reply, delivered as a voice message in Telegram.

You can switch the LLM in config: OpenAI, Claude or a self-hosted model. Spend stays under control with
per-user rate limits, a daily budget and a maximum audio length. Without keys it runs in demo mode, so the client sees
results before paying for any API.

Stack: Python, FastAPI, Telegram Bot API, OpenAI Whisper & TTS, Anthropic, Docker.
A similar project takes 5–10 days.

**Tags:** Telegram bot, AI assistant, OpenAI, Claude, speech-to-text, Whisper, meeting notes, summarization, automation, Python
