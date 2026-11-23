<script>
  import { onMount, onDestroy } from 'svelte'
  import { api, setToken, getToken, ApiError } from '../lib/api.js'
  import { t, pick } from '../lib/i18n.js'
  import { money, fmtWhen, fmtTime, dateLabel, durationLabel } from '../lib/fmt.js'
  import * as tg from '../lib/tg.js'

  let lang = $state('en')
  let ready = $state(false)
  let fatal = $state('')
  let toast = $state('')
  let cfg = $state({ tz: 'Europe/Moscow', pay_methods: [], deposit_percent: 30, hold_minutes: 15 })
  let user = $state(null)
  let services = $state([])
  let masters = $state([])
  let screen = $state('services') // services | master | time | confirm | booking | my
  let sel = $state({ service: null, master: 0, date: '', slot: null })
  let days = $state([])
  let slots = $state([])
  let loadingSlots = $state(false)
  let form = $state({ name: '', phone: '' })
  let method = $state('')
  let booking = $state(null)
  let myList = $state([])
  let busy = $state(false)
  let sheetBusy = $state(false)
  let nowTs = $state(Math.floor(Date.now() / 1000))
  let starsSheet = $state(null) // {title, amount, resolve}
  let checkoutUrl = $state('')
  let liveLink = $state('')
  let checkoutResolve = null
  let timer, poller

  const L = (k, v) => t(lang, k, v)
  const svc = $derived(services.find((s) => s.id === sel.service))
  // methods come from the edition (ru: card/YooKassa; intl: Stars + card/Stripe); Max has no Stars
  const methods = $derived(cfg.pay_methods.filter((m) => (tg.platform === 'max' ? m === 'card' : true)))
  const cardCur = $derived(cfg.card_currency || 'RUB')
  const cardPrice = (s) => (cardCur === 'USD' ? s.price_usd : s.price_rub)
  const mastersFor = $derived(svc ? masters.filter((m) => m.service_ids.includes(svc.id)) : [])
  const price = (s) => money(cardCur, cardPrice(s), lang)
  const depositOf = (s, m) => {
    const p = cfg.deposit_percent / 100
    return m === 'stars' ? Math.max(1, Math.ceil(s.price_stars * p)) : Math.max(100, Math.ceil((cardPrice(s) * p) / 100) * 100)
  }
  const amountOf = (s, m) => (m === 'stars' ? money('XTR', depositOf(s, m), lang) : money(cardCur, depositOf(s, m), lang))
  const fullOf = (s, m) => (m === 'stars' ? money('XTR', s.price_stars, lang) : money(cardCur, cardPrice(s), lang))
  const holdLeft = $derived(booking && booking.status === 'pending' ? Math.max(0, booking.hold_until - nowTs) : 0)

  function flash(msg) { toast = msg; setTimeout(() => (toast = ''), 2800) }
  function errMsg(e) {
    if (e instanceof ApiError && e.status === 409) return e.message || L('slotTaken')
    if (e instanceof ApiError && e.status === 401) return L('reopen')
    return L('err')
  }

  onMount(async () => {
    tg.ready()
    document.documentElement.dataset.theme = tg.colorScheme()
    tg.onEmulatorInvoice((url) => new Promise((resolve) => (starsSheet = { url, amount: booking?.pay_amount, title: pick(lang, booking?.service), resolve })))
    tg.onEmulatorCheckout((url) => new Promise((resolve) => { checkoutUrl = url + (url.includes('?') ? '&' : '?') + 'embed=1&lang=' + lang; checkoutResolve = resolve }))
    window.addEventListener('message', onMessage)
    timer = setInterval(() => (nowTs = Math.floor(Date.now() / 1000)), 1000)
    try {
      const auth = await api('POST', '/api/auth', { init_data: tg.initData(), platform: tg.platform })
      setToken(auth.token); user = auth.user
      form.name = auth.user.first_name || ''
      const [c, cat] = await Promise.all([api('GET', '/api/config'), api('GET', '/api/catalog')])
      cfg = c; services = cat.services; masters = cat.masters; lang = c.lang === 'ru' ? 'ru' : 'en'
      const q = new URLSearchParams(location.search)
      if (q.get('booking')) await openBooking(+q.get('booking'))
      else if (q.get('screen') === 'my') await openMy()
      ready = true
    } catch (e) {
      if (e instanceof ApiError && e.code === 'consent_required') fatal = t('ru', 'consentFirst')
      else fatal = tg.initData() ? L('reopen') : 'Open this app from the Telegram bot (or the demo emulator).'
    }
  })
  onDestroy(() => { clearInterval(timer); clearInterval(poller); window.removeEventListener('message', onMessage) })

  function onMessage(ev) {
    if (ev.origin !== location.origin) return
    if (ev.data?.type === 'checkout_done') { checkoutUrl = ''; checkoutResolve?.(ev.data.outcome); checkoutResolve = null }
  }


  function go(s) { screen = s; window.scrollTo(0, 0); tg.haptic() }

  function chooseService(s) { sel.service = s.id; sel.master = 0; sel.slot = null; go('master') }
  async function chooseMaster(id) {
    sel.master = id; sel.slot = null; go('time')
    const r = await api('GET', `/api/days?service=${sel.service}&master=${id}`)
    days = r.days
    const first = days.find((d) => d.free > 0) || days[0]
    pickDate(first.date)
  }
  async function pickDate(d) {
    sel.date = d; sel.slot = null; loadingSlots = true
    try { slots = (await api('GET', `/api/slots?service=${sel.service}&master=${sel.master}&date=${d}`)).slots } finally { loadingSlots = false }
  }
  function chooseSlot(s) { sel.slot = s; tg.haptic() }
  function toConfirm() { booking = null; method = methods[0] || ''; go('confirm') }

  async function pay() {
    if (!form.name.trim()) return flash(L('name'))
    busy = true
    try {
      if (!booking || booking.status !== 'pending' || holdLeft <= 0) {
        booking = await api('POST', '/api/bookings', { service_id: sel.service, master_id: sel.slot.master_id, start: sel.slot.start, name: form.name, phone: form.phone })
      }
      const p = await api('POST', `/api/bookings/${booking.id}/pay`, { method })
      booking = { ...booking, pay_amount: p.amount, pay_currency: p.currency }
      if (p.live) flash(L('liveOn', { n: p.live_left, prov: cfg.live_provider }))
      else if (['user_daily', 'ip_daily', 'global_daily', 'budget_month'].includes(p.live_reason)) flash(L('liveOff', { prov: cfg.live_provider }))
      if (p.method === 'stars') {
        const status = await tg.openInvoice(p.invoice_link)
        if (status === 'paid') { tg.haptic('success'); await openBooking(booking.id, true) }
        else if (status === 'failed') flash(L('err'))
      } else {
        // live test shop is a real external page: open it outside the emulator overlay
        let r
        if (p.live && tg.isEmulator) { // real YooKassa page: a separate tab (it cannot live inside the imitation)
          const w = window.open(p.confirmation_url, '_blank')
          if (w) w.opener = null
          else liveLink = p.confirmation_url // popup blocked: the visitor taps the link
          r = 'opened'
        } else r = await tg.openCheckout(p.confirmation_url)
        await openBooking(booking.id, true)
        if (r === 'canceled') flash(L('st_pending'))
      }
    } catch (e) {
      flash(errMsg(e))
      if (e instanceof ApiError && e.status === 409) { booking = null; go('time'); pickDate(sel.date) }
    } finally { busy = false }
  }

  async function payStarsEmu() {
    const sheet = starsSheet
    sheetBusy = true
    try {
      await api('POST', '/demo/stars/pay', { booking_id: booking.id })
      starsSheet = null; sheet.resolve('paid')
    } catch (e) { starsSheet = null; flash(errMsg(e)); sheet.resolve('failed') } finally { sheetBusy = false }
  }

  async function openBooking(id, poll = false) {
    booking = await api('GET', `/api/bookings/${id}`)
    go('booking')
    clearInterval(poller)
    if (booking.status === 'pending') {
      poller = setInterval(async () => {
        try { booking = await api('GET', `/api/bookings/${id}`) } catch {}
        if (booking.status !== 'pending') { clearInterval(poller); if (booking.status === 'confirmed') tg.haptic('success') }
      }, 2000)
    }
  }

  async function openMy() { myList = (await api('GET', '/api/bookings/my')).bookings.filter((b) => b.status !== 'expired'); go('my') }

  async function cancelBooking() {
    const msg = `${L('cancelConfirm')} ${booking.cancel_refundable ? L('refundYes') : booking.status === 'confirmed' ? L('refundNo') : ''}`
    if (!confirm(msg)) return
    busy = true
    try { booking = await api('POST', `/api/bookings/${booking.id}/cancel`); tg.haptic('warning') } catch (e) { flash(errMsg(e)) } finally { busy = false }
  }

  function restart() { sel = { service: null, master: 0, date: '', slot: null }; booking = null; go('services') }
  const back = () => go({ master: 'services', time: 'master', confirm: 'time', booking: 'services', my: 'services' }[screen] || 'services')
