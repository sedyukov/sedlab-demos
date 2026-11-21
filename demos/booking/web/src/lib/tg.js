// Telegram WebApp adapter. In Telegram → window.Telegram.WebApp (telegram-web-app.js).
// In the browser emulator (platform "unknown") → initData from the URL hash (#tgWebAppData=…, signed by the
// server-side emulator via telegram-core) and in-page sheets for Stars invoices and the checkout page.
const W = typeof window !== 'undefined' ? window.Telegram?.WebApp : undefined

function hashParam(name) {
  return new URLSearchParams(location.hash.slice(1)).get(name) || ''
}

const hashData = hashParam('tgWebAppData')
if (hashData) { try { sessionStorage.setItem('tgWebAppData', hashData) } catch {} }

export const isTelegram = !!(W && W.initData && W.platform && W.platform !== 'unknown')
export const isEmulator = !isTelegram

export function initData() {
  if (isTelegram) return W.initData
  if (hashData) return hashData
  try { return sessionStorage.getItem('tgWebAppData') || '' } catch { return '' }
}

export function ready() {
  if (!W) return
  try { W.ready(); W.expand(); W.setHeaderColor?.('secondary_bg_color') } catch {}
}

export function haptic(kind = 'light') {
  try { if (isTelegram) kind === 'success' || kind === 'error' ? W.HapticFeedback.notificationOccurred(kind) : W.HapticFeedback.impactOccurred(kind) } catch {}
}

export function colorScheme() {
  if (isTelegram) return W.colorScheme
  return new URLSearchParams(location.search).get('theme') || 'light'
}

// openInvoice(url) → Promise<'paid'|'cancelled'|'failed'>. Emulator: onEmulatorInvoice renders the sheet.
let emuInvoiceHandler = null
export const onEmulatorInvoice = (fn) => (emuInvoiceHandler = fn)
export function openInvoice(url) {
  if (isTelegram) return new Promise((resolve) => W.openInvoice(url, (status) => resolve(status)))
  return emuInvoiceHandler ? emuInvoiceHandler(url) : Promise.resolve('failed')
}

// openCheckout(url) — external card checkout. Telegram: opens the browser and the app polls.
// Emulator: onEmulatorCheckout shows the (mock) checkout page in an overlay.
let emuCheckoutHandler = null
export const onEmulatorCheckout = (fn) => (emuCheckoutHandler = fn)
export function openCheckout(url) {
  if (isTelegram) { W.openLink(url); return Promise.resolve('opened') }
  return emuCheckoutHandler ? emuCheckoutHandler(url) : (location.href = url)
}

export function close() { if (isTelegram) W.close(); else window.parent?.postMessage({ type: 'emu_close' }, '*') }
