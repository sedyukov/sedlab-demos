// Browser e2e through the Telegram/Max emulator, per edition (and media for the portfolio with --media).
//   BASE=http://localhost:8091 EDITION=intl node flow.mjs [--media]   → media/*-en.png, demo-en.mp4/gif
//   BASE=http://localhost:8092 EDITION=ru   node flow.mjs [--media]   → media/*-ru.png, demo-ru.mp4/gif
import { chromium } from 'playwright'
import { mkdirSync, rmSync, readdirSync, statSync } from 'node:fs'
import { execFileSync } from 'node:child_process'

const BASE = process.env.BASE || 'http://localhost:8091'
const ED = process.env.EDITION === 'ru' ? 'ru' : 'intl'
const SFX = ED === 'ru' ? 'ru' : 'en'
const MEDIA = process.argv.includes('--media')
const OUT = new URL('../media/', import.meta.url).pathname
const TMP = new URL(`../.localdata/video-${SFX}/`, import.meta.url).pathname
rmSync(TMP, { recursive: true, force: true })
mkdirSync(OUT, { recursive: true }); mkdirSync(TMP, { recursive: true })
const pause = (ms) => (MEDIA ? new Promise((r) => setTimeout(r, ms * 1.8)) : Promise.resolve())
const shot = async (p, name) => { if (MEDIA) await p.screenshot({ path: OUT + name }) }

const browser = await chromium.launch()
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2, locale: ED === 'ru' ? 'ru-RU' : 'en-US',
  ...(MEDIA ? { recordVideo: { dir: TMP, size: { width: 1280, height: 860 } } } : {}),
})
const page = await ctx.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
const guestName = ED === 'ru' ? 'София' : 'Sofia'
await page.addInitScript((n) => { if (!sessionStorage.getItem('init')) { localStorage.clear(); localStorage.setItem('emu_name', n); sessionStorage.setItem('init', '1') } }, guestName)

const app = () => page.frameLocator('iframe[title="Mini App"]')
const step = (s) => console.log('·', s)
const T = ED === 'ru'
  ? { book: '📅 Записаться', next: 'Далее', booked: 'Вы записаны!', any: 'Любой свободный мастер', card: 'Оплатить картой', pay: /^Оплатить/, owner: /Глазами владельца/, admin: /Открыть админку/, remind: /придёт напоминание/, reminder: 'Напоминание', chatBooked: 'Вы записаны' }
  : { book: '📅 Book now', next: 'Continue', booked: 'You are booked!', any: 'Any available master', card: 'Pay by card (Stripe)', pay: /^Pay/, owner: /Salon owner/, admin: /admin panel/, remind: /Send me a reminder now/, reminder: 'Reminder', chatBooked: 'Booked!' }

async function pickSlot(service, master, dayIndex = 1) {
  await page.getByRole('button', { name: T.book }).last().click()
  await app().getByText(service).click(); await pause(600)
  await app().getByText(master, { exact: true }).click()
  await app().locator('.slot').first().waitFor(); await pause(600)
  const days = app().locator('.day:not(.empty)')
  if (dayIndex > 0 && (await days.count()) > dayIndex) { await days.nth(dayIndex).click(); await app().locator('.slot').first().waitFor(); await pause(400) }
  await app().locator('.slot').last().click(); await pause(600)
}

async function payByCard() {
  await app().getByText(T.card).click(); await pause(500)
  await app().locator('.bar .btn').click()
  const btn = app().frameLocator('.checkout iframe').getByRole('button', { name: T.pay })
  await btn.waitFor(); await page.waitForTimeout(400); await pause(600)
  await btn.click()
  await app().getByText(T.booked).waitFor({ timeout: 15000 })
}

const closeApp = () => page.locator('.ma-head button').click()

async function consentIfAsked() {
  if (ED !== 'ru') return
  const btn = page.getByRole('button', { name: 'Согласен, начать' }).last()
  await btn.waitFor(); await pause(1000)
  await btn.click(); step('consent bot-v1 pressed')
}

await page.goto(BASE)
await consentIfAsked()
await page.getByRole('button', { name: T.book }).last().waitFor()
step('welcome message in chat'); await pause(1200)
await shot(page, `01-emulator-chat-${SFX}.png`)

