package main

import (
	"fmt"
	"time"
)

// Bot message texts (the Mini App has its own dictionaries in web/src/i18n.js).
var botTexts = map[string]map[string]string{
	"en": {
		"welcome":   "Hi, %s! 👋\nBook a haircut, manicure or brows in a few taps: pick a service, a master and a time.",
		"open":      "📅 Book now",
		"my_open":   "📋 My bookings",
		"confirmed": "✅ <b>Booked!</b>\n%s with %s\n🗓 %s\n💳 Prepaid: %s\n\nWe'll remind you a day and 2 hours before.",
		"remind24":  "⏰ Reminder: tomorrow, <b>%s</b> with %s at %s.\nCan't make it? Cancel in the app.",
		"remind2":   "⏰ See you in 2 hours: <b>%s</b> with %s at %s.",
		"cancelled": "❌ Your booking %s with %s on %s was cancelled.%s",
		"refunded":  "\nPrepayment %s is refunded.",
		"help":      "Commands: /start opens the booking app, /my shows your bookings.",
		"none":      "You have no upcoming bookings.",
		"upcoming":  "Your upcoming bookings:",
		"owner_new": "🆕 New booking #%d: %s, %s with %s, %s (%s)",
		"pay_fail":  "This slot is no longer held. Please pick another time.",
		"demo_note": "\n\nDemo: use test data only. Nothing is stored after the session.",
		"deleted":   "Done, this chat's data is deleted.",
	},
	"ru": {
		"welcome":     "Здравствуйте, %s! 👋\nЗапишитесь на стрижку, маникюр или брови за пару касаний: выберите услугу, мастера и время.",
		"open":        "📅 Записаться",
		"my_open":     "📋 Мои записи",
		"confirmed":   "✅ <b>Вы записаны!</b>\n%s, мастер %s\n🗓 %s\n💳 Предоплата: %s\n\nНапомним за сутки и за 2 часа.",
		"remind24":    "⏰ Напоминание: завтра <b>%s</b>, мастер %s, в %s.\nНе получается прийти? Отмените запись в приложении.",
		"remind2":     "⏰ Ждём вас через 2 часа: <b>%s</b>, мастер %s, в %s.",
		"cancelled":   "❌ Запись «%s» к мастеру %s на %s отменена.%s",
		"refunded":    "\nПредоплата %s возвращена.",
		"help":        "Команды: /start открывает запись, /my показывает ваши записи.",
		"none":        "У вас нет предстоящих записей.",
		"upcoming":    "Ваши предстоящие записи:",
		"owner_new":   "🆕 Новая запись #%d: %s, %s, мастер %s, %s (%s)",
		"pay_fail":    "Слот больше не удерживается, выберите другое время.",
		"consent":     "Это демо. Вводите тестовые данные. Нажимая «Согласен, начать», вы даёте <a href=\"https://sedlab.dev/consent/bot\">согласие на обработку персональных данных</a>: имя и username в мессенджере и то, что вы введёте, хранятся на сервере в России до 7 дней. Удалить в любой момент: /delete.",
		"consent_btn": "Согласен, начать",
		"deleted":     "Готово, данные этого чата удалены.",
	},
}

func T(lang, key string, args ...any) string {
	m, ok := botTexts[lang]
	if !ok {
		m = botTexts["en"]
	}
	s, ok := m[key]
	if !ok {
		s = botTexts["en"][key]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

var monthsRU = []string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
var weekdaysRU = []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func fmtWhen(lang string, t time.Time) string {
	if lang == "ru" {
		return fmt.Sprintf("%s, %d %s, %s", weekdaysRU[t.Weekday()], t.Day(), monthsRU[t.Month()], t.Format("15:04"))
	}
	return t.Format("Mon, Jan 2, 15:04")
}

func fmtMoney(currency string, amount int64) string {
	switch currency {
	case "XTR":
		return fmt.Sprintf("%d ⭐", amount)
	case "USD":
		if amount%100 == 0 {
			return fmt.Sprintf("$%d", amount/100)
		}
		return fmt.Sprintf("$%.2f", float64(amount)/100)
	case "RUB":
		if amount%100 == 0 {
			return fmt.Sprintf("%d ₽", amount/100)
		}
		return fmt.Sprintf("%.2f ₽", float64(amount)/100)
	}
	return fmt.Sprintf("%d %s", amount, currency)
}
