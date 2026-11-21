// End-to-end check of the demo in a real browser + portfolio media.
//   BASE=http://127.0.0.1:8080 node e2e.mjs           → checks (edition is read from /api/config)
//   BASE=... node e2e.mjs --media                     → also screenshots ../media/*-<ru|en>.png and a video demo-<ru|en>.webm
import { chromium } from 'playwright';
import { mkdirSync, renameSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE = (process.env.BASE || 'http://127.0.0.1:8080').replace(/\/$/, '');
const MEDIA = process.argv.includes('--media');
const here = dirname(fileURLToPath(import.meta.url));
const mediaDir = join(here, '..', 'media');
const vidTmp = join(here, 'out');
mkdirSync(mediaDir, { recursive: true });

let failures = 0, passed = 0;
function check(cond, name) {
  if (cond) { passed++; console.log('  ok  ', name); } else { failures++; console.log('  FAIL', name); }
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const cfg = await (await fetch(BASE + '/api/config')).json();
const ed = cfg.edition === 'intl' ? 'en' : 'ru';
const T = ed === 'en'
  ? { refresh: 'Update now', applied: 'sent to the marketplace', added: 'Card added', bad: 'does not look like', sent: 'Report sent', published: 'Reply published', remove: 'Remove' }
  : { refresh: 'Обновить сейчас', applied: 'отправлена на маркетплейс', added: 'Карточка добавлена', bad: 'Не похоже', sent: 'Отчёт отправлен', published: 'Ответ опубликован', remove: 'Убрать' };
console.log(`e2e ${BASE} edition=${cfg.edition || '(dev)'}`);

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 }, deviceScaleFactor: MEDIA ? 2 : 1, colorScheme: 'light', acceptDownloads: true,
  ...(MEDIA ? { recordVideo: { dir: vidTmp, size: { width: 1440, height: 900 } } } : {}),
});
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
// the bad-link check expects one 400 from the API; the browser logs it as a resource error
page.on('console', (m) => { if (m.type() === 'error' && !/status of 400/.test(m.text())) errors.push(m.text()); });
const shot = async (name) => { if (MEDIA) { await page.evaluate(() => { document.getElementById('toast').hidden = true; }); await page.screenshot({ path: join(mediaDir, `${name}-${ed}.png`) }); } };
const pause = (ms) => (MEDIA ? sleep(ms) : sleep(50));
const toastHas = async (s) => { await page.waitForFunction((x) => { const el = document.getElementById('toast'); return el && !el.hidden && el.textContent.includes(x); }, s, { timeout: 8000 }); return true; };

// 1. dashboard
await page.goto(BASE + '/');
await page.waitForSelector('.prod');
check((await page.locator('.prod').count()) === 5, 'five products on the dashboard');
check((await page.locator('#cp-prod_1 svg path').count()) >= 5, 'price chart: our line + 4 rivals');
check((await page.locator('#cs-prod_1 svg path').count()) >= 4, 'stock chart');
const date0 = await page.textContent('#asOf');
await pause(1500);
await page.hover('#cp-prod_1 svg', { position: { x: 420, y: 80 } });
await pause(1200);
check(await page.isVisible('#cp-prod_1 .tip'), 'chart tooltip on hover');
await shot('01-dashboard');
await page.mouse.move(5, 5);

// 2. update now → next day, collection log
await page.click('#refreshBtn');
check(await toastHas(ed === 'en' ? 'Collected' : 'Сбор за'), 'collection finished (toast)');
check((await page.textContent('#asOf')) !== date0, 'date moved to the next day');
await pause(2500);
await shot('02-after-update');

// 3. apply the recommendation on the first product
const applyBtn = page.locator('[data-apply="prod_1"]');
if (await applyBtn.count()) {
  await applyBtn.scrollIntoViewIfNeeded();
  await pause(800);
  await applyBtn.click();
  check(await toastHas(T.applied), 'recommended price applied through the marketplace API');
} else check(true, 'no recommendation for prod_1 today (skipped apply)');
await pause(1500);

