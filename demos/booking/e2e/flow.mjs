// Browser e2e of the whole demo through the Telegram emulator (and media for the portfolio with --media).
//   BASE=http://localhost:8091 node flow.mjs [--media]
import { chromium } from 'playwright'
import { mkdirSync } from 'node:fs'
import { execFileSync } from 'node:child_process'

const BASE = process.env.BASE || 'http://localhost:8091'
const MEDIA = process.argv.includes('--media')
const OUT = new URL('../media/', import.meta.url).pathname
const TMP = new URL('../.localdata/video/', import.meta.url).pathname
import { rmSync } from 'node:fs'
rmSync(TMP, { recursive: true, force: true })
mkdirSync(OUT, { recursive: true }); mkdirSync(TMP, { recursive: true })
const pause = (ms) => (MEDIA ? new Promise((r) => setTimeout(r, ms * 1.8)) : Promise.resolve())
const shot = async (page, name, opts = {}) => { if (MEDIA) await page.screenshot({ path: OUT + name, ...opts }) }

const browser = await chromium.launch()
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2, locale: 'en-US',
  ...(MEDIA ? { recordVideo: { dir: TMP, size: { width: 1280, height: 860 } } } : {}),
})
const page = await ctx.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
await page.addInitScript(() => { if (!sessionStorage.getItem('init')) { localStorage.clear(); localStorage.setItem('emu_lang', 'en'); localStorage.setItem('emu_name', 'Sofia'); sessionStorage.setItem('init', '1') } })

process.on('unhandledRejection', async (e) => { try { await page.screenshot({ path: TMP + '../fail.png' }) } catch {} console.error(String(e).split('\n')[0]); process.exit(1) })
const app = () => page.frameLocator('iframe[title="Mini App"]')
const step = (s) => console.log('·', s)

// ---- EN client: Stars ----
await page.goto(BASE)
await page.getByRole('button', { name: '📅 Book now' }).last().waitFor()
step('welcome message in chat'); await pause(1200)
await shot(page, '01-emulator-chat-en.png')
await page.getByRole('button', { name: '📅 Book now' }).last().click()
await app().getByText("Women's haircut & styling").click(); await pause(700)
await app().getByText('Anna', { exact: true }).click()
await app().locator('.slot').first().waitFor(); await pause(800)
const daysWithSlots = app().locator('.day:not(.empty)')
await daysWithSlots.nth(1).click(); await app().locator('.slot').first().waitFor(); await pause(500)
await app().locator('.slot').last().click(); await pause(600)
await shot(page, '02-miniapp-slots-en.png')
await app().getByRole('button', { name: 'Continue' }).click(); await pause(900)
await app().locator('.bar .btn').click()
await app().getByText('Confirm your purchase').waitFor(); await pause(1000)
await shot(page, '03-stars-payment-en.png')
await app().getByRole('button', { name: /Confirm and pay/ }).click()
await app().getByText('You are booked!').waitFor(); step('Stars payment → confirmed'); await pause(1500)
await shot(page, '04-booked-en.png')
await page.locator('.ma-head button').click()
await page.getByText('Booked!').last().waitFor(); step('confirmation in chat'); await pause(800)
await page.getByRole('button', { name: /Send me a reminder now/ }).click()
await page.locator('.bubble', { hasText: 'Reminder' }).last().waitFor(); step('reminder in chat'); await pause(1500)

// ---- RU client: card (YooKassa sandbox) ----
await page.getByRole('button', { name: 'RU', exact: true }).click(); await pause(800)
await page.getByRole('button', { name: '📅 Записаться' }).last().click()
await app().getByText('Маникюр с покрытием').click(); await pause(500)
await app().getByText('Любой свободный мастер').click()
await app().locator('.slot').first().waitFor(); await pause(500)
await app().locator('.slot').last().click()
await app().getByRole('button', { name: 'Далее' }).click(); await pause(500)
await app().getByText('Оплатить картой (ЮKassa)').click(); await pause(500)
await shot(page, '05-confirm-card-ru.png')
await app().locator('.bar .btn').click()
const checkout = app().frameLocator('.checkout iframe')
const payBtn = checkout.getByRole('button', { name: /^Оплатить/ })
await payBtn.waitFor(); await page.waitForTimeout(400); await pause(600)
await payBtn.click()
await app().getByText('Вы записаны!').waitFor({ timeout: 15000 }).catch(async (e) => { await page.screenshot({ path: TMP + '../fail.png' }); throw e }); step('card payment → confirmed'); await pause(1500)
await page.locator('.ma-head button').click(); await pause(800)

// ---- Owner: admin ----
await page.getByRole('button', { name: /Владелец салона/ }).click()
await page.getByRole('button', { name: 'EN', exact: true }).click()
await page.getByText('New booking').last().waitFor().catch(() => {})
const [admin] = await Promise.all([ctx.waitForEvent('page'), page.getByRole('button', { name: /admin panel/ }).click()])
await admin.locator('.appt').first().waitFor(); step('admin schedule'); await pause(800)
await admin.setViewportSize({ width: 1440, height: 900 })
await shot(admin, '06-admin-schedule-en.png')
await admin.getByRole('button', { name: 'Bookings' }).click(); await admin.locator('tbody tr').first().waitFor()
step('admin bookings')
await admin.getByRole('button', { name: 'Masters' }).click(); await admin.locator('.mcard').first().waitFor()
await admin.close()
await ctx.close()
await browser.close()

if (errors.length) { console.error('page errors:', errors); process.exit(1) }
console.log('e2e ok')

if (MEDIA) {
  const { readdirSync, statSync } = await import('node:fs')
  // main flow (largest recording) followed by the admin tab
  const vids = readdirSync(TMP).filter((f) => f.endsWith('.webm')).map((f) => TMP + f).sort((a, b) => statSync(b).size - statSync(a).size)
  const joined = TMP + 'joined.mp4'
  const inputs = vids.flatMap((v) => ['-i', v])
  const filter = vids.map((_, i) => `[${i}:v]scale=1280:860,setsar=1[v${i}]`).join(';') + ';' + vids.map((_, i) => `[v${i}]`).join('') + `concat=n=${vids.length}:v=1:a=0[out]`
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', ...inputs, '-filter_complex', filter, '-map', '[out]', '-c:v', 'libx264', '-crf', '18', '-pix_fmt', 'yuv420p', joined])
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', joined, '-c:v', 'libx264', '-crf', '28', '-preset', 'slow', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', '-an', OUT + 'demo.mp4'])
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', joined, '-vf', 'fps=8,scale=720:-2:flags=lanczos,split[a][b];[a]palettegen=max_colors=96[p];[b][p]paletteuse=dither=bayer', OUT + 'demo.gif'])
  console.log('media →', OUT)
}
