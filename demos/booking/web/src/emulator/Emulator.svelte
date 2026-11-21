<script>
  // Browser emulator of the Telegram client: a bot chat + Mini App overlay. The server signs initData with
  // telegram-core (sandbox token) and turns chat input into real webhook updates — the backend code is the same.
  import { onMount, onDestroy, tick } from 'svelte'

  const store = (k, v) => { try { if (v === undefined) return localStorage.getItem(k); localStorage.setItem(k, v) } catch {} }
  let lang = $state(store('emu_lang') || (navigator.language?.startsWith('ru') ? 'ru' : 'en'))
  let persona = $state(store('emu_persona') || 'guest')
  let platform = $state(store('emu_platform') || 'telegram') // telegram | max
  let name = $state(store('emu_name') || '')
  let guestId = Number(store('emu_guest')) || Math.floor(Math.random() * 1e11)
  store('emu_guest', String(guestId))
  let initData = $state('')
  let userId = $state(0)
  let token = ''
  let messages = $state([])
  let input = $state('')
  let appUrl = $state('')
  let chatEl
  let poll
  let notice = $state('')

  const T = {
    en: {
      h1: 'Salon booking bot + Telegram Mini App', lead: 'Live demo. This page emulates the Telegram app, so you can try it without an account. The backend is the production one: signed launch data, webhooks, payments sandbox.',
      persona: 'You are', guest: 'Client', owner: 'Salon owner', name: 'Your name', steps: 'Try it',
      s1: 'Tap «Book now» in the chat', s2: 'Pick a service, a master and a time', s3: 'Pay the deposit in Telegram Stars (sandbox)', s4: 'Get the confirmation and reminders in the chat',
      ownerSteps: 'As the owner you get a message for every new booking and manage the schedule, masters and services in the admin panel.',
      remind: '⏩ Send me a reminder now', admin: 'Open the admin panel ↗', reset: 'Reset demo data', resetDone: 'Demo data restored',
      bot: 'Bloom Salon Bot', online: 'bot', placeholder: 'Message', open: 'Open', start: 'Start',
      built: 'Built with', more: 'Same code runs as a real Telegram bot: just add a BotFather token.', noRem: 'Book something first: the reminder is about your next visit.',
      variants: 'Payments: Telegram Stars (international) · card via YooKassa (Russia, switch to RU). Max: card only.', messenger: 'Messenger',
    },
    ru: {
      h1: 'Бот записи в салон + Telegram Mini App', lead: 'Живое демо. Страница эмулирует Telegram, поэтому попробовать можно без аккаунта. Бэкенд боевой: подписанные данные запуска, вебхуки, песочница платежей.',
      persona: 'Вы', guest: 'Клиент', owner: 'Владелец салона', name: 'Ваше имя', steps: 'Как попробовать',
      s1: 'Нажмите «Записаться» в чате', s2: 'Выберите услугу, мастера и время', s3: 'Внесите предоплату картой (ЮKassa, песочница) или в Telegram Stars', s4: 'Получите подтверждение и напоминания в чат',
      ownerSteps: 'Владельцу приходит сообщение о каждой новой записи, а в админке он ведёт расписание, мастеров и услуги.',
      remind: '⏩ Прислать напоминание сейчас', admin: 'Открыть админку ↗', reset: 'Сбросить демо-данные', resetDone: 'Демо-данные восстановлены',
      bot: 'Bloom Salon Bot', online: 'бот', placeholder: 'Сообщение', open: 'Открыть', start: 'Старт',
      built: 'Стек', more: 'Тот же код работает как настоящий Telegram-бот, нужен только токен от BotFather.', noRem: 'Сначала запишитесь: напоминание приходит о ближайшем визите.',
      variants: 'Оплата: ЮKassa (РФ) и Telegram Stars (международный рынок, переключите на EN). В Max оплата картой.', messenger: 'Мессенджер',
    },
  }
  const L = $derived(T[lang])

  async function post(path, body, auth) {
    const r = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json', ...(auth ? { Authorization: 'Bearer ' + token } : {}) }, body: JSON.stringify(body || {}) })
    return r.ok ? r.json() : Promise.reject(await r.json().catch(() => ({})))
  }

  async function signIn() {
    store('emu_lang', lang); store('emu_persona', persona); store('emu_name', name); store('emu_platform', platform)
    const r = await post('/demo/initdata', { persona, guest_id: guestId, name, lang, platform })
    initData = r.init_data
    const changed = userId !== r.user_id
    userId = r.user_id
    const a = await post('/api/auth', { init_data: initData, platform: r.platform })
    token = a.token
    await post('/api/lang', { lang }, true)
    if (changed) { messages = []; await loadChat(true) }
  }

  async function loadChat(first = false) {
    if (!userId) return
    const after = messages.length ? messages[messages.length - 1].id : 0
    const r = await fetch(`/demo/chat?user=${userId}&after=${after}`).then((x) => x.json()).catch(() => ({ messages: [] }))
    if (r.messages?.length) {
      messages = [...messages, ...r.messages]
      await tick(); chatEl?.scrollTo({ top: chatEl.scrollHeight, behavior: first ? 'auto' : 'smooth' })
    } else if (first && messages.length === 0) {
      await send('/start')
    }
  }

  async function send(text) {
    text = (text ?? input).trim()
    if (!text) return
    input = ''
    await post('/demo/send', { user: userId, first_name: name || (persona === 'owner' ? L.owner : L.guest), lang, text })
    await loadChat()
  }

  function openApp(url) {
    if (!initData) return
    const u = new URL(url, location.origin)
    if (u.origin !== location.origin) return
    if (platform === 'max' && persona === 'guest') u.searchParams.set('platform', 'max')
    appUrl = u.pathname + u.search + '#tgWebAppData=' + encodeURIComponent(initData) + '&tgWebAppVersion=8.0&tgWebAppPlatform=unknown'
  }
  function openAdmin() { window.open('/admin/#tgWebAppData=' + encodeURIComponent(initData), '_blank', 'noopener') }

  async function remindNow() {
    const r = await post('/demo/reminders/run', {}, true).catch(() => ({ sent: 0 }))
    if (!r.sent) flash(L.noRem)
    loadChat()
  }
  async function resetDemo() { await post('/api/admin/reset', {}, true); flash(L.resetDone) }
  function flash(m) { notice = m; setTimeout(() => (notice = ''), 3000) }

  // safe subset of Telegram HTML: escape everything, then allow <b>
  const render = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/&lt;(\/?)b&gt;/g, '<$1b>').replace(/\n/g, '<br>')
  const btnOf = (m) => { try { return m.button ? JSON.parse(m.button) : null } catch { return null } }
  const time = (ts) => new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  function onMsg(ev) { if (ev.origin === location.origin && ev.data?.type === 'emu_close') appUrl = '' }
  onMount(async () => {
    window.addEventListener('message', onMsg)
    await signIn()
    poll = setInterval(loadChat, 2000)
  })
  onDestroy(() => { clearInterval(poll); window.removeEventListener('message', onMsg) })

  async function switchPersona(p) { persona = p; appUrl = ''; messages = []; initData = ''; await signIn() }
  async function switchPlatform(p) { platform = p; if (p === 'max' && lang === 'en') lang = 'ru'; appUrl = ''; messages = []; initData = ''; await signIn() }
  const onMax = $derived(platform === 'max' && persona === 'guest')
  async function switchLang(l) { lang = l; appUrl = ''; await signIn(); await send('/start') }