if (ED === 'intl') {
  // Telegram Stars
  await pickSlot("Women's haircut & styling", 'Anna')
  await shot(page, '02-miniapp-slots-en.png')
  await app().getByRole('button', { name: T.next }).click(); await pause(900)
  await app().getByText('Pay with Telegram Stars').click()
  await app().locator('.bar .btn').click()
  await app().getByText('Confirm your purchase').waitFor(); await pause(1000)
  await shot(page, '03-stars-payment-en.png')
  await app().getByRole('button', { name: /Confirm and pay/ }).click()
  await app().getByText(T.booked).waitFor(); step('Stars payment → confirmed'); await pause(1500)
  await shot(page, '04-booked-en.png')
  await closeApp()
  await page.locator('.bubble', { hasText: T.chatBooked }).last().waitFor(); step('confirmation in chat'); await pause(800)
  // card (Stripe sandbox)
  await pickSlot('Gel manicure', T.any, 0)
  await app().getByRole('button', { name: T.next }).click(); await pause(600)
  await app().getByText(T.card).click(); await pause(400)
  await shot(page, '05-confirm-card-en.png')
  await payByCard(); step('card (Stripe) → confirmed'); await pause(1200)
  await closeApp()
  if (await page.getByRole('button', { name: 'Max', exact: true }).count()) throw new Error('Max offered in intl edition')
} else {
  // card (YooKassa sandbox) in Telegram
  await pickSlot('Женская стрижка и укладка', 'Анна')
  await shot(page, '02-miniapp-slots-ru.png')
  await app().getByRole('button', { name: T.next }).click(); await pause(900)
  if (await app().getByText('Оплатить Telegram Stars').count()) throw new Error('Stars offered in ru edition')
  await shot(page, '05-confirm-card-ru.png')
  await payByCard(); step('card (imitation) → confirmed'); await pause(1500)
  await shot(page, '04-booked-ru.png')
  await closeApp()
  await page.locator('.bubble', { hasText: T.chatBooked }).last().waitFor(); step('confirmation in chat'); await pause(800)
  // Max
  await page.getByRole('button', { name: 'Max', exact: true }).click()
  await page.locator('.chat-head', { hasText: 'Max' }).waitFor()
  await consentIfAsked()
  await page.getByRole('button', { name: T.book }).last().waitFor(); await pause(1000)
  await pickSlot('Мужская стрижка', T.any, 0)
  await app().getByRole('button', { name: T.next }).click(); await pause(600)
  await shot(page, '07-max-confirm-ru.png')
  await payByCard(); step('Max: card → confirmed'); await pause(1200)
  await closeApp()
  await page.locator('.bubble', { hasText: T.chatBooked }).last().waitFor(); await pause(800)
  await page.getByRole('button', { name: 'Telegram', exact: true }).click(); await pause(500)
  await page.getByRole('button', { name: T.book }).last().waitFor()
  // honesty: no provider names or fake card on the imitated checkout; sign-in stub is hidden
  if (await page.getByRole('button', { name: 'Войти и попробовать вживую' }).count()) throw new Error('accounts stub shown')
}

await page.getByRole('button', { name: T.remind }).click()
await page.locator('.bubble', { hasText: T.reminder }).last().waitFor(); step('reminder in chat'); await pause(1500)

// owner: admin
await page.getByRole('button', { name: T.owner }).click()
await page.getByRole('button', { name: T.admin }).waitFor(); await pause(800)
const [admin] = await Promise.all([ctx.waitForEvent('page'), page.getByRole('button', { name: T.admin }).click()])
await admin.locator('.appt').first().waitFor().catch(async (e) => { await admin.screenshot({ path: TMP + '../fail.png' }); throw e }); step('admin schedule'); await pause(800)
await shot(admin, `06-admin-schedule-${SFX}.png`)
await admin.locator('nav button').nth(1).click(); await admin.locator('tbody tr').first().waitFor(); await pause(800)
step('admin bookings')
await admin.locator('nav button').nth(3).click(); await admin.locator('.mcard').first().waitFor(); await pause(600)
await admin.close()
await ctx.close()
await browser.close()

if (errors.length) { console.error('page errors:', errors); process.exit(1) }
console.log(`e2e ok (${ED})`)

if (MEDIA) {
  // main flow (largest recording) followed by the admin tab
  const vids = readdirSync(TMP).filter((f) => f.endsWith('.webm')).map((f) => TMP + f).sort((a, b) => statSync(b).size - statSync(a).size)
  const joined = TMP + 'joined.mp4'
  const inputs = vids.flatMap((v) => ['-i', v])
  const filter = vids.map((_, i) => `[${i}:v]scale=1280:860,setsar=1[v${i}]`).join(';') + ';' + vids.map((_, i) => `[v${i}]`).join('') + `concat=n=${vids.length}:v=1:a=0[out]`
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', ...inputs, '-filter_complex', filter, '-map', '[out]', '-c:v', 'libx264', '-crf', '18', '-pix_fmt', 'yuv420p', joined])
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', joined, '-c:v', 'libx264', '-crf', '28', '-preset', 'slow', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', '-an', OUT + `demo-${SFX}.mp4`])
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', joined, '-vf', 'fps=8,scale=720:-2:flags=lanczos,split[a][b];[a]palettegen=max_colors=96[p];[b][p]paletteuse=dither=bayer', OUT + `demo-${SFX}.gif`])
  console.log('media →', OUT)
}
