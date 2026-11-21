"use strict";
(() => {
  const I18N = {
    en: {
      brand: "Voice AI Assistant",
      h1: "Voice messages and calls → summary, tasks and a voice reply",
      lead: "Upload a recording or speak into the mic. The assistant transcribes it, writes a short summary, pulls out action items (who / what / when) and answers with a voice.",
      s_audio: "Audio / video", s_transcript: "Transcript", s_summary: "Summary", s_tasks: "Tasks", s_reply: "Voice reply",
      t_samples: "Samples", t_upload: "Upload file", t_record: "Record", llm: "LLM",
      drop: "Drop an audio or video file here, or click to choose",
      drop_hint: "mp3, m4a, ogg/opus (Telegram voice), wav, webm, mp4/mov, up to {mb} MB",
      rec_start: "Start recording", rec_stop: "Stop and process",
      rec_hint: "Say a few sentences, e.g. “Anna will send the report by Friday, I’ll call the client tomorrow”.",
      edit: "Edit text", done_edit: "Done", reanalyze: "Re-analyze", copy_md: "Copy as Markdown", copied: "Copied ✓",
      th_what: "What", th_who: "Who", th_due: "Due", play_reply: "Play the reply", stop: "Stop",
      about_h: "What’s inside",
      f1: "Speech-to-text for voice notes, calls and video files (Whisper / gpt-4o-transcribe), timestamps per segment.",
      f2: "Summary and action items with owner and due date as JSON, ready to push to a CRM, Notion, Jira or a Google Sheet.",
      f3: "One LLM interface: OpenAI, Anthropic, GigaChat, YandexGPT (or a local model), switched by config, no code changes.",
      f4: "Voice reply (TTS). In a Telegram bot it comes back as a voice message.",
      f5: "Cost control: rate limit per user, daily budget, maximum audio length.",
      foot: "Demo by sedlab · samples are synthetic voices, no personal data.",
      privacy: "Recordings are not stored: they are processed in memory and discarded right away. Please don’t upload recordings with personal data: the samples show everything.",
      mode_demo: "Demo mode", mode_live: "Live", mode_demo_t: "No API keys on this server: samples are pre-computed, everything else runs on mocks.",
      mode_live_t: "Real models with spend limits.",
      tag_live: "live", tag_demo: "demo",
      limits_live: "Live mode: up to {sec} s of audio, {mb} MB, {n} runs per hour per visitor. Runs left today: {left}.",
      limits_demo: "Demo mode: no keys, no costs. In live mode: audio up to {sec} s and {n} runs per hour per visitor.",
      k_meeting: "meeting", k_voice: "voice note", sec: "s",
      n_sample: "Pre-computed result for this sample (demo mode). With API keys the same button runs the real pipeline.",
      n_stt_mock: "Demo mode: live speech recognition is off, so the transcript is a placeholder from the mock. Press “Edit text”, type a few lines and “Re-analyze”, or try a sample.",
      n_stt_browser: "Demo mode: the transcript comes from your browser’s speech recognition.",
      n_llm_mock: "The LLM is a mock without a key: summary = first sentence, tasks = lines like “- Send the report @Anna 2026-12-01”.",
      e_generic: "Something went wrong. Please try again.", e_mic: "Microphone is not available: {m}",
      e_decode: "This browser can’t decode the file. Try mp3, m4a or wav.",
      no_tasks: "No action items found", processing: "Processing…",
      tg_line: "Telegram bot: send it a voice message and it replies with the summary, tasks and a voice answer.",
      p_sample: "sample", p_mock: "mock", p_browser: "browser", p_live: "live",
      seg_count: "{n} segments", dur: "{s} s",
    },
    ru: {
      brand: "Голосовой AI-ассистент",
      h1: "Голосовые и созвоны → саммари, задачи и ответ голосом",
      lead: "Загрузите запись или скажите в микрофон. Ассистент расшифрует, напишет короткое саммари, выпишет задачи (кто / что / срок) и ответит голосом.",
      s_audio: "Аудио / видео", s_transcript: "Расшифровка", s_summary: "Саммари", s_tasks: "Задачи", s_reply: "Ответ голосом",
      t_samples: "Примеры", t_upload: "Загрузить файл", t_record: "Записать", llm: "LLM",
      drop: "Перетащите аудио или видео сюда или нажмите, чтобы выбрать",
      drop_hint: "mp3, m4a, ogg/opus (голосовые Telegram), wav, webm, mp4/mov, до {mb} МБ",
      rec_start: "Начать запись", rec_stop: "Остановить и обработать",
      rec_hint: "Скажите пару фраз, например: «Анна пришлёт отчёт до пятницы, я завтра позвоню клиенту».",
      edit: "Править текст", done_edit: "Готово", reanalyze: "Пересчитать", copy_md: "Скопировать в Markdown", copied: "Скопировано ✓",
      th_what: "Что", th_who: "Кто", th_due: "Срок", play_reply: "Прослушать ответ", stop: "Стоп",
      about_h: "Что внутри",
      f1: "Распознавание голосовых, созвонов и видеофайлов (Whisper / gpt-4o-transcribe), таймкоды по фразам.",
      f2: "Саммари и задачи с ответственным и сроком в JSON, сразу в CRM, Notion, Jira или Google Таблицу.",
      f3: "Единый интерфейс LLM: OpenAI, Anthropic, GigaChat, YandexGPT (или локальная модель), переключение конфигом, без правок кода.",
      f4: "Ответ голосом (TTS). В Telegram-боте приходит голосовым сообщением.",
      f5: "Контроль расходов: лимит на пользователя, дневной бюджет, максимальная длина аудио.",
      foot: "Демо sedlab · в примерах синтезированные голоса, без персональных данных.",
      privacy: "Записи не сохраняются: обрабатываются в памяти и сразу удаляются. Пожалуйста, не загружайте записи с персональными данными: всё видно на примерах.",
      mode_demo: "Демо-режим", mode_live: "Живой режим", mode_demo_t: "На сервере нет API-ключей: примеры посчитаны заранее, остальное работает на моках.",
      mode_live_t: "Настоящие модели с лимитами расходов.",
      tag_live: "live", tag_demo: "демо",
      limits_live: "Живой режим: аудио до {sec} с и {mb} МБ, {n} запусков в час на посетителя. Осталось на сегодня: {left}.",
      limits_demo: "Демо-режим: без ключей и расходов. В живом режиме: аудио до {sec} с, {n} запусков в час на посетителя.",
      k_meeting: "созвон", k_voice: "голосовое", sec: "с",
      n_sample: "Результат для примера посчитан заранее (демо-режим). С API-ключами та же кнопка запускает настоящий конвейер.",
      n_stt_mock: "Демо-режим: живое распознавание выключено, поэтому расшифровка будет заглушкой мока. Нажмите «Править текст», напишите пару строк и «Пересчитать», или откройте пример.",
      n_stt_browser: "Демо-режим: расшифровка сделана распознаванием речи вашего браузера.",
      n_llm_mock: "LLM без ключа работает как мок: саммари = первое предложение, задачи = строки вида «- Прислать отчёт @Анна 2026-12-01».",
      e_generic: "Что-то пошло не так. Попробуйте ещё раз.", e_mic: "Микрофон недоступен: {m}",
      e_decode: "Браузер не смог прочитать файл. Попробуйте mp3, m4a или wav.",
      no_tasks: "Задач не найдено", processing: "Обработка…",
      tg_line: "Telegram-бот: пришлите ему голосовое, и он ответит саммари, задачами и голосом.",
      p_sample: "пример", p_mock: "мок", p_browser: "браузер", p_live: "live",
      seg_count: "{n} фраз", dur: "{s} с",
    },
  };
  const LLM_NAMES = { openai: "OpenAI", anthropic: "Anthropic", gigachat: "GigaChat", yandexgpt: "YandexGPT" };
  const $ = (s) => document.querySelector(s);
  const $$ = (s) => [...document.querySelectorAll(s)];
  const store = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };
  const params = new URLSearchParams(location.search);
  const state = {
    lang: params.get("lang") || store.get("voice.lang") || ((navigator.language || "en").startsWith("ru") ? "ru" : "en"),
    llm: store.get("voice.llm") || "",
    cfg: null, result: null, mediaUrl: null, busy: false,
  };
  const t = (k, vars) => {
    let s = (I18N[state.lang] && I18N[state.lang][k]) || I18N.en[k] || k;
    if (vars) for (const [a, b] of Object.entries(vars)) s = s.replaceAll("{" + a + "}", b);
    return s;
  };
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const fmtTime = (s) => { s = Math.max(0, Math.floor(s)); return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0"); };

  // client-side event log (helps debugging on phones without devtools)
  window.addEventListener("error", (e) => console.warn("voice:error", e.message));

  /* ---------- i18n ---------- */
  function applyLang() {
    document.documentElement.lang = state.lang;
    $$("[data-i18n]").forEach((el) => { el.textContent = t(el.dataset.i18n); });
    $$("[data-lang]").forEach((b) => b.classList.toggle("on", b.dataset.lang === state.lang));
    document.title = t("brand");
    if (state.cfg) { renderConfig(); }
    if (state.result) { render(state.result, { instant: true }); }
    if (rec.active) $("#micLabel").textContent = t("rec_stop");
  }
  $$("[data-lang]").forEach((b) => b.addEventListener("click", () => {
    state.lang = b.dataset.lang; store.set("voice.lang", state.lang); applyLang();
  }));

  /* ---------- config ---------- */
  async function loadConfig() {
    const r = await fetch("/api/config");
    state.cfg = await r.json();
    const llm = state.cfg.providers.llm;
    if (!llm[state.llm] || (llm[state.llm] !== "live" && Object.values(llm).includes("live"))) state.llm = state.cfg.default_llm || Object.keys(llm)[0];
    renderConfig();
  }

  function renderConfig() {
    const c = state.cfg, p = c.providers;
    const live = p.stt === "live" || Object.values(p.llm).includes("live");
    const b = $("#modeBadge");
    b.className = "badge " + (live ? "live" : "demo");
    b.textContent = live ? "● " + t("mode_live") : "● " + t("mode_demo");
    b.title = live ? t("mode_live_t") : t("mode_demo_t");

    $("#llmSeg").innerHTML = Object.entries(p.llm).map(([k, v]) =>
      `<button type="button" role="radio" data-llm="${k}" class="${k === state.llm ? "on" : ""}" aria-checked="${k === state.llm}">
        ${LLM_NAMES[k]} <span class="tag ${v}">${t("tag_" + v)}</span></button>`).join("");
    $$("#llmSeg button").forEach((x) => x.addEventListener("click", () => {
      state.llm = x.dataset.llm; store.set("voice.llm", state.llm); renderConfig();
    }));

    $("#samples").innerHTML = c.samples.map((s) => {
      const ico = s.file.endsWith(".mp4") ? "🎬" : s.kind === "voice" ? "🎙" : "👥";
      const bars = Array.from({ length: 36 }, (_, i) => {
        const h = 20 + Math.abs(Math.sin((i + s.id.length) * 1.7) * 60) + (i % 5) * 4;
        return `<i style="height:${Math.min(100, h)}%"></i>`;
      }).join("");
      return `<button type="button" class="sample" data-sample="${s.id}">
        <span class="s-top"><span class="s-ico">${ico}</span>${esc(s.title[state.lang] || s.title.en)}</span>
        <span class="wave">${bars}</span>
        <span class="s-meta">${s.lang.toUpperCase()} · ${t("k_" + s.kind)} · ${Math.round(s.duration_sec)} ${t("sec")} · ${s.file.split(".").pop()}</span>
      </button>`;
    }).join("");
    $$(".sample").forEach((x) => x.addEventListener("click", () => runSample(x.dataset.sample)));
    if (state.current) $$(".sample").forEach((x) => x.classList.toggle("on", x.dataset.sample === state.current));

    const L = c.limits;
    $("#limits").textContent = live
      ? t("limits_live", { sec: L.max_audio_sec, mb: L.max_upload_mb, n: L.ip_per_hour, left: L.daily_left })
      : t("limits_demo", { sec: L.max_audio_sec, n: L.ip_per_hour });
    $("#dropHint").textContent = t("drop_hint", { mb: L.max_upload_mb });
    $("#tgLine").hidden = !c.telegram;
    $("#tgLine").textContent = t("tg_line");
  }

  /* ---------- tabs ---------- */
  $$("[data-tab]").forEach((b) => b.addEventListener("click", () => {
    $$("[data-tab]").forEach((x) => x.classList.toggle("on", x === b));
    $$("[data-pane]").forEach((p) => { p.hidden = p.dataset.pane !== b.dataset.tab; });
  }));

  /* ---------- steps ---------- */
  const STEPS = ["audio", "transcript", "summary", "tasks", "reply"];
  function setStep(name) {
    const i = STEPS.indexOf(name);
    $$("#steps li").forEach((li, j) => {
      li.classList.toggle("done", i < 0 ? false : j < i || name === "all");
      li.classList.toggle("active", j === i);
    });
    if (name === "all") $$("#steps li").forEach((li) => { li.classList.add("done"); li.classList.remove("active"); });
  }

  /* ---------- audio helpers ---------- */
  async function toWav16k(blob) {
    const AC = window.AudioContext || window.webkitAudioContext;
    const ctx = new AC();
    try {
      const buf = await ctx.decodeAudioData(await blob.arrayBuffer());
      const rate = 16000, len = Math.ceil(buf.duration * rate);
      const off = new OfflineAudioContext(1, Math.max(1, len), rate);
      const src = off.createBufferSource(); src.buffer = buf; src.connect(off.destination); src.start();
      const out = await off.startRendering();
      const pcm = out.getChannelData(0);
      const view = new DataView(new ArrayBuffer(44 + pcm.length * 2));
      const w = (o, s) => { for (let i = 0; i < s.length; i++) view.setUint8(o + i, s.charCodeAt(i)); };
      w(0, "RIFF"); view.setUint32(4, 36 + pcm.length * 2, true); w(8, "WAVE"); w(12, "fmt ");
      view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true);
      view.setUint32(24, rate, true); view.setUint32(28, rate * 2, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true);
      w(36, "data"); view.setUint32(40, pcm.length * 2, true);
      for (let i = 0; i < pcm.length; i++) { const v = Math.max(-1, Math.min(1, pcm[i])); view.setInt16(44 + i * 2, v < 0 ? v * 0x8000 : v * 0x7fff, true); }
      return { wav: new Blob([view], { type: "audio/wav" }), duration: buf.duration };
    } finally { ctx.close && ctx.close(); }
  }

  /* ---------- API ---------- */
  async function api(url, opts) {
    const r = await fetch(url, opts);
    let j = null;
    try { j = await r.json(); } catch { /* non-JSON */ }
    if (!r.ok) throw new Error((j && j.message) || t("e_generic"));
    return j;
  }

  function startBusy() {
    state.busy = true;
    $("#results").hidden = false;
    $("#notice").hidden = true;
    $("#segments").innerHTML = '<div class="skeleton"></div><div class="skeleton" style="width:80%"></div><div class="skeleton" style="width:60%"></div>';
    $("#summary").innerHTML = '<div class="skeleton"></div><div class="skeleton" style="width:70%"></div>';
    $("#tasks").innerHTML = '<tr><td colspan="3"><div class="skeleton"></div></td></tr>';
    $("#reply").innerHTML = '<div class="skeleton"></div>';
    $("#trMeta").innerHTML = $("#llmMeta").innerHTML = $("#ttsMeta").innerHTML = "";
    $("#trEdit").hidden = true; $("#reBtn").hidden = true; $("#segments").hidden = false;
    stopSpeaking();
  }
  function fail(e) {
    state.busy = false; setStep(null);
    $("#notice").hidden = false; $("#notice").textContent = e.message || t("e_generic");
    $("#segments").innerHTML = $("#summary").innerHTML = $("#tasks").innerHTML = $("#reply").innerHTML = "";
  }

  function setMedia(url, isVideo) {
    if (state.mediaUrl && state.mediaUrl.startsWith("blob:")) URL.revokeObjectURL(state.mediaUrl);
    state.mediaUrl = url;
    const a = $("#player"), v = $("#vplayer");
    v.hidden = !isVideo; a.hidden = !!isVideo;
    (isVideo ? v : a).src = url;
    (isVideo ? a : v).removeAttribute("src");
  }
  const mediaEl = () => ($("#vplayer").hidden ? $("#player") : $("#vplayer"));

  async function runSample(id) {
    if (state.busy) return;
    state.current = id;
    $$(".sample").forEach((x) => x.classList.toggle("on", x.dataset.sample === id));
    const s = state.cfg.samples.find((x) => x.id === id);
    setMedia(s.file, s.file.endsWith(".mp4"));
    startBusy(); setStep("audio");
    try {
      const fd = new FormData(); fd.append("sample", id); fd.append("llm", state.llm); fd.append("lang", s.lang);
      const [r] = await Promise.all([api("/api/process", { method: "POST", body: fd }), sleep(500)]);
      await render(r);
    } catch (e) { fail(e); }
  }

  async function runBlob(blob, name, extra) {
    if (state.busy) return;
    state.current = null;
    $$(".sample").forEach((x) => x.classList.remove("on"));
    const isVideo = /^video\//.test(blob.type) || /\.(mp4|mov|webm)$/i.test(name) && !/^audio\//.test(blob.type);
    setMedia(URL.createObjectURL(blob), isVideo);
    startBusy(); setStep("audio");
    try {
      let body = blob, fname = name;
      try { const c = await toWav16k(blob); body = c.wav; fname = "audio.wav"; }
      catch (e) { if (state.cfg.providers.stt === "live") throw new Error(t("e_decode")); }
      setStep("transcript");
      const fd = new FormData();
      fd.append("file", body, fname); fd.append("llm", state.llm); fd.append("lang", state.lang);
      if (extra && extra.browserTranscript) fd.append("browser_transcript", extra.browserTranscript);
      const r = await api("/api/process", { method: "POST", body: fd });
      await render(r);
      loadConfig().catch(() => {});
    } catch (e) { fail(e); }
  }

  /* ---------- render ---------- */
  function chip(txt) { return `<span class="chip">${esc(txt)}</span>`; }
  function provLabel(p) { return t("p_" + p) !== "p_" + p ? t("p_" + p) : p; }

  async function render(r, opts = {}) {
    state.result = r;
    const instant = !!opts.instant;
    const step = async (name) => { if (!instant) { setStep(name); await sleep(420); } };
    const tr = r.transcript || { text: "", segments: [] };

    await step("transcript");
    const segs = tr.segments || [];
    $("#segments").innerHTML = segs.length
      ? segs.map((s, i) => `<button type="button" class="seg-line reveal" data-i="${i}" data-start="${s.start}" style="animation-delay:${instant ? 0 : i * 60}ms"><time>${fmtTime(s.start)}</time><span>${esc(s.text)}</span></button>`).join("")
      : `<div class="plain reveal">${esc(tr.text)}</div>`;
    $$(".seg-line").forEach((b) => b.addEventListener("click", () => { const m = mediaEl(); m.currentTime = +b.dataset.start; m.play(); }));
    $("#trEdit").value = tr.text;
    const meta = [];
    if (r.duration_sec) meta.push(t("dur", { s: Math.round(r.duration_sec) }));
    if (tr.language) meta.push(tr.language.toUpperCase());
    if (segs.length) meta.push(t("seg_count", { n: segs.length }));
    meta.push("STT: " + provLabel(r.mode && r.mode.stt));
    $("#trMeta").innerHTML = meta.map(chip).join("");

    await step("summary");
    $("#summary").innerHTML = `<span class="reveal">${esc(r.summary)}</span>`;
    const llmChip = r.llm ? `${LLM_NAMES[state.llm] || ""} → ${r.llm.provider}${r.llm.model ? " · " + r.llm.model : ""}` : t("p_sample");
    $("#llmMeta").innerHTML = chip(llmChip);

    await step("tasks");
    renderTasks(r.tasks || []);

    await step("reply");
    $("#reply").innerHTML = `<span class="reveal">${esc(r.reply.text)}</span>`;
    $("#ttsMeta").innerHTML = chip("TTS: " + provLabel(r.reply.audio ? (r.mode.tts === "sample" ? "sample" : "live") : "browser"));
    $("#replyAudio").src = r.reply.audio || "";

    const ns = (r.notices || []).map((n) => t("n_" + n)).filter((x) => !x.startsWith("n_"));
    $("#notice").hidden = !ns.length;
    $("#notice").textContent = ns.join(" ");
    if (!instant) { await sleep(200); setStep("all"); }
    state.busy = false;
  }

  function renderTasks(tasks) {
    const colors = ["#4f46e5", "#0891b2", "#db2777", "#059669", "#d97706", "#7c3aed"];
    const color = (name) => colors[[...name].reduce((a, c) => a + c.charCodeAt(0), 0) % colors.length];
    $("#tasks").innerHTML = tasks.length ? tasks.map((x, i) => `<tr class="reveal" style="animation-delay:${i * 70}ms">
      <td>${esc(x.title)}</td>
      <td>${x.owner ? `<span class="who"><span class="av" style="background:${color(x.owner)}">${esc(x.owner[0])}</span>${esc(x.owner)}</span>` : '<span class="empty">·</span>'}</td>
      <td>${x.due ? esc(fmtDate(x.due)) : '<span class="empty">·</span>'}</td></tr>`).join("")
      : `<tr><td colspan="3" class="empty">${t("no_tasks")}</td></tr>`;
  }
  function fmtDate(d) {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(d);
    if (!m) return d;
    try { return new Date(+m[1], +m[2] - 1, +m[3]).toLocaleDateString(state.lang === "ru" ? "ru-RU" : "en-US", { day: "numeric", month: "short" }); }
    catch { return d; }
  }

  // highlight the segment being played
  ["#player", "#vplayer"].forEach((sel) => $(sel).addEventListener("timeupdate", (e) => {
    const now = e.target.currentTime;
    const segs = (state.result && state.result.transcript && state.result.transcript.segments) || [];
    $$(".seg-line").forEach((b, i) => b.classList.toggle("now", !!segs[i] && now >= segs[i].start && now < segs[i].end + 0.4));
  }));

  /* ---------- edit / re-analyze / copy ---------- */
  $("#editBtn").addEventListener("click", () => {
    const editing = $("#trEdit").hidden;
    $("#trEdit").hidden = !editing; $("#segments").hidden = editing; $("#reBtn").hidden = !editing;
    $("#editBtn").textContent = editing ? t("done_edit") : t("edit");
    if (editing) $("#trEdit").focus();
  });
  $("#reBtn").addEventListener("click", async () => {
    const text = $("#trEdit").value.trim();
    if (!text || state.busy) return;
    state.busy = true; $("#reBtn").disabled = true;
    try {
      const r = await api("/api/analyze", { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, llm: state.llm, lang: state.lang }) });
      const prev = state.result || {};
      const tts = prev.mode && prev.mode.tts === "live" ? "live" : "browser";
      let audio = null;
      if (tts === "live") { try { audio = (await api("/api/speak", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text: r.reply.text }) })).audio; } catch { /* browser voice */ } }
      await render({ ...prev, transcript: { ...(prev.transcript || {}), text, segments: [] },
        summary: r.summary, tasks: r.tasks, llm: r.llm, reply: { text: r.reply.text, audio },
        mode: { ...(prev.mode || {}), llm: r.mode.llm, tts: audio ? "live" : "browser" }, notices: r.notices });
      $("#trEdit").hidden = true; $("#segments").hidden = false; $("#reBtn").hidden = true; $("#editBtn").textContent = t("edit");
    } catch (e) { $("#notice").hidden = false; $("#notice").textContent = e.message; state.busy = false; }
    finally { $("#reBtn").disabled = false; }
  });
  $("#copyBtn").addEventListener("click", async () => {
    const r = state.result; if (!r) return;
    const md = `## ${t("s_summary")}\n${r.summary}\n\n## ${t("s_tasks")}\n` +
      (r.tasks || []).map((x) => `- [ ] ${x.title}${x.owner || x.due ? " (" + [x.owner, x.due].filter(Boolean).join(", ") + ")" : ""}`).join("\n");
    try { await navigator.clipboard.writeText(md); $("#copyBtn").textContent = t("copied"); setTimeout(() => { $("#copyBtn").textContent = t("copy_md"); }, 1500); }
    catch { /* clipboard blocked */ }
  });

  /* ---------- voice reply ---------- */
  function stopSpeaking() {
    const a = $("#replyAudio"); a.pause();
    if (window.speechSynthesis) speechSynthesis.cancel();
    $("#speakBtn").classList.remove("playing");
  }
  $("#replyAudio").addEventListener("ended", () => $("#speakBtn").classList.remove("playing"));
  $("#speakBtn").addEventListener("click", () => {
    const r = state.result; if (!r) return;
    if ($("#speakBtn").classList.contains("playing")) { stopSpeaking(); return; }
    $("#speakBtn").classList.add("playing");
    if (r.reply.audio) { const a = $("#replyAudio"); a.currentTime = 0; a.play().catch(() => stopSpeaking()); return; }
    if (!window.speechSynthesis) { stopSpeaking(); return; }
    const u = new SpeechSynthesisUtterance(r.reply.text);
    const lang = r.lang || (r.transcript && r.transcript.language) || state.lang;
    u.lang = lang === "ru" ? "ru-RU" : "en-US";
    const v = speechSynthesis.getVoices().find((x) => x.lang && x.lang.toLowerCase().startsWith(lang));
    if (v) u.voice = v;
    u.onend = u.onerror = () => $("#speakBtn").classList.remove("playing");
    speechSynthesis.speak(u);
  });

  /* ---------- upload ---------- */
  const drop = $("#drop");
  $("#fileIn").addEventListener("change", (e) => { const f = e.target.files[0]; if (f) pickFile(f); e.target.value = ""; });
  ["dragenter", "dragover"].forEach((ev) => drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.add("over"); }));
  ["dragleave", "drop"].forEach((ev) => drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.remove("over"); }));
  drop.addEventListener("drop", (e) => { const f = e.dataTransfer.files[0]; if (f) pickFile(f); });
  function pickFile(f) {
    const L = state.cfg.limits;
    if (f.size > L.max_upload_mb * 1024 * 1024) { fail(new Error(t("drop_hint", { mb: L.max_upload_mb }))); $("#results").hidden = false; return; }
    runBlob(f, f.name);
  }

  /* ---------- record ---------- */
  const rec = { active: false, chunks: [], mr: null, stream: null, t0: 0, timer: 0, raf: 0, sr: null, text: "" };
  $("#micBtn").addEventListener("click", () => (rec.active ? stopRec() : startRec()));
  async function startRec() {
    try {
      rec.stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
    } catch (e) { $("#results").hidden = false; fail(new Error(t("e_mic", { m: e.message || e.name }))); return; }
    rec.chunks = []; rec.text = "";
    const mime = ["audio/webm;codecs=opus", "audio/mp4", "audio/webm"].find((m) => window.MediaRecorder && MediaRecorder.isTypeSupported(m)) || "";
    rec.mr = new MediaRecorder(rec.stream, mime ? { mimeType: mime } : undefined);
    rec.mr.ondataavailable = (e) => e.data.size && rec.chunks.push(e.data);
    rec.mr.onstop = () => {
      const blob = new Blob(rec.chunks, { type: rec.mr.mimeType || "audio/webm" });
      rec.stream.getTracks().forEach((x) => x.stop());
      runBlob(blob, "recording." + (blob.type.includes("mp4") ? "m4a" : "webm"), { browserTranscript: rec.text });
    };
    rec.mr.start(250);
    rec.active = true; rec.t0 = Date.now();
    $("#micBtn").classList.add("on"); $("#micBtn").setAttribute("aria-pressed", "true"); $("#micLabel").textContent = t("rec_stop");
    const max = state.cfg.limits.max_audio_sec;
    rec.timer = setInterval(() => {
      const s = (Date.now() - rec.t0) / 1000;
      $("#recTimer").textContent = fmtTime(s) + " / " + fmtTime(max);
      if (s >= max) stopRec();
    }, 200);
    meter(rec.stream);
    // demo mode: live captions from the browser's own speech recognition, if it has one
    const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    $("#liveCap").hidden = true;
    if (SR && state.cfg.providers.stt !== "live") {
      try {
        const sr = new SR(); rec.sr = sr;
        sr.lang = state.lang === "ru" ? "ru-RU" : "en-US"; sr.continuous = true; sr.interimResults = true;
        sr.onresult = (e) => {
          let fin = "", tmp = "";
          for (const res of e.results) (res.isFinal ? (fin += res[0].transcript + " ") : (tmp += res[0].transcript));
          rec.text = (fin + tmp).trim();
          $("#liveCap").hidden = !rec.text; $("#liveCap").textContent = rec.text;
        };
        sr.onerror = () => {};
        sr.start();
      } catch { rec.sr = null; }
    }
  }
  function stopRec() {
    if (!rec.active) return;
    rec.active = false;
    clearInterval(rec.timer); cancelAnimationFrame(rec.raf);
    if (rec.sr) { try { rec.sr.stop(); } catch { /* already stopped */ } rec.sr = null; }
    $("#micBtn").classList.remove("on"); $("#micBtn").setAttribute("aria-pressed", "false"); $("#micLabel").textContent = t("rec_start");
    // give the recognizer a moment to deliver the last phrase
    setTimeout(() => rec.mr && rec.mr.state !== "inactive" && rec.mr.stop(), 350);
  }
  function meter(stream) {
    const AC = window.AudioContext || window.webkitAudioContext;
    const ctx = new AC(), an = ctx.createAnalyser(); an.fftSize = 256;
    ctx.createMediaStreamSource(stream).connect(an);
    const cv = $("#meter"), g = cv.getContext("2d"), data = new Uint8Array(an.frequencyBinCount);
    const accent = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#4f46e5";
    const draw = () => {
      if (!rec.active) { ctx.close(); g.clearRect(0, 0, cv.width, cv.height); return; }
      an.getByteFrequencyData(data);
      g.clearRect(0, 0, cv.width, cv.height); g.fillStyle = accent;
      const n = 40, w = cv.width / n;
      for (let i = 0; i < n; i++) { const v = data[i * 2] / 255, h = Math.max(2, v * cv.height); g.fillRect(i * w + 1, (cv.height - h) / 2, w - 2, h); }
      rec.raf = requestAnimationFrame(draw);
    };
    draw();
  }

  /* ---------- boot ---------- */
  applyLang();
  loadConfig().then(() => {
    const s = params.get("sample");
    if (s) runSample(s);
  }).catch(() => fail(new Error(t("e_generic"))));
})();