</script>

<div class="page">
  <section class="intro">
    <div class="top">
      <span class="badge">sedlab · demo</span>
      <div class="lang"><button class:on={lang === 'en'} onclick={() => switchLang('en')}>EN</button><button class:on={lang === 'ru'} onclick={() => switchLang('ru')}>RU</button></div>
    </div>
    <h1>{L.h1}</h1>
    <p class="lead">{L.lead}</p>

    <div class="block">
      <div class="label">{L.persona}</div>
      <div class="seg">
        <button class:on={persona === 'guest'} onclick={() => switchPersona('guest')}>👤 {L.guest}</button>
        <button class:on={persona === 'owner'} onclick={() => switchPersona('owner')}>💼 {L.owner}</button>
      </div>
      {#if persona === 'guest'}
        <div class="label sp">{L.messenger}</div>
        <div class="seg">
          <button class:on={platform === 'telegram'} onclick={() => switchPlatform('telegram')}>Telegram</button>
          <button class:on={platform === 'max'} onclick={() => switchPlatform('max')}>Max</button>
        </div>
        <input class="name" placeholder={L.name} bind:value={name} maxlength="32" onchange={signIn} />
      {/if}
    </div>

    {#if persona === 'guest'}
      <div class="block">
        <div class="label">{L.steps}</div>
        <ol><li>{L.s1}</li><li>{L.s2}</li><li>{L.s3}</li><li>{L.s4}</li></ol>
        <button class="btn soft" onclick={remindNow}>{L.remind}</button>
      </div>
    {:else}
      <div class="block">
        <p>{L.ownerSteps}</p>
        <div class="row"><button class="btn" onclick={openAdmin}>{L.admin}</button><button class="btn ghost" onclick={resetDemo}>{L.reset}</button></div>
      </div>
    {/if}
    <p class="hint">{L.variants}</p>
    <div class="block stack">
      <div class="label">{L.built}</div>
      <div class="chips">{#each ['Go', 'SQLite', 'Svelte', 'Telegram Mini App', 'Telegram Stars', 'YooKassa', 'Docker'] as c}<span>{c}</span>{/each}</div>
      <p class="hint">{L.more}</p>
    </div>
    {#if notice}<div class="notice">{notice}</div>{/if}
  </section>

  <section class="phone-wrap">
    <div class="phone" class:max={onMax}>
      <div class="chat-head">
        <div class="bot-ava">✿</div>
        <div><b>{L.bot}</b><div class="hint">{L.online}{onMax ? ' · Max' : ''}</div></div>
      </div>
      <div class="chat" bind:this={chatEl}>
        {#each messages as m (m.id)}
          {@const b = btnOf(m)}
          <div class="msg" class:me={m.kind === 'user'}>
            <div class="bubble">{@html render(m.text)}<span class="ts">{time(m.created_at)}</span></div>
            {#if b}<button class="inline-btn" onclick={() => openApp(b.url)}>{b.text}</button>{/if}
          </div>
        {/each}
      </div>
      <div class="composer">
        <button class="chip" onclick={() => send('/start')}>/start</button>
        <button class="chip" onclick={() => send('/my')}>/my</button>
        <input placeholder={L.placeholder} bind:value={input} onkeydown={(e) => e.key === 'Enter' && send()} maxlength="200" />
        <button class="send" onclick={() => send()} aria-label="send">➤</button>
      </div>
      {#if appUrl}
        <div class="miniapp">
          <div class="ma-head"><button onclick={() => (appUrl = '')}>✕</button><b>Bloom Salon</b><span class="hint">{onMax ? 'Max · ' : ''}mini app</span></div>
          <iframe src={appUrl} title="Mini App"></iframe>
        </div>
      {/if}
    </div>
  </section>
</div>

<style>
  :global(body) { background: radial-gradient(1200px 600px at 10% -10%, #efe9ff, transparent), radial-gradient(900px 500px at 110% 110%, #ffe3ee, transparent), #f7f6fb; }
  .page { max-width: 1120px; margin: 0 auto; padding: 32px 16px; display: grid; grid-template-columns: minmax(0, 1fr) 400px; gap: 40px; align-items: start; }
  .intro { padding-top: 12px; }
  .top { display: flex; justify-content: space-between; align-items: center; }
  .badge { font-size: 12px; font-weight: 600; letter-spacing: .5px; color: #6b4eff; background: #6b4eff14; padding: 4px 10px; border-radius: 99px; }
  h1 { font-size: 38px; line-height: 1.1; margin: 18px 0 12px; letter-spacing: -.5px; }
  .lead { font-size: 17px; color: #4b4b57; max-width: 560px; }
  .block { margin-top: 22px; max-width: 520px; }
  .label { font-size: 12px; text-transform: uppercase; letter-spacing: .6px; color: var(--hint); font-weight: 600; margin-bottom: 8px; }
  .seg, .lang { display: inline-flex; background: #fff; border-radius: 12px; padding: 3px; box-shadow: 0 1px 3px #0001; }
  .seg button, .lang button { border: 0; background: none; padding: 8px 14px; border-radius: 9px; font-weight: 600; color: var(--hint); }
  .lang button { padding: 5px 10px; font-size: 13px; }
  .seg button.on, .lang button.on { background: #6b4eff; color: #fff; }
  .label.sp { margin-top: 12px; }
  .phone.max .chat { background: #eef1f7; }
  .phone.max .bot-ava { background: linear-gradient(135deg, #3a7bff, #8a4dff); }
  .phone.max .inline-btn, .phone.max .chip, .phone.max .send { color: #3a6cf4; }
  .phone.max .msg.me .bubble { background: #dbe6ff; }
  .name { display: block; margin-top: 10px; border: 1.5px solid #e4e2ee; border-radius: 10px; padding: 10px 12px; width: 260px; background: #fff; }
  ol { margin: 0 0 14px; padding-left: 20px; line-height: 1.8; }
  .row { display: flex; gap: 10px; flex-wrap: wrap; }
  .row .btn { width: auto; }
  .block .btn.soft { width: auto; display: inline-block; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chips span { background: #fff; border: 1px solid #e8e6f2; border-radius: 8px; padding: 4px 10px; font-size: 13px; }
  .notice { position: fixed; left: 50%; bottom: 24px; transform: translateX(-50%); background: #222; color: #fff; padding: 10px 16px; border-radius: 10px; z-index: 50; }

  .phone { position: relative; height: 760px; border-radius: 44px; background: #fff; border: 10px solid #15151a; box-shadow: 0 30px 80px #2a1f5a33; overflow: hidden; display: flex; flex-direction: column; }
  .chat-head { display: flex; gap: 10px; align-items: center; padding: 14px 16px 10px; background: #fff; border-bottom: 1px solid #eee; }
  .chat-head .hint { font-size: 12px; }
  .bot-ava { width: 38px; height: 38px; border-radius: 50%; background: linear-gradient(135deg, #7c5cff, #e86a92); color: #fff; display: grid; place-items: center; font-size: 20px; }
  .chat { flex: 1; overflow-y: auto; padding: 12px; background: #dfe7d4 url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='40' height='40'%3E%3Ccircle cx='20' cy='20' r='1.5' fill='%23c6d3b8'/%3E%3C/svg%3E"); display: flex; flex-direction: column; gap: 8px; }
  .msg { display: flex; flex-direction: column; align-items: flex-start; max-width: 85%; }
  .msg.me { align-self: flex-end; align-items: flex-end; }
  .bubble { background: #fff; padding: 8px 10px 6px; border-radius: 14px 14px 14px 4px; font-size: 14.5px; line-height: 1.35; box-shadow: 0 1px 1px #0001; }
  .msg.me .bubble { background: #e3fdc9; border-radius: 14px 14px 4px 14px; }
  .ts { float: right; font-size: 11px; color: #9aa39a; margin: 6px 0 0 8px; }
  .inline-btn { margin-top: 4px; width: 100%; border: 0; background: #ffffffd9; border-radius: 10px; padding: 9px; font-weight: 600; color: #3d7dca; }
  .composer { display: flex; gap: 6px; align-items: center; padding: 8px; border-top: 1px solid #eee; background: #fff; }
  .composer input { flex: 1; min-width: 0; border: 0; background: #f2f2f5; border-radius: 18px; padding: 9px 12px; outline: none; }
  .chip { border: 1px solid #e3e3ea; background: #fff; border-radius: 14px; padding: 5px 8px; font-size: 12px; color: #3d7dca; }
  .send { border: 0; background: none; font-size: 20px; color: #3d7dca; }
  .miniapp { position: absolute; inset: 0; background: var(--bg2); display: flex; flex-direction: column; animation: up .28s ease-out; z-index: 10; }
  .ma-head { display: flex; align-items: center; gap: 10px; padding: 14px 14px 10px; background: #fff; border-bottom: 1px solid #eee; }
  .ma-head button { border: 0; background: none; font-size: 18px; color: #3d7dca; }
  .ma-head .hint { margin-left: auto; font-size: 12px; }
  .miniapp iframe { flex: 1; border: 0; width: 100%; }
  @keyframes up { from { transform: translateY(100%) } }

  @media (max-width: 900px) {
    .page { grid-template-columns: 1fr; gap: 24px; padding: 20px 16px; }
    h1 { font-size: 28px; }
    .phone { height: 680px; border-width: 6px; border-radius: 30px; max-width: 420px; margin: 0 auto; }
  }
</style>
