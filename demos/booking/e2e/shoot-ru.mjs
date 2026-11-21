// RU stills for covers: same frame/size as 01/02 EN.   BASE=http://localhost:8091 node shoot-ru.mjs
import { chromium } from 'playwright'
const BASE = process.env.BASE || 'http://localhost:8091'
const OUT = new URL('../media/', import.meta.url).pathname
const browser = await chromium.launch()
const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2, locale: 'ru-RU' })
const page = await ctx.newPage()
await page.addInitScript(() => { if (!sessionStorage.getItem('init')) { localStorage.clear(); localStorage.setItem('emu_lang', 'ru'); localStorage.setItem('emu_name', 'София'); sessionStorage.setItem('init', '1') } })
const app = () => page.frameLocator('iframe[title="Mini App"]')
await page.goto(BASE)
await page.getByRole('button', { name: '📅 Записаться' }).last().waitFor(); await page.waitForTimeout(800)
await page.screenshot({ path: OUT + '01-emulator-chat-ru.png' })
await page.getByRole('button', { name: '📅 Записаться' }).last().click()
await app().getByText('Женская стрижка и укладка').click(); await page.waitForTimeout(500)
await app().getByText('Анна', { exact: true }).click()
await app().locator('.slot').first().waitFor()
await app().locator('.day:not(.empty)').nth(1).click(); await app().locator('.slot').first().waitFor(); await page.waitForTimeout(400)
await app().locator('.slot').last().click(); await page.waitForTimeout(600)
await page.screenshot({ path: OUT + '02-miniapp-slots-ru.png' })
await browser.close()
console.log('ok')