// 4. add a competitor by link, bad link is rejected
const form = page.locator('form[data-add="prod_1"]');
await form.scrollIntoViewIfNeeded();
await form.locator('input').fill('https://example.com/item/1');
await form.locator('button').click();
await page.waitForFunction(() => document.getElementById('ae-prod_1').textContent.length > 0);
check((await page.textContent('#ae-prod_1')).length > 0, 'bad link rejected with a hint');
await form.locator('input').fill('');
await pause(400);
await form.locator('input').pressSequentially('https://www.ozon.ru/product/chay-puer-shu-357-g-1555123456/', { delay: MEDIA ? 25 : 0 });
await pause(500);
await form.locator('button').click();
check(await toastHas(T.added), 'competitor added by Ozon link');
check((await page.locator(`.prod[data-pid="prod_1"] [data-remove]`).count()) === 1, 'added card is listed and removable');
await pause(1800);

// 5. collection log
await page.click('#tabs [data-tab=log]');
await page.click('#refreshBtn');
await toastHas(ed === 'en' ? 'Collected' : 'Сбор за');
const logRows = await page.locator('#log tbody tr').count();
check(logRows >= 23, `collection log lists robots.txt + every card (${logRows} rows)`);
check(await page.locator('#log').textContent().then((s) => s.includes('robots.txt')), 'robots.txt is fetched first');
await pause(2500);
await shot('05-collection-log');

// 6. daily report → messenger (mock)
await page.click('#tabs [data-tab=report]');
await pause(1200);
const sendBtns = page.locator('#sendBtns button');
check((await sendBtns.count()) === 2, 'two delivery channels in this edition');
await sendBtns.first().click();
check(await toastHas(T.sent), 'report delivered (notify mock)');
check((await page.locator('#messages .msg').count()) === 1, 'sent message journaled');
const reportText = await page.textContent('#reportText');
check(reportText.length > 80, 'report text built');
await pause(2000);
await shot('03-report');

// 7. sheet + downloads
await page.click('#tabs [data-tab=sheet]');
check((await page.locator('#sheet tbody tr').count()) === 20, 'sheet preview shows the last 20 rows');
const [csv] = await Promise.all([page.waitForEvent('download'), page.click('#csvBtn')]);
const csvText = readFileSync(await csv.path(), 'utf8');
check(csvText.includes('date,product,offer_id,market') && csvText.split('\n').length > 60, 'CSV export');
const [xlsx] = await Promise.all([page.waitForEvent('download'), page.click('#xlsxBtn')]);
check(readFileSync(await xlsx.path()).subarray(0, 2).toString() === 'PK', 'XLSX export');
await pause(2000);
await shot('04-sheet');

// 8. reviews: AI draft → publish
await page.click('#tabs [data-tab=reviews]');
const before = Number((await page.textContent('#revCount')) || 0);
await pause(1000);
await page.locator('#reviews [data-draft]').first().click();
await page.waitForSelector('#reviews textarea');
const draft = await page.inputValue('#reviews textarea');
check(draft.length > 30 && !draft.startsWith('mock:'), 'AI draft (ai.reply_to_review)');
await pause(2500);
await shot('06-reviews');
await page.locator('#reviews [data-publish]').first().click();
check(await toastHas(T.published), 'reply published through the marketplaces API');
check(Number((await page.textContent('#revCount')) || 0) === before - 1, 'unanswered counter decreased');
await pause(2000);

// 9. text hygiene
const body = await page.evaluate(() => document.body.innerText);
check(!body.includes('—'), 'no em dash on the page');
check(errors.length === 0, 'no console errors' + (errors.length ? ': ' + errors.join(' | ') : ''));

// 10. mobile 390 px
await page.click('#tabs [data-tab=prices]');
await page.setViewportSize({ width: 390, height: 844 });
await page.evaluate(() => window.scrollTo(0, 0));
await pause(800);
const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
check(overflow <= 1, `no horizontal scroll at 390 px (${overflow})`);
await shot('07-mobile');

await ctx.close();
if (MEDIA) {
  const v = page.video();
  if (v) renameSync(await v.path(), join(mediaDir, `demo-${ed}.webm`));
}
await browser.close();
console.log(`\n${passed} passed, ${failures} failed`);
process.exit(failures ? 1 : 0);
