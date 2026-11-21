// End-to-end check of the live demo in a real browser + portfolio media.
//   node e2e.mjs                    → assertions against BASE (default http://127.0.0.1:8090)
//   node e2e.mjs --media            → also screenshots (../media/*.png) and a video (../media/demo.webm)
//   BASE=https://leads.demo.sedlab.dev node e2e.mjs
import { chromium } from 'playwright';
import { mkdirSync, renameSync, readdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE = process.env.BASE || 'http://127.0.0.1:8090';
const MEDIA = process.argv.includes('--media');
const here = dirname(fileURLToPath(import.meta.url));
const mediaDir = join(here, '..', 'media');
const vidTmp = join(here, 'out', 'video');
mkdirSync(mediaDir, { recursive: true });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const pace = (ms) => (MEDIA ? sleep(ms * 1.6) : Promise.resolve());
let failures = 0;
function check(cond, msg) {
  console.log((cond ? 'ok   ' : 'FAIL ') + msg);
  if (!cond) failures++;
}

async function shot(page, name) {
  if (MEDIA) await page.screenshot({ path: join(mediaDir, name), fullPage: false });
}

async function waitStatus(page, leadId, status, timeout = 15000) {
  await page.waitForFunction(([id, st]) => {
    const s = window.leadsDemo && window.leadsDemo.state;
    return s && (s.leads || []).some((l) => l.id === id && l.status === st);
  }, [leadId, status], { timeout });
}

async function submit(page) {
  const w = page.locator('#widget');
  if (await w.locator('.done').count()) await w.locator('.done button').click();
  await w.locator('[data-demo]').click();
  await pace(900);
  await w.locator('button.send').click();
  await w.locator('.done').waitFor();
  const text = await w.locator('.done p').innerText();
  return text.match(/L-\d+/)[0];
}

async function scenario(browser, { lang, stack, video }) {
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 900 }, deviceScaleFactor: MEDIA ? 2 : 1,
    colorScheme: 'light', ...(video ? { recordVideo: { dir: vidTmp, size: { width: 1440, height: 900 } } } : {}),
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`${BASE}/?lang=${lang}&stack=${stack}`);
  await page.waitForSelector('#flow li');
  await page.locator('#liveDot.on').waitFor();
  await pace(1200);

  // 1. Lead → CRM → sheet → Telegram → payment link
  const id1 = await submit(page);
  await waitStatus(page, id1, 'awaiting_payment');
  check(true, `[${stack}] ${id1} reached awaiting_payment`);
  await pace(1500);
  await shot(page, `01-lead-flow-${stack}.png`);
  const logCount = await page.locator('#log li').count();
  check(logCount >= 8, `[${stack}] live log has ${logCount} events`);
  check(await page.locator('#flow li[data-s="1"]').count() >= 6, `[${stack}] pipeline nodes lit`);

  // 2. Failures → retries
  await page.locator('label.switch').click();
  await pace(600);
  const id2 = await submit(page);
  await waitStatus(page, id2, 'awaiting_payment', 30000);
  const retries = await page.locator('#log .b-retry').count();
  check(retries >= 4, `[${stack}] retries visible in log: ${retries}`);
  await pace(800);
  await shot(page, `02-retries-${stack}.png`);
  await page.locator('label.switch').click();

  // 3. Sheet + Telegram tabs
  await page.locator('.tabs [data-tab="sheet"]').click();
  const rows = await page.locator('#tab-sheet tbody tr').count();
  check(rows === 3, `[${stack}] sheet has header + 2 rows (${rows})`);
  await pace(1500);
  await shot(page, `03-sheet-${stack}.png`);
  await page.locator('.tabs [data-tab="inbox"]').click();
  check(await page.locator('#tab-inbox .bubble').count() >= 2, `[${stack}] notifications delivered`);
  await pace(1500);

  // 4. Pay → webhook → deal won; replay (duplicate) and forged (rejected)
  await page.locator('.tabs [data-tab="pay"]').click();
  await pace(800);
  await page.locator(`#tab-pay [data-act="pay"][data-id="${id1}"]`).click();
  await page.locator('#checkout[open]').waitFor();
  await pace(1200);
  await shot(page, `04-checkout-${stack}.png`);
  await page.locator('#checkout button[value="succeeded"]').click();
  await waitStatus(page, id1, 'paid');
  await page.waitForFunction(() => document.querySelectorAll('#log li').length && [...document.querySelectorAll('#log .ev-step')].some((e) => e.textContent.includes('crm.update_deal')));
  check(true, `[${stack}] ${id1} paid → deal moved`);
  await pace(1800);
  await shot(page, `05-deal-won-${stack}.png`);
  await page.locator('.tabs [data-tab="pay"]').click();
  await pace(600);
  await page.locator(`#tab-pay [data-act="replay-webhook"][data-id="${id1}"]`).click();
  await page.locator('#log .b-duplicate').first().waitFor();
  check(true, `[${stack}] replayed webhook marked duplicate`);
  await pace(1200);
  await page.locator(`#tab-pay [data-act="forge-webhook"][data-id="${id1}"]`).click();
  await page.locator('#log .b-rejected').first().waitFor();
  check(true, `[${stack}] forged webhook rejected`);
  await pace(2500);
  await shot(page, `06-webhooks-${stack}.png`);
  check(errors.length === 0, `[${stack}] no JS errors ${errors.join('; ')}`);

  // mobile layout sanity
  if (!video) {
    await page.setViewportSize({ width: 390, height: 844 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    check(overflow <= 0, `[${stack}] no horizontal scroll at 390px (${overflow})`);
    if (MEDIA) await page.screenshot({ path: join(mediaDir, `07-mobile-${stack}.png`), fullPage: false });
  }
  const vid = video ? page.video() : null;
  await ctx.close();
  if (vid) {
    const p = await vid.path();
    renameSync(p, join(mediaDir, `demo-${stack}.webm`));
  }
}

const browser = await chromium.launch();
try {
  rmSync(vidTmp, { recursive: true, force: true });
  await scenario(browser, { lang: 'ru', stack: 'ru', video: MEDIA });
  await scenario(browser, { lang: 'en', stack: 'intl', video: MEDIA });
} finally {
  await browser.close();
}
console.log(failures ? `\n${failures} check(s) failed` : '\nall e2e checks passed');
process.exit(failures ? 1 : 0);
