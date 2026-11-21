<script>
  import { onMount } from 'svelte'
  import { api, setToken, ApiError } from '../lib/api.js'
  import { pick } from '../lib/i18n.js'
  import { money, fmtTime, fmtDay, durationLabel } from '../lib/fmt.js'
  import * as tg from '../lib/tg.js'

  const D = {
    en: { title: 'Salon admin', schedule: 'Schedule', bookings: 'Bookings', services: 'Services', masters: 'Masters', today: 'Today',
      kWeek: 'Bookings, next 7 days', kRub: 'Card prepayments, 30 d', kStars: 'Stars prepayments, 30 d', kHold: 'Unpaid holds now',
      client: 'Client', service: 'Service', master: 'Master', when: 'When', status: 'Status', paid: 'Prepaid', actions: '',
      cancel: 'Cancel', cancelRefund: 'Cancel + refund', complete: 'Done', all: 'All', save: 'Save', add: 'Add', active: 'Active',
      nameRu: 'Name (RU)', nameEn: 'Name (EN)', duration: 'Duration, min', priceRub: 'Price, ₽', priceStars: 'Price, ⭐',
      roleRu: 'Role (RU)', roleEn: 'Role (EN)', color: 'Colour', hours: 'Working hours', does: 'Services', off: 'day off', saved: 'Saved',
      noAccess: 'Owner access only. Open the admin from the bot (/admin) or from the demo emulator as «Salon owner».', free: 'free',
      st: { pending: 'Awaiting payment', confirmed: 'Confirmed', completed: 'Completed', cancelled: 'Cancelled' }, refunded: 'refunded', newMaster: 'New master', newService: 'New service',
      wd: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] },
    ru: { title: 'Админка салона', schedule: 'Расписание', bookings: 'Записи', services: 'Услуги', masters: 'Мастера', today: 'Сегодня',
      kWeek: 'Записей на 7 дней', kRub: 'Предоплаты картой, 30 дн', kStars: 'Предоплаты Stars, 30 дн', kHold: 'Неоплаченных броней',
      client: 'Клиент', service: 'Услуга', master: 'Мастер', when: 'Когда', status: 'Статус', paid: 'Предоплата', actions: '',
      cancel: 'Отменить', cancelRefund: 'Отменить + возврат', complete: 'Пришёл', all: 'Все', save: 'Сохранить', add: 'Добавить', active: 'Активна',
      nameRu: 'Название (RU)', nameEn: 'Название (EN)', duration: 'Длительность, мин', priceRub: 'Цена, ₽', priceStars: 'Цена, ⭐',
      roleRu: 'Специализация (RU)', roleEn: 'Специализация (EN)', color: 'Цвет', hours: 'Часы работы', does: 'Услуги', off: 'выходной', saved: 'Сохранено',
      noAccess: 'Только для владельца. Откройте админку из бота (/admin) или из демо-эмулятора как «Владелец салона».', free: 'свободно',
      st: { pending: 'Ждёт оплаты', confirmed: 'Подтверждена', completed: 'Завершена', cancelled: 'Отменена' }, refunded: 'возвращена', newMaster: 'Новый мастер', newService: 'Новая услуга',
      wd: ['Вс', 'Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб'] },
  }
  let lang = $state('en')
  const L = $derived(D[lang])
  let tab = $state('schedule')
  let err = $state('')
  let ready = $state(false)
  let tz = $state('Europe/Moscow')
  let date = $state('')
  let ov = $state(null)
  let list = $state([])
  let statusFilter = $state('')
  let services = $state([])
  let masters = $state([])
  let toast = $state('')
  const START = 9, END = 21, PX = 72 // hour rows

  const flash = (m) => { toast = m; setTimeout(() => (toast = ''), 2200) }
  const isoToday = () => new Intl.DateTimeFormat('en-CA', { timeZone: tz }).format(new Date())
  const shift = (iso, n) => { const d = new Date(iso + 'T12:00:00Z'); d.setUTCDate(d.getUTCDate() + n); return d.toISOString().slice(0, 10) }

  onMount(async () => {
    tg.ready()
    document.documentElement.dataset.theme = tg.colorScheme()
    try {
      const a = await api('POST', '/api/auth', { init_data: tg.initData() })
      setToken(a.token); lang = a.user.lang || 'en'
      if (a.role !== 'owner') { err = 'noAccess'; return }
      tz = (await api('GET', '/api/config')).tz
      date = isoToday()
      await Promise.all([loadOverview(), loadCatalog()])
      ready = true
    } catch { err = 'noAccess' }
  })

  async function loadOverview() { ov = await api('GET', `/api/admin/overview?date=${date}`) }
  async function loadList() { list = (await api('GET', `/api/admin/bookings?from=${shift(isoToday(), -7)}&days=30${statusFilter ? '&status=' + statusFilter : ''}`)).bookings }
  async function loadCatalog() {
    services = (await api('GET', '/api/admin/services')).services
    masters = (await api('GET', '/api/admin/masters')).masters.map((m) => ({ ...m, hoursMap: Object.fromEntries(m.hours.map((h) => [h.weekday, h])) }))
  }
  async function setTab(t) { tab = t; if (t === 'bookings') await loadList(); if (t === 'schedule') await loadOverview() }
  async function setDate(d) { date = d; await loadOverview() }

  async function act(b, kind) {
    try {
      if (kind === 'complete') await api('POST', `/api/admin/bookings/${b.id}/complete`)
      else await api('POST', `/api/admin/bookings/${b.id}/cancel`, { refund: kind === 'refund' })
      flash(L.saved); await (tab === 'bookings' ? loadList() : loadOverview())
    } catch (e) { flash(e.message) }
  }

  async function saveService(s) {
    try { await api('POST', '/api/admin/services', { ...s, duration_min: +s.duration_min, price_rub: Math.round(+s.price_rub_view * 100), price_stars: +s.price_stars }); flash(L.saved); await loadCatalog() }
    catch (e) { flash(e.message) }
  }
  function addService() { services = [...services, { id: 0, name_ru: '', name_en: '', duration_min: 60, price_rub: 100000, price_stars: 600, active: true, sort: services.length + 1 }] }

  async function saveMaster(m) {
    const hours = Object.values(m.hoursMap).filter(Boolean).map((h) => ({ weekday: h.weekday, start_min: +h.start_min, end_min: +h.end_min }))
    try { await api('POST', '/api/admin/masters', { ...m, hours }); flash(L.saved); await loadCatalog() } catch (e) { flash(e.message) }
  }
  function addMaster() { masters = [...masters, { id: 0, name_ru: '', name_en: '', role_ru: '', role_en: '', color: '#2b8cff', active: true, service_ids: [], hours: [], hoursMap: {} }] }
  function toggleDay(m, wd) { m.hoursMap[wd] = m.hoursMap[wd] ? undefined : { weekday: wd, start_min: 600, end_min: 1200 } }
  const hhmm = (min) => `${String(Math.floor(min / 60)).padStart(2, '0')}:${String(min % 60).padStart(2, '0')}`
  const toMin = (s) => { const [h, m] = s.split(':').map(Number); return h * 60 + (m || 0) }

  // schedule geometry
  const minuteOfDay = (ts) => { const [h, m] = fmtTime(ts, tz).split(':').map(Number); return h * 60 + m }
  const top = (ts) => ((minuteOfDay(ts) - START * 60) / 60) * PX
  const height = (b) => ((b.end_at - b.start_at) / 3600) * PX
  const hoursOf = (m) => m.hours.find((h) => h.weekday === new Date(date + 'T12:00:00Z').getUTCDay())
