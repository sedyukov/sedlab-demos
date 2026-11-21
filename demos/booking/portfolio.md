# Portfolio text · Текст для портфолио

No links or contacts, so it can be pasted on Kwork, FL.ru, Habr Freelance and Upwork. Media for the gallery: `media/` (01–06 + demo.mp4 / demo.gif).

---

## RU — заголовок
Telegram-бот записи в салон + мини-апп: предоплата ЮKassa / Telegram Stars, напоминания, админка

## RU — описание
Бот и мини-апп для онлайн-записи в салон красоты, барбершоп, клинику или студию.

Клиент открывает мини-апп из чата, выбирает услугу, мастера и свободное время и вносит предоплату картой (ЮKassa) или в
Telegram Stars. Пока он платит, время закреплено за ним 15 минут, поэтому двойных записей не бывает. Бот сразу
подтверждает запись и напоминает о визите за сутки и за 2 часа. Если клиент отменяет запись позже чем за сутки до визита,
предоплата возвращается автоматически.

Владелец получает уведомление о каждой записи. В админке (она тоже открывается в Telegram) он видит расписание дня по
мастерам, записи и выручку по предоплатам, редактирует услуги, цены и график мастеров.

Интерфейс на русском и английском. Подпись данных мини-аппа проверяется, вебхуки идемпотентны. Сервис занимает около
10 МБ памяти и работает на самом дешёвом VPS. Есть версия для Max: тот же бот и мини-апп, оплата
в Max картой.

Стек: Go, SQLite, Svelte, Telegram Bot API / Mini Apps / Stars, ЮKassa, Docker.
Срок такого проекта: 7–10 дней.

---

## EN — title
Telegram booking bot + Mini App: deposits in Telegram Stars or by card, reminders, owner admin panel

## EN — description
A Telegram bot and Mini App for online booking at a beauty salon, barbershop, clinic or studio.

Clients open the Mini App from the chat, pick a service, a master and a free time slot, and pay a deposit in Telegram
Stars or by card. The slot is held for 15 minutes during payment, so double bookings can't happen. The bot confirms the
booking right away and sends reminders 24 h and 2 h before the visit. A cancellation more than 24 h ahead refunds the
deposit automatically.

The owner gets a Telegram message for every booking. The admin panel, which also opens inside Telegram, shows the day
schedule by master, bookings and deposit revenue, and lets the owner edit services, prices and working hours.

English and Russian UI. Mini App launch data is signature-checked and webhooks are idempotent. The service uses about
10 MB of RAM and runs on the cheapest VPS. Stars invoices, pre-checkout checks and refunds are built in, which is also what
an existing bot needs to move its payments to Stars. A version for the Max messenger runs on the same code.

Stack: Go, SQLite, Svelte, Telegram Bot API / Mini Apps / Stars, YooKassa / Stripe, Docker.
Typical timeline: 7–10 days.