</script>

{#if fatal}
  <div class="center"><div class="logo">✿</div><p>{fatal}</p></div>
{:else if !ready}
  <div class="center"><div class="spinner"></div></div>
{:else}
  <header>
    {#if screen !== 'services'}<button class="icon" onclick={back} aria-label={L('back')}>‹</button>{:else}<span class="logo sm">✿</span>{/if}
    <div class="title">
      <b>{L('salon')}</b>
      <span class="hint">{L('tagline')}</span>
    </div>
  </header>

  <main>
    {#if screen === 'services'}
      <div class="row-between"><h2>{L('services')}</h2><button class="link" onclick={openMy}>{L('my')} →</button></div>
      <div class="list">
        {#each services as s (s.id)}
          <button class="card item" onclick={() => chooseService(s)}>
            <div class="grow"><div class="name">{pick(lang, { ru: s.name_ru, en: s.name_en })}</div><div class="hint">{durationLabel(s.duration_min, lang)}</div></div>
            <div class="price">{price(s)}</div>
          </button>
        {/each}
      </div>

    {:else if screen === 'master'}
      <h2>{L('master')}</h2>
      <p class="hint">{pick(lang, { ru: svc.name_ru, en: svc.name_en })} · {durationLabel(svc.duration_min, lang)} · {price(svc)}</p>
      <div class="list">
        <button class="card item" onclick={() => chooseMaster(0)}>
          <div class="avatar any">★</div>
          <div class="grow"><div class="name">{L('anyMaster')}</div><div class="hint">{L('anyMasterHint')}</div></div>
        </button>
        {#each mastersFor as m (m.id)}
          <button class="card item" onclick={() => chooseMaster(m.id)}>
            <div class="avatar" style="background:{m.color}">{pick(lang, { ru: m.name_ru, en: m.name_en })[0]}</div>
            <div class="grow"><div class="name">{pick(lang, { ru: m.name_ru, en: m.name_en })}</div><div class="hint">{pick(lang, { ru: m.role_ru, en: m.role_en })}</div></div>
          </button>
        {/each}
      </div>

    {:else if screen === 'time'}
      <h2>{L('time')}</h2>
      <div class="days">
        {#each days as d, i (d.date)}
          {@const l = dateLabel(d.date, lang)}
          <button class="day" class:on={d.date === sel.date} class:empty={d.free === 0} onclick={() => pickDate(d.date)}>
            <span class="wd">{i === 0 ? L('today') : i === 1 ? L('tomorrow') : l.wd}</span><b>{l.day}</b><span class="mon">{l.mon}</span>
          </button>
        {/each}
      </div>
      {#if loadingSlots}<div class="center small"><div class="spinner"></div></div>
      {:else if slots.length === 0}<p class="hint pad">{L('noSlots')}</p>
      {:else}
        <div class="slots">
          {#each slots as s (s.start + '-' + s.master_id)}
            <button class="slot" class:on={sel.slot?.start === s.start && sel.slot?.master_id === s.master_id} onclick={() => chooseSlot(s)}>{s.label}</button>
          {/each}
        </div>
      {/if}
      <div class="bar"><button class="btn" disabled={!sel.slot} onclick={toConfirm}>{L('next')}</button></div>

    {:else if screen === 'confirm'}
      {@const m = masters.find((x) => x.id === sel.slot.master_id)}
      <h2>{L('confirm')}</h2>
      <div class="card summary">
        <div class="avatar" style="background:{m.color}">{pick(lang, { ru: m.name_ru, en: m.name_en })[0]}</div>
        <div class="grow">
          <div class="name">{pick(lang, { ru: svc.name_ru, en: svc.name_en })}</div>
          <div class="hint">{L('with')} {pick(lang, { ru: m.name_ru, en: m.name_en })}</div>
          <div class="when">{fmtWhen(sel.slot.start, cfg.tz, lang)}</div>
        </div>
      </div>
      <label class="field"><span>{L('name')}</span><input bind:value={form.name} maxlength="60" autocomplete="name" /></label>
      {#if cfg.collect_phone}<label class="field"><span>{L('phone')}</span><input bind:value={form.phone} maxlength="30" inputmode="tel" autocomplete="tel" placeholder="+7 …" /></label>{/if}

      <h3>{L('prepay')}</h3>
      <p class="hint">{L('prepayHint', { p: cfg.deposit_percent })}</p>
      <div class="list">
        {#each methods as mth}
          <button class="card item method" class:on={method === mth} onclick={() => (method = mth)}>
            <span class="radio"></span>
            <div class="grow"><div class="name">{mth === 'stars' ? L('payStars') : L('payCard')}</div>
              <div class="hint">{L('total')}: {fullOf(svc, mth)}</div></div>
            <div class="price">{amountOf(svc, mth)}</div>
          </button>
        {/each}
      </div>
      {#if lang === 'ru'}<p class="hint">{cfg.live_available ? L('livePay', { prov: cfg.live_provider }) + (cfg.live_provider === 'Robokassa' ? L('livePayRobo') : L('livePayYoo')) : L('demoPay')}</p>{/if}
      {#if liveLink}<a class="btn soft" href={liveLink} target="_blank" rel="noopener" onclick={() => (liveLink = '')}>{L('liveOpen', { prov: cfg.live_provider })} ↗</a>{/if}
      {#if booking?.status === 'pending' && holdLeft > 0}
        <p class="hint hold">⏱ {L('held', { m: Math.floor(holdLeft / 60), s: String(holdLeft % 60).padStart(2, '0') })}</p>
      {/if}
      <div class="bar">
        <button class="btn" disabled={busy || !method} onclick={pay}>
          {busy ? '…' : L('pay', { amount: amountOf(svc, method) })}
        </button>
      </div>

    {:else if screen === 'booking' && booking}
      {#if booking.status === 'confirmed'}
        <div class="hero ok"><div class="check">✓</div><h2>{L('confirmedTitle')}</h2><p class="hint">{L('confirmedText')}</p></div>
      {:else if booking.status === 'pending'}
        <div class="hero"><div class="spinner"></div><p>{L('waitingPay')}</p></div>
      {/if}
      <div class="card details">
        <div class="row-between"><b>{pick(lang, booking.service)}</b><span class="pill st-{booking.status}">{L('st_' + booking.status)}</span></div>
        <div class="hint">{L('with')} {pick(lang, booking.master)}</div>
        <div class="when">{fmtWhen(booking.start_at, cfg.tz, lang)}–{fmtTime(booking.end_at, cfg.tz)}</div>
        {#if booking.paid_at}<div class="paid">{L('paid')}: {money(booking.pay_currency, booking.pay_amount, lang)} {booking.refunded ? `· ${L('refunded')}` : ''}</div>{/if}
      </div>
      <div class="stack">
        {#if booking.status === 'confirmed'}<button class="btn danger" disabled={busy} onclick={cancelBooking}>{L('cancel')}</button>{/if}
        <button class="btn soft" onclick={openMy}>{L('my')}</button>
        <button class="btn ghost" onclick={restart}>{L('bookAnother')}</button>
      </div>

    {:else if screen === 'my'}
      <h2>{L('my')}</h2>
      {#if myList.length === 0}<p class="hint pad">{L('none')}</p>{/if}
      <div class="list">
        {#each myList as b (b.id)}
          <button class="card item" onclick={() => openBooking(b.id)}>
            <div class="avatar" style="background:{b.master.color}">{pick(lang, b.master)[0]}</div>
            <div class="grow"><div class="name">{pick(lang, b.service)}</div><div class="hint">{fmtWhen(b.start_at, cfg.tz, lang)}</div></div>
            <span class="pill st-{b.status}">{L('st_' + b.status)}</span>
          </button>
        {/each}
      </div>
      <div class="bar"><button class="btn" onclick={restart}>{L('book')}</button></div>
    {/if}
  </main>

  {#if starsSheet}
    <div class="scrim" onclick={() => { starsSheet.resolve('cancelled'); starsSheet = null }} role="presentation"></div>
    <div class="sheet" role="dialog" aria-label={L('starsTitle')}>
      <div class="star-big">⭐</div>
      <h3>{L('starsTitle')}</h3>
      <p>{starsSheet.title}: <b>{money('XTR', booking.pay_amount, lang)}</b></p>
      <p class="hint">{L('starsBalance')}</p>
      <button class="btn stars" disabled={sheetBusy} onclick={payStarsEmu}>{L('starsPay')} {money('XTR', booking.pay_amount, lang)}</button>
      <p class="hint small-note">{L('starsNote')}</p>
    </div>
  {/if}

  {#if checkoutUrl}
    <div class="checkout">
      <div class="checkout-bar"><span>{new URL(checkoutUrl).host === location.host ? (lang === 'ru' ? 'Учебная оплата' : 'Practice payment') : '🔒 ' + new URL(checkoutUrl).host}</span><button class="icon" onclick={() => { checkoutUrl = ''; checkoutResolve?.('closed') }}>✕</button></div>
      <iframe src={checkoutUrl} title="checkout"></iframe>
    </div>
  {/if}

  {#if toast}<div class="toast">{toast}</div>{/if}
{/if}

<style>
  header { position: sticky; top: 0; z-index: 5; display: flex; align-items: center; gap: 10px; padding: 10px 14px; background: var(--bg); border-bottom: 1px solid var(--line); }
  .title { display: flex; flex-direction: column; flex: 1; line-height: 1.2; }
  .icon { border: 0; background: none; font-size: 26px; line-height: 1; padding: 0 6px; color: var(--link); }
  .logo { font-size: 40px; color: var(--btn); }
  .logo.sm { font-size: 24px; }
  .lang { display: flex; background: var(--bg2); border-radius: 8px; padding: 2px; }
  .lang button { border: 0; background: none; padding: 4px 8px; border-radius: 6px; font-size: 12px; font-weight: 600; color: var(--hint); }
  .lang button.on { background: var(--card); color: var(--text); box-shadow: 0 1px 2px #0002; }
  main { padding: 6px 14px 110px; max-width: 560px; margin: 0 auto; }
  h2 { font-size: 20px; margin: 16px 0 10px; }
  h3 { margin: 20px 0 4px; font-size: 16px; }
  .row-between { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
  .link { border: 0; background: none; color: var(--link); font-weight: 500; }
  .list { display: flex; flex-direction: column; gap: 8px; }
  .item { display: flex; align-items: center; gap: 12px; padding: 14px; border: 1.5px solid transparent; text-align: left; width: 100%; transition: transform .08s; }
  .item:active { transform: scale(.985); }
  .grow { flex: 1; min-width: 0; }
  .name { font-weight: 600; }
  .price { font-weight: 600; white-space: nowrap; }
  .avatar { width: 42px; height: 42px; border-radius: 50%; display: grid; place-items: center; color: #fff; font-weight: 700; font-size: 18px; flex: none; }
  .avatar.any { background: linear-gradient(135deg, #7c5cff, #e86a92); }
  .days { display: flex; gap: 8px; overflow-x: auto; padding: 4px 0 10px; scrollbar-width: none; }
  .day { flex: none; width: 58px; padding: 8px 0; border-radius: 12px; border: 1.5px solid var(--line); background: var(--card); display: flex; flex-direction: column; align-items: center; gap: 2px; }
  .day b { font-size: 18px; }
  .day .wd, .day .mon { font-size: 11px; color: var(--hint); }
  .day .wd { text-transform: capitalize; }
  .day.on { background: var(--btn); border-color: var(--btn); color: var(--btn-text); }
  .day.on .wd, .day.on .mon { color: inherit; opacity: .85; }
  .day.empty:not(.on) { opacity: .45; }
  .slots { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; margin-top: 6px; }
  .slot { padding: 11px 0; border-radius: 10px; border: 1.5px solid var(--line); background: var(--card); font-weight: 600; }
  .slot.on { background: var(--btn); color: var(--btn-text); border-color: var(--btn); }
  .bar { position: fixed; left: 0; right: 0; bottom: 0; padding: 10px 14px calc(10px + env(safe-area-inset-bottom)); background: var(--bg); border-top: 1px solid var(--line); z-index: 4; }
  .bar .btn { max-width: 532px; margin: 0 auto; }
  .summary { display: flex; gap: 12px; padding: 14px; align-items: center; }
  .when { margin-top: 4px; font-weight: 600; }
  .field { display: flex; flex-direction: column; gap: 4px; margin-top: 12px; font-size: 13px; color: var(--hint); }
  .field input { border: 1.5px solid var(--line); border-radius: 10px; padding: 12px; font-size: 16px; outline: none; }
  .field input:focus { border-color: var(--btn); }
  .method.on { border-color: var(--btn); }
  .radio { width: 20px; height: 20px; border-radius: 50%; border: 2px solid var(--line); flex: none; }
  .method.on .radio { border: 6px solid var(--btn); }
  .hold { margin-top: 10px; color: var(--warn); }
  .hero { text-align: center; padding: 24px 0 8px; }
  .hero h2 { margin: 10px 0 4px; }
  .check { width: 64px; height: 64px; margin: 0 auto; border-radius: 50%; background: var(--ok); color: #fff; font-size: 34px; display: grid; place-items: center; animation: pop .35s ease-out; }
  @keyframes pop { from { transform: scale(.4); opacity: 0 } to { transform: scale(1); opacity: 1 } }
  .details { padding: 14px; margin-top: 14px; display: flex; flex-direction: column; gap: 4px; }
  .paid { margin-top: 6px; color: var(--ok); font-weight: 600; font-size: 14px; }
  .stack { display: flex; flex-direction: column; gap: 8px; margin-top: 16px; }
  .center { min-height: 70vh; display: grid; place-items: center; text-align: center; padding: 24px; }
  .center.small { min-height: 120px; }
  .pad { padding: 16px 0; }
  .spinner { width: 30px; height: 30px; border-radius: 50%; border: 3px solid var(--line); border-top-color: var(--btn); animation: spin .8s linear infinite; margin: 0 auto; }
  @keyframes spin { to { transform: rotate(360deg) } }
  .scrim { position: fixed; inset: 0; background: #0007; z-index: 20; animation: fade .2s; }
  .sheet { position: fixed; left: 0; right: 0; bottom: 0; z-index: 21; background: var(--bg); border-radius: 18px 18px 0 0; padding: 20px 18px calc(18px + env(safe-area-inset-bottom)); text-align: center; animation: up .25s ease-out; }
  .sheet h3 { margin: 6px 0; }
  .star-big { font-size: 44px; }
  .btn.stars { background: linear-gradient(90deg, #f7b733, #fc9a2b); margin-top: 8px; }
  .small-note { font-size: 11px; margin-top: 10px; }
  @keyframes up { from { transform: translateY(100%) } }
  @keyframes fade { from { opacity: 0 } }
  .checkout { position: fixed; inset: 0; z-index: 30; background: var(--bg); display: flex; flex-direction: column; animation: up .25s ease-out; }
  .checkout-bar { display: flex; justify-content: space-between; align-items: center; padding: 8px 12px; font-size: 13px; color: var(--hint); border-bottom: 1px solid var(--line); }
  .checkout iframe { flex: 1; border: 0; width: 100%; }
  .toast { position: fixed; left: 50%; bottom: 90px; transform: translateX(-50%); background: #222c; color: #fff; padding: 10px 14px; border-radius: 10px; z-index: 40; max-width: 90%; font-size: 14px; }
</style>
