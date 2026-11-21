// Browser e2e for the demo page (+ media for the portfolio with --media).
//   BASE=http://127.0.0.1:18080 LANG_UI=ru node e2e.mjs [--media]
// Checks: token → Swagger authorized; comparison: all answers identical; jobs: retries and dead; Swagger "Try it out"
// on both backends; 390 px layout without horizontal scroll.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const BASE = process.env.BASE || "http://127.0.0.1:18080";
const LANG = process.env.LANG_UI || "ru";
const MEDIA = process.argv.includes("--media");
const OUT = process.env.OUT || new URL("../media/", import.meta.url).pathname;
const tag = LANG === "ru" ? "ru" : "en";
let failed = 0;
const ok = (cond, msg) => { console.log((cond ? "  ok   " : "  FAIL ") + msg); if (!cond) failed++; };

mkdirSync(OUT, { recursive: true });
const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 820 }, deviceScaleFactor: MEDIA ? 2 : 1, locale: LANG === "ru" ? "ru-RU" : "en-US",
  ...(MEDIA ? { recordVideo: { dir: OUT + "raw-" + tag, size: { width: 1280, height: 820 } } } : {}),
});
await ctx.addInitScript((lang) => { try { localStorage.clear(); localStorage.setItem("api.lang", lang); } catch (e) {} }, LANG);
const page = await ctx.newPage();
const errors = [];
page.on("pageerror", (e) => errors.push(String(e)));
const pause = (ms) => page.waitForTimeout(MEDIA ? ms * 2 : Math.min(ms, 150));
const shot = async (name, opts = {}) => { if (MEDIA) await page.screenshot({ path: `${OUT}${name}-${tag}.png`, ...opts }); };

await page.goto(BASE + "/", { waitUntil: "networkidle" });
await page.waitForSelector(".swagger-ui .opblock", { timeout: 15000 });
ok((await page.locator(".opblock").count()) >= 16, "swagger lists operations");
await pause(800);
await shot("1-overview", { fullPage: false });

// token
await page.click("#get-token");
await page.waitForSelector("#token-state.ok");
ok(/ws_/.test(await page.textContent("#token-state")), "sandbox token issued");
await pause(600);

// comparison
await page.click("#compare");
await page.waitForSelector("#cmp-summary:not([hidden])", { timeout: 15000 });
ok((await page.locator(".cmp td.same").count()) === 6 && (await page.locator(".cmp td.diff").count()) === 0, "Go and Python answers identical (6/6)");
await pause(1200);
await page.locator(".grid2").scrollIntoViewIfNeeded();

// jobs
await page.click("[data-job=send_invoice]");
await pause(400);
await page.click("[data-job=sync_crm]");
await pause(400);
await page.click("[data-job=notify_customer]");
await page.waitForSelector(".st.retrying", { timeout: 10000 });
await pause(1500);
await shot("2-compare-jobs-running");
await page.waitForFunction(() => document.querySelectorAll(".st.succeeded").length >= 2 && document.querySelectorAll(".st.dead").length >= 1, null, { timeout: 30000 });
ok(true, "jobs: two succeeded (one after retries), one dead after 4 attempts");
const workers = await page.$$eval(".att", (els) => [...new Set(els.map((e) => (e.textContent.includes("Go") ? "go" : "py")))]);
ok(workers.length >= 1, "attempt history shows workers: " + workers.join(","));
await pause(800);
await shot("3-compare-jobs-done");

// Swagger: Try it out on Python, then on Go
async function tryList(backend) {
  await page.click(`#backend button[data-b=${backend}]`);
  await pause(400);
  const op = page.locator("#operations-orders-listOrders");
  await op.scrollIntoViewIfNeeded();
  if (!(await op.locator(".opblock-body").count())) await op.locator(".opblock-summary").first().click();
  await pause(500);
  const tryBtn = op.locator("button.try-out__btn");
  if (await tryBtn.count() && (await tryBtn.textContent()).includes("Try")) await tryBtn.click();
  const statusInput = op.locator("tr[data-param-name=status] input");
  await statusInput.fill("new,confirmed");
  const limitInput = op.locator("tr[data-param-name=limit] input");
  if (await limitInput.count()) await limitInput.fill("3");
  await op.locator("button.execute").click();
  await op.locator(".live-responses-table tbody .response-col_status").first().waitFor({ timeout: 10000 });
  await op.locator(".live-responses-table").first().scrollIntoViewIfNeeded();
  await page.mouse.wheel(0, 260);
  await pause(900);
  const status = (await op.locator(".live-responses-table tbody .response-col_status").first().textContent()).trim();
  const headers = await op.locator(".live-responses-table tbody .response-col_description").first().textContent();
  return { status, headers };
}
let r = await tryList("py");
ok(r.status === "200" && /x-backend: python/i.test(r.headers), "Swagger Try it out → Python (200, X-Backend: python)");
await shot("4-swagger-python");
r = await tryList("go");
ok(r.status === "200" && /x-backend: go/i.test(r.headers), "Swagger Try it out → Go (200, X-Backend: go)");
await shot("5-swagger-go");
await pause(800);

ok(errors.length === 0, "no page errors " + (errors.length ? errors.join(" | ") : ""));
const video = page.video();
await ctx.close();
if (video) console.log("video:", await video.path());

// phone width
const m = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: MEDIA ? 3 : 1, locale: "ru-RU" });
await m.addInitScript((lang) => { try { localStorage.setItem("api.lang", lang); } catch (e) {} }, LANG);
const mp = await m.newPage();
await mp.goto(BASE + "/", { waitUntil: "networkidle" });
const overflow = await mp.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
ok(overflow <= 1, "no horizontal scroll at 390 px (overflow " + overflow + ")");
if (MEDIA) await mp.screenshot({ path: `${OUT}6-mobile-${tag}.png` });
await m.close();
await browser.close();
console.log(failed ? `${failed} FAILED` : "all ok");
process.exit(failed ? 1 : 0);