</script>

{#if err}
  <div class="center"><p>{L[err]}</p></div>
{:else if !ready}
  <div class="center"><div class="spinner"></div></div>
{:else}
  <header>
    <div class="brand"><span class="logo">✿</span><b>Bloom</b><span class="hint">{L.title}</span></div>
    <nav>
      {#each ['schedule', 'bookings', 'services', 'masters'] as t}<button class:on={tab === t} onclick={() => setTab(t)}>{L[t]}</button>{/each}
    </nav>
    <div class="lang"><button class:on={lang === 'en'} onclick={() => (lang = 'en')}>EN</button><button class:on={lang === 'ru'} onclick={() => (lang = 'ru')}>RU</button></div>
  </header>

  <main>
    {#if tab === 'schedule' && ov}
      <div class="kpis">
        <div class="card kpi"><span class="hint">{L.kWeek}</span><b>{ov.kpi.week_bookings}</b></div>
        <div class="card kpi"><span class="hint">{L.kRub}</span><b>{money('RUB', ov.kpi.prepaid_rub_30d, lang)}</b></div>
        <div class="card kpi"><span class="hint">{L.kStars}</span><b>{money('XTR', ov.kpi.prepaid_stars_30d, lang)}</b></div>
        <div class="card kpi"><span class="hint">{L.kHold}</span><b>{ov.kpi.pending_holds}</b></div>
      </div>
      <div class="daynav">
        <button onclick={() => setDate(shift(date, -1))}>‹</button>
        <b class="cap">{fmtDay(Date.parse(date + 'T12:00:00Z') / 1000, 'UTC', lang, { weekday: 'long', day: 'numeric', month: 'long' })}</b>
        <button onclick={() => setDate(shift(date, 1))}>›</button>
        <button class="today" onclick={() => setDate(isoToday())}>{L.today}</button>
      </div>
      <div class="card grid-wrap">
        <div class="grid" style="--cols:{ov.masters.filter((m) => m.active).length}">
          <div class="corner"></div>
          {#each ov.masters.filter((m) => m.active) as m (m.id)}
            <div class="colhead"><span class="dot" style="background:{m.color}"></span>{pick(lang, { ru: m.name_ru, en: m.name_en })}<span class="hint">{pick(lang, { ru: m.role_ru, en: m.role_en })}</span></div>
          {/each}
          <div class="times">{#each Array(END - START) as _, i}<div style="height:{PX}px">{String(START + i).padStart(2, '0')}:00</div>{/each}</div>
          {#each ov.masters.filter((m) => m.active) as m (m.id)}
            {@const h = hoursOf(m)}
            <div class="col" style="height:{(END - START) * PX}px">
              {#if h}<div class="work" style="top:{((h.start_min - START * 60) / 60) * PX}px;height:{((h.end_min - h.start_min) / 60) * PX}px"></div>{:else}<div class="offday">{L.off}</div>{/if}
              {#each ov.bookings.filter((b) => b.master_id === m.id) as b (b.id)}
                <div class="appt st-{b.status}" style="top:{top(b.start_at)}px;height:{height(b) - 3}px;--c:{m.color}" title="{b.client_name} · {pick(lang, b.service)}">
                  <b>{fmtTime(b.start_at, tz)} {b.client_name}</b>
                  <span>{pick(lang, b.service)}</span>
                  {#if b.paid_at && height(b) > 60}<span class="paid">{money(b.pay_currency, b.pay_amount, lang)}</span>{/if}
                </div>
              {/each}
            </div>
          {/each}
        </div>
      </div>

    {:else if tab === 'bookings'}
      <div class="filters">
        {#each ['', 'confirmed', 'pending', 'completed', 'cancelled'] as s}<button class:on={statusFilter === s} onclick={() => { statusFilter = s; loadList() }}>{s ? L.st[s] : L.all}</button>{/each}
      </div>
      <div class="card table-wrap">
        <table>
          <thead><tr><th>#</th><th>{L.when}</th><th>{L.client}</th><th>{L.service}</th><th>{L.master}</th><th>{L.paid}</th><th>{L.status}</th><th></th></tr></thead>
          <tbody>
            {#each list as b (b.id)}
              <tr>
                <td class="hint">{b.id}</td>
                <td class="nowrap cap">{fmtDay(b.start_at, tz, lang)} {fmtTime(b.start_at, tz)}</td>
                <td>{b.client_name}<div class="hint">{b.client_phone}</div></td>
                <td>{pick(lang, b.service)}</td>
                <td><span class="dot" style="background:{b.master.color}"></span>{pick(lang, b.master)}</td>
                <td class="nowrap">{b.paid_at ? money(b.pay_currency, b.pay_amount, lang) : '—'}{#if b.refunded}<div class="hint">{L.refunded}</div>{/if}</td>
                <td><span class="pill st-{b.status}">{L.st[b.status] || b.status}</span></td>
                <td class="nowrap actions">
                  {#if b.status === 'confirmed'}
                    <button onclick={() => act(b, 'complete')}>{L.complete}</button>
                    <button class="bad" onclick={() => act(b, 'refund')}>{L.cancelRefund}</button>
                  {:else if b.status === 'pending'}<button class="bad" onclick={() => act(b, 'cancel')}>{L.cancel}</button>{/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

    {:else if tab === 'services'}
      <div class="card table-wrap">
        <table class="edit">
          <thead><tr><th>{L.nameRu}</th><th>{L.nameEn}</th><th>{L.duration}</th><th>{L.priceRub}</th><th>{L.priceStars}</th><th>{L.active}</th><th></th></tr></thead>
          <tbody>
            {#each services as s, i (s.id || 'new' + i)}
              {@const _ = (s.price_rub_view ??= s.price_rub / 100)}
              <tr>
                <td><input bind:value={s.name_ru} /></td><td><input bind:value={s.name_en} /></td>
                <td><input type="number" step="15" min="15" bind:value={s.duration_min} class="num" /></td>
                <td><input type="number" min="1" bind:value={s.price_rub_view} class="num" /></td>
                <td><input type="number" min="1" bind:value={s.price_stars} class="num" /></td>
                <td><input type="checkbox" bind:checked={s.active} /></td>
                <td><button class="save" onclick={() => saveService(s)}>{L.save}</button></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <button class="btn soft add" onclick={addService}>+ {L.newService}</button>

    {:else if tab === 'masters'}
      <div class="masters">
        {#each masters as m, i (m.id || 'new' + i)}
          <div class="card mcard">
            <div class="mhead"><input type="color" bind:value={m.color} /><input bind:value={m.name_ru} placeholder={L.nameRu} /><input bind:value={m.name_en} placeholder={L.nameEn} /><label class="chk"><input type="checkbox" bind:checked={m.active} />{L.active}</label></div>
            <div class="mrow"><input bind:value={m.role_ru} placeholder={L.roleRu} /><input bind:value={m.role_en} placeholder={L.roleEn} /></div>
            <div class="label">{L.does}</div>
            <div class="svcs">
              {#each services as s (s.id)}
                <label class="tag" class:on={m.service_ids.includes(s.id)}><input type="checkbox" checked={m.service_ids.includes(s.id)} onchange={(e) => (m.service_ids = e.currentTarget.checked ? [...m.service_ids, s.id] : m.service_ids.filter((x) => x !== s.id))} />{pick(lang, { ru: s.name_ru, en: s.name_en })}</label>
              {/each}
            </div>
            <div class="label">{L.hours}</div>
            <div class="week">
              {#each [1, 2, 3, 4, 5, 6, 0] as wd}
                {@const h = m.hoursMap[wd]}
                <div class="wday" class:off={!h}>
                  <button onclick={() => toggleDay(m, wd)}>{L.wd[wd]}</button>
                  {#if h}
                    <input type="time" step="1800" value={hhmm(h.start_min)} onchange={(e) => (h.start_min = toMin(e.currentTarget.value))} />
                    <input type="time" step="1800" value={hhmm(h.end_min)} onchange={(e) => (h.end_min = toMin(e.currentTarget.value))} />
                  {:else}<span class="hint">{L.off}</span>{/if}
                </div>
              {/each}
            </div>
            <button class="btn soft" onclick={() => saveMaster(m)}>{L.save}</button>
          </div>
        {/each}
      </div>
      <button class="btn soft add" onclick={addMaster}>+ {L.newMaster}</button>
    {/if}
  </main>
  {#if toast}<div class="toast">{toast}</div>{/if}
{/if}

<style>
  header { position: sticky; top: 0; z-index: 10; display: flex; align-items: center; gap: 16px; padding: 10px 20px; background: var(--card); border-bottom: 1px solid var(--line); flex-wrap: wrap; }
  .brand { display: flex; align-items: baseline; gap: 8px; }
  .logo { color: var(--btn); font-size: 22px; }
  nav { display: flex; gap: 4px; flex: 1; overflow-x: auto; }
  nav button, .filters button { border: 0; background: none; padding: 8px 12px; border-radius: 8px; font-weight: 600; color: var(--hint); white-space: nowrap; }
  nav button.on, .filters button.on { background: color-mix(in srgb, var(--btn) 12%, transparent); color: var(--btn); }
  .lang { display: flex; background: var(--bg2); border-radius: 8px; padding: 2px; }
  .lang button { border: 0; background: none; padding: 4px 8px; border-radius: 6px; font-size: 12px; font-weight: 600; color: var(--hint); }
  .lang button.on { background: var(--card); color: var(--text); }
  main { max-width: 1200px; margin: 0 auto; padding: 20px; }
  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
  .kpi { padding: 14px 16px; display: flex; flex-direction: column; gap: 4px; }
  .kpi b { font-size: 24px; }
  .daynav { display: flex; align-items: center; gap: 10px; margin: 18px 0 10px; }
  .daynav button { border: 1px solid var(--line); background: var(--card); border-radius: 8px; padding: 4px 12px; font-size: 18px; }
  .daynav .today { font-size: 14px; padding: 6px 12px; }
  .cap { text-transform: capitalize; }
  .grid-wrap { overflow-x: auto; padding: 12px; }
  .grid { display: grid; grid-template-columns: 56px repeat(var(--cols), minmax(150px, 1fr)); min-width: calc(56px + var(--cols) * 150px); }
  .colhead { display: flex; flex-direction: column; padding: 0 8px 10px; font-weight: 600; }
  .colhead .hint { font-weight: 400; font-size: 12px; }
  .dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 6px; }
  .times div { font-size: 11px; color: var(--hint); border-top: 1px solid var(--line); padding-top: 2px; }
  .col { position: relative; border-left: 1px solid var(--line); background: repeating-linear-gradient(to bottom, var(--line) 0 1px, transparent 1px 72px); }
  .work { position: absolute; left: 0; right: 0; background: color-mix(in srgb, var(--ok) 5%, transparent); }
  .offday { position: absolute; inset: 0; display: grid; place-items: center; color: var(--hint); background: repeating-linear-gradient(45deg, transparent 0 8px, color-mix(in srgb, var(--hint) 8%, transparent) 8px 16px); }
  .appt { position: absolute; left: 4px; right: 4px; border-radius: 8px; padding: 4px 8px; font-size: 12px; overflow: hidden; background: color-mix(in srgb, var(--c) 16%, var(--card)); border-left: 4px solid var(--c); display: flex; flex-direction: column; line-height: 1.25; }
  .appt.st-pending { opacity: .6; border-left-style: dashed; }
  .appt.st-completed { filter: grayscale(.5); opacity: .75; }
  .appt .paid { color: var(--ok); font-weight: 600; }
  .filters { display: flex; gap: 4px; margin-bottom: 12px; flex-wrap: wrap; }
  .table-wrap { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th { text-align: left; font-size: 12px; color: var(--hint); font-weight: 600; padding: 10px 12px; border-bottom: 1px solid var(--line); }
  td { padding: 10px 12px; border-bottom: 1px solid var(--line); vertical-align: top; }
  .nowrap { white-space: nowrap; }
  .actions button, .save { border: 1px solid var(--line); background: var(--card); border-radius: 8px; padding: 5px 10px; margin-right: 4px; font-size: 13px; }
  .actions .bad { color: var(--bad); }
  table.edit input:not([type=checkbox]) { width: 100%; border: 1px solid var(--line); border-radius: 8px; padding: 6px 8px; }
  table.edit .num { width: 90px; }
  .add { width: auto; display: inline-block; margin-top: 12px; }
  .masters { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 14px; }
  .mcard { padding: 14px; display: flex; flex-direction: column; gap: 8px; }
  .mhead, .mrow { display: flex; gap: 6px; align-items: center; }
  .mhead input:not([type]), .mrow input, .mhead input[type=text] { flex: 1; min-width: 0; border: 1px solid var(--line); border-radius: 8px; padding: 7px 9px; }
  .mhead input[type=color] { width: 34px; height: 34px; border: 0; padding: 0; background: none; }
  .chk { display: flex; gap: 4px; align-items: center; font-size: 13px; white-space: nowrap; }
  .label { font-size: 12px; color: var(--hint); font-weight: 600; text-transform: uppercase; margin-top: 4px; }
  .svcs { display: flex; flex-wrap: wrap; gap: 6px; }
  .tag { font-size: 13px; border: 1px solid var(--line); border-radius: 99px; padding: 4px 10px; cursor: pointer; }
  .tag input { display: none; }
  .tag.on { background: color-mix(in srgb, var(--btn) 12%, transparent); border-color: var(--btn); color: var(--btn); }
  .week { display: flex; flex-direction: column; gap: 4px; }
  .wday { display: flex; align-items: center; gap: 6px; font-size: 13px; }
  .wday button { width: 44px; border: 1px solid var(--line); border-radius: 6px; background: var(--card); padding: 3px 0; font-weight: 600; }
  .wday.off button { color: var(--hint); }
  .wday input { border: 1px solid var(--line); border-radius: 6px; padding: 3px 6px; }
  .center { min-height: 70vh; display: grid; place-items: center; text-align: center; padding: 24px; max-width: 520px; margin: 0 auto; }
  .spinner { width: 30px; height: 30px; border-radius: 50%; border: 3px solid var(--line); border-top-color: var(--btn); animation: spin .8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg) } }
  .toast { position: fixed; right: 20px; bottom: 20px; background: #222; color: #fff; padding: 10px 16px; border-radius: 10px; z-index: 50; }
  @media (max-width: 760px) { .kpis { grid-template-columns: 1fr 1fr; } main { padding: 12px; } header { padding: 10px 12px; } }
</style>
