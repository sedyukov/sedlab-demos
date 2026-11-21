export function money(currency, amount, lang = 'en') {
  if (currency === 'XTR') return `${amount.toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-US')} ⭐`
  if (currency === 'RUB') return `${(amount / 100).toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-US', { maximumFractionDigits: 2 })} ₽`
  return `${amount} ${currency}`
}

export function fmtDay(ts, tz, lang, opts = { weekday: 'short', day: 'numeric', month: 'short' }) {
  return new Intl.DateTimeFormat(lang === 'ru' ? 'ru-RU' : 'en-GB', { ...opts, timeZone: tz }).format(new Date(ts * 1000))
}

export function fmtTime(ts, tz) {
  return new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', timeZone: tz }).format(new Date(ts * 1000))
}

export function fmtWhen(ts, tz, lang) {
  return `${fmtDay(ts, tz, lang, { weekday: 'short', day: 'numeric', month: 'long' })}, ${fmtTime(ts, tz)}`
}

// "2026-11-24" → Date at local noon (for weekday/day labels)
export function dateLabel(iso, lang, tz) {
  const [y, m, d] = iso.split('-').map(Number)
  const ts = Date.UTC(y, m - 1, d, 12) / 1000
  return {
    wd: new Intl.DateTimeFormat(lang === 'ru' ? 'ru-RU' : 'en-GB', { weekday: 'short', timeZone: 'UTC' }).format(new Date(ts * 1000)),
    day: d,
    mon: new Intl.DateTimeFormat(lang === 'ru' ? 'ru-RU' : 'en-GB', { month: 'short', timeZone: 'UTC' }).format(new Date(ts * 1000)),
  }
}

export const durationLabel = (min, lang) => {
  const h = Math.floor(min / 60), m = min % 60
  if (lang === 'ru') return [h ? `${h} ч` : '', m ? `${m} мин` : ''].filter(Boolean).join(' ')
  return [h ? `${h} h` : '', m ? `${m} min` : ''].filter(Boolean).join(' ')
}
