// UI dictionaries (EN + RU). Bot texts live in server/i18n.go.
const dict = {
  en: {
    salon: 'Bloom Salon', tagline: 'Book in 30 seconds',
    services: 'Choose a service', master: 'Choose a master', anyMaster: 'Any available master', anyMasterHint: 'Earliest free time',
    time: 'Pick a date & time', noSlots: 'No free time this day — try another date', confirm: 'Confirm booking',
    name: 'Your name', phone: 'Phone (optional)', min: 'min', with: 'with', back: 'Back', next: 'Continue',
    prepay: 'Prepayment', prepayHint: '{p}% deposit secures your slot. Free cancellation up to 24 h before.',
    payStars: 'Pay with Telegram Stars', payCard: 'Pay by card (YooKassa)', total: 'Service price', deposit: 'Deposit now',
    held: 'Slot held for {m}:{s}', holdExpired: 'The hold has expired — pick a time again', pay: 'Pay {amount}',
    my: 'My bookings', none: 'No bookings yet', book: 'Book a visit', bookAnother: 'Book another visit',
    st_pending: 'Awaiting payment', st_confirmed: 'Confirmed', st_completed: 'Completed', st_cancelled: 'Cancelled', st_expired: 'Expired',
    confirmedTitle: 'You are booked!', confirmedText: 'We sent the details to the chat and will remind you a day and 2 hours before.',
    waitingPay: 'Waiting for payment confirmation…', cancel: 'Cancel booking', cancelConfirm: 'Cancel this booking?',
    refundYes: 'The deposit will be refunded.', refundNo: 'Less than 24 h left — the deposit is not refunded.', refunded: 'Deposit refunded',
    paid: 'Paid', starsTitle: 'Confirm your purchase', starsBalance: 'Demo balance: 5 000 ⭐', starsPay: 'Confirm and pay',
    starsNote: 'Demo: no real Stars are spent. In Telegram this is the native payment sheet.', close: 'Close',
    err: 'Something went wrong', slotTaken: 'This time was just taken — choose another one', reopen: 'Session expired — reopen the app',
    today: 'Today', tomorrow: 'Tomorrow',
  },
  ru: {
    salon: 'Салон Bloom', tagline: 'Запись за 30 секунд',
    services: 'Выберите услугу', master: 'Выберите мастера', anyMaster: 'Любой свободный мастер', anyMasterHint: 'Ближайшее время',
    time: 'Выберите дату и время', noSlots: 'На этот день свободного времени нет — выберите другую дату', confirm: 'Подтверждение',
    name: 'Ваше имя', phone: 'Телефон (необязательно)', min: 'мин', with: 'мастер', back: 'Назад', next: 'Далее',
    prepay: 'Предоплата', prepayHint: 'Предоплата {p}% закрепляет время. Бесплатная отмена — не позднее чем за 24 часа.',
    payStars: 'Оплатить Telegram Stars', payCard: 'Оплатить картой (ЮKassa)', total: 'Стоимость услуги', deposit: 'Предоплата сейчас',
    held: 'Время закреплено ещё {m}:{s}', holdExpired: 'Время брони истекло — выберите слот заново', pay: 'Оплатить {amount}',
    my: 'Мои записи', none: 'Записей пока нет', book: 'Записаться', bookAnother: 'Записаться ещё',
    st_pending: 'Ожидает оплаты', st_confirmed: 'Подтверждена', st_completed: 'Завершена', st_cancelled: 'Отменена', st_expired: 'Истекла',
    confirmedTitle: 'Вы записаны!', confirmedText: 'Детали отправили в чат, напомним за сутки и за 2 часа до визита.',
    waitingPay: 'Ждём подтверждения оплаты…', cancel: 'Отменить запись', cancelConfirm: 'Отменить запись?',
    refundYes: 'Предоплата вернётся.', refundNo: 'До визита меньше 24 часов — предоплата не возвращается.', refunded: 'Предоплата возвращена',
    paid: 'Оплачено', starsTitle: 'Подтвердите покупку', starsBalance: 'Демо-баланс: 5 000 ⭐', starsPay: 'Подтвердить и оплатить',
    starsNote: 'Демо: настоящие Stars не списываются. В Telegram здесь нативное окно оплаты.', close: 'Закрыть',
    err: 'Что-то пошло не так', slotTaken: 'Это время только что заняли — выберите другое', reopen: 'Сессия истекла — откройте приложение заново',
    today: 'Сегодня', tomorrow: 'Завтра',
  },
}

export function t(lang, key, vars = {}) {
  let s = (dict[lang] || dict.en)[key] ?? dict.en[key] ?? key
  for (const [k, v] of Object.entries(vars)) s = s.replace(`{${k}}`, v)
  return s
}

export const pick = (lang, obj) => (obj ? obj[lang] ?? obj.en : '')
