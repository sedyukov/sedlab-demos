<script>
  // Mock card checkout (payments module, provider=mock). It stands in for the provider's hosted page:
  // "Pay" → mock_complete → signed webhook → our /payments webhook logic → booking confirmed.
  // No card data is collected — the demo card is fixed and read-only.
  import { onMount } from 'svelte'
  import { money } from '../lib/fmt.js'
  const q = new URLSearchParams(location.search)
  const lang = q.get('lang') === 'ru' ? 'ru' : 'en'
  const embed = q.get('embed') === '1'
  const pid = location.pathname.split('/').filter(Boolean).pop()
  const T = {
    en: { title: 'Demo checkout', sub: 'Sandbox of the card payment provider: no real money moves', pay: 'Pay', cancel: 'Cancel payment', card: 'Test card', ok: 'Payment received', canceled: 'Payment cancelled', back: 'Back to the app', nf: 'Payment not found', note: 'In production this is the provider’s hosted page (YooKassa / Stripe via the payments module).' },
    ru: { title: 'Демо-оплата', sub: 'Песочница платёжного провайдера: реальные деньги не списываются', pay: 'Оплатить', cancel: 'Отменить платёж', card: 'Тестовая карта', ok: 'Оплата прошла', canceled: 'Платёж отменён', back: 'Вернуться в приложение', nf: 'Платёж не найден', note: 'В проде здесь страница провайдера (ЮKassa / Stripe через модуль payments).' },
  }[lang]
  let p = $state(null), state = $state('loading'), busy = $state(false)

  onMount(async () => {
    const r = await fetch(`/demo/checkout/${encodeURIComponent(pid)}`)
    if (!r.ok) { state = 'nf'; return }
    p = await r.json()
    state = p.status === 'pending' ? 'form' : p.status === 'succeeded' ? 'ok' : 'canceled'
  })

  async function complete(outcome) {
    busy = true
    const r = await fetch(`/demo/checkout/${encodeURIComponent(pid)}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ outcome }) })
    busy = false
    state = r.ok ? (outcome === 'succeeded' ? 'ok' : 'canceled') : 'nf'
    setTimeout(done, 900)
  }
  function done() {
    if (embed && window.parent !== window) window.parent.postMessage({ type: 'checkout_done', outcome: state === 'ok' ? 'succeeded' : 'canceled' }, location.origin)
    else if (p?.return_url) location.href = p.return_url
  }
</script>

<div class="wrap">
  <div class="card box">
    <div class="brand"><span class="lock">🔒</span><div><b>{T.title}</b><div class="hint">{T.sub}</div></div></div>
    {#if state === 'loading'}<p class="hint">…</p>
    {:else if state === 'nf'}<p>{T.nf}</p>
    {:else if p}
      <div class="amount">{money(p.amount.currency, p.amount.value, lang)}</div>
      <div class="desc">{p.description}</div>
      {#if state === 'form'}
        <div class="cardnum"><span class="hint">{T.card}</span><b>5555 5555 5555 4477</b><span class="hint">12/30 · 123</span></div>
        <button class="btn" disabled={busy} onclick={() => complete('succeeded')}>{T.pay} {money(p.amount.currency, p.amount.value, lang)}</button>
        <button class="btn ghost" disabled={busy} onclick={() => complete('canceled')}>{T.cancel}</button>
      {:else}
        <div class="result" class:ok={state === 'ok'}>{state === 'ok' ? '✓ ' + T.ok : T.canceled}</div>
        <button class="btn soft" onclick={done}>{T.back}</button>
      {/if}
    {/if}
    <p class="hint note">{T.note}</p>
  </div>
</div>

<style>
  .wrap { min-height: 100vh; display: grid; place-items: center; padding: 16px; }
  .box { width: 100%; max-width: 380px; padding: 20px; display: flex; flex-direction: column; gap: 12px; box-shadow: 0 6px 30px #0001; }
  .brand { display: flex; gap: 10px; align-items: center; }
  .lock { font-size: 22px; }
  .amount { font-size: 32px; font-weight: 700; margin-top: 6px; }
  .desc { color: var(--hint); }
  .cardnum { display: flex; flex-direction: column; gap: 2px; padding: 14px; border-radius: 12px; background: linear-gradient(135deg, #2b2d42, #4a4e69); color: #fff; }
  .cardnum .hint { color: #fffa; }
  .cardnum b { font-size: 18px; letter-spacing: 1px; }
  .result { font-size: 18px; font-weight: 600; color: var(--bad); }
  .result.ok { color: var(--ok); }
  .note { font-size: 11px; }
</style>
