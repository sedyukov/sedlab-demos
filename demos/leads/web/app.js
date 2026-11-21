(function () {
  'use strict';
  var I18N = {
    ru: {
      title: 'Заявка → CRM', tagline: 'Форма → CRM → Google Таблица → Telegram → оплата → сделка «Успешно»',
      ruStack: 'RU-стек', intlStack: 'International', site: 'Сайт клиента', widgetHint: 'встраиваемый JS-виджет',
      chaos: 'Симулировать сбои API', chaosHint: 'CRM 503, таблица 429 с Retry-After, Telegram 502: смотрите повторы в логе',
      reset: 'Сбросить песочницу', retention: 'У каждого посетителя своя песочница. Данные удаляются раз в сутки.',
      log: 'Лог событий', onlyLead: 'Только выбранная', clear: 'Очистить',
      logEmpty: 'Отправьте форму слева, и каждый шаг появится здесь в реальном времени.',
      sheetTab: 'Таблица', payTab: 'Оплата',
      foot: 'Демо собрано из модулей crm · payments · sheets-storage · notify (Go). Без ключей работает на sandbox-провайдерах с подписанными вебхуками.',
      sandboxCheckout: 'Тестовая оплата · sandbox', payBtn: 'Оплатить', declineBtn: 'Отказаться',
      coHint: 'Это имитация страницы оплаты: провайдер пришлёт нам подписанный вебхук.',
      steps: { 'form.submit': 'Заявка с сайта', 'crm.upsert_contact': 'Контакт', 'crm.create_deal': 'Сделка', 'sheets.append_rows': 'Строка в таблице',
        notify: 'Уведомление', 'payments.create_payment': 'Ссылка на оплату', 'webhook.payment': 'Вебхук оплаты', 'crm.update_deal': 'Сделка → итог' },
      stages: { New: 'Новая', Won: 'Успешно', Lost: 'Закрыто' }, noDeals: 'Сделок пока нет', noRows: 'Таблица пуста', noMsgs: 'Сообщений пока нет',
      noPays: 'Платежей пока нет. Отправьте заявку', pay: 'Оплатить…', replay: 'Повторить вебхук', forge: 'Поддельный вебхук',
      replayHint: 'Повтор: тот же вебхук ещё раз (идемпотентность). Подделка: чужая подпись (отклоняется).',
      chatName: 'Отдел продаж', bot: 'бот уведомлений', selected: 'Выбрана заявка', liveOn: 'онлайн', liveOff: 'переподключение…',
      st: { processing: 'обработка', awaiting_payment: 'ждёт оплаты', paid: 'оплачено', canceled: 'отменено', failed: 'ошибка',
        pending: 'ожидает', succeeded: 'оплачен' }, mock: 'песочница', live: 'боевой', fakeUrl: 'example-shop', payload: 'данные',
      badge: { ok: 'успех', retry: 'повтор', error: 'ошибка', duplicate: 'дубль', rejected: 'отклонён', info: 'инфо', start: 'старт' }
    },
    en: {
      title: 'Lead → CRM', tagline: 'Form → CRM → Google Sheet → Telegram → payment → deal won',
      ruStack: 'RU stack', intlStack: 'International', site: 'Client website', widgetHint: 'embeddable JS widget',
      chaos: 'Simulate API failures', chaosHint: 'CRM 503, Sheets 429 with Retry-After, Telegram 502: watch the retries in the log',
      reset: 'Reset sandbox', retention: 'Every visitor gets an isolated sandbox. Data is wiped daily.',
      log: 'Event log', onlyLead: 'Selected only', clear: 'Clear',
      logEmpty: 'Submit the form on the left and every step shows up here in real time.',
      sheetTab: 'Sheet', payTab: 'Payments',
      foot: 'Built from the crm · payments · sheets-storage · notify modules (Go). Without keys it runs on sandbox providers with signed webhooks.',
      sandboxCheckout: 'Test checkout · sandbox', payBtn: 'Pay', declineBtn: 'Decline',
      coHint: 'Simulated checkout page: the provider will send us a signed webhook.',
      steps: { 'form.submit': 'Website form', 'crm.upsert_contact': 'Contact', 'crm.create_deal': 'Deal', 'sheets.append_rows': 'Sheet row',
        notify: 'Notification', 'payments.create_payment': 'Payment link', 'webhook.payment': 'Payment webhook', 'crm.update_deal': 'Deal → result' },
      stages: { New: 'New', Won: 'Won', Lost: 'Lost' }, noDeals: 'No deals yet', noRows: 'Sheet is empty', noMsgs: 'No messages yet',
      noPays: 'No payments yet. Submit a lead', pay: 'Pay…', replay: 'Replay webhook', forge: 'Forged webhook',
      replayHint: 'Replay: the same webhook again (idempotency). Forged: wrong signature (rejected).',
      chatName: 'Sales team', bot: 'notification bot', selected: 'Selected lead', liveOn: 'live', liveOff: 'reconnecting…',
      st: { processing: 'processing', awaiting_payment: 'awaiting payment', paid: 'paid', canceled: 'canceled', failed: 'failed',
        pending: 'pending', succeeded: 'succeeded' }, mock: 'sandbox', live: 'live', fakeUrl: 'example-shop', payload: 'payload',
      badge: { ok: 'ok', retry: 'retry', error: 'error', duplicate: 'duplicate', rejected: 'rejected', info: 'info', start: 'start' }
    }
  };

  function ls(k, v) { try { if (v === undefined) return localStorage.getItem(k); localStorage.setItem(k, v); } catch (e) { return null; } }
  function uid() { return (crypto.randomUUID ? crypto.randomUUID() : String(Math.random()).slice(2) + Date.now()).replace(/-/g, ''); }
  var q = new URLSearchParams(location.search);
  var S = {
    session: ls('leads.session') || uid(),
    lang: q.get('lang') || ls('leads.lang') || ((navigator.language || 'ru').slice(0, 2) === 'ru' ? 'ru' : 'en'),
    profile: q.get('stack') || ls('leads.profile') || null,
    cfg: null, state: null, selected: null, onlyLead: false, lastSeq: 0, events: []
  };
  if (S.lang !== 'ru' && S.lang !== 'en') S.lang = 'en';
  if (!S.profile) S.profile = S.lang === 'ru' ? 'ru' : 'intl';
  ls('leads.session', S.session);

  var $ = function (s) { return document.querySelector(s); };
  function t(k) { return I18N[S.lang][k]; }
  // "crm.create_deal" → "crm › create_deal": dotted ids read like web addresses in screenshots
  function undot(s) { return String(s == null ? '' : s).replace(/([A-Za-z_]+)\.([A-Za-z_]+)/g, '$1 › $2'); }
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function money(m) {
    if (!m) return '';
    try { return new Intl.NumberFormat(S.lang === 'ru' ? 'ru-RU' : 'en-US', { style: 'currency', currency: m.currency, maximumFractionDigits: 0 }).format(m.value / 100); }
    catch (e) { return m.value / 100 + ' ' + m.currency; }
  }
  function api(path, body) {
    return fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.assign({ session: S.session }, body || {})) })
      .then(function (r) { return r.json(); });
  }
  function notifyNames(p) { return p.notify.map(function (c) { return { telegram: 'Telegram', max: 'Max', email: 'Email' }[c] || c; }); }
  function profile() { return S.cfg ? S.cfg.profiles.filter(function (p) { return p.key === S.profile; })[0] : null; }

  // ---------- i18n + switches
  function applyLang() {
    document.documentElement.lang = S.lang;
    document.querySelectorAll('[data-i18n]').forEach(function (el) { var v = t(el.dataset.i18n); if (typeof v === 'string') el.textContent = v; });
    document.querySelectorAll('#langSeg button').forEach(function (b) { b.setAttribute('aria-checked', String(b.dataset.lang === S.lang)); });
    $('#fakeUrl').textContent = t('fakeUrl');
    document.title = (S.lang === 'ru' ? 'Заявка → CRM' : 'Lead → CRM') + ' · live demo';
  }
  function applyProfile() {
    document.querySelectorAll('#profileSeg button').forEach(function (b) { b.setAttribute('aria-checked', String(b.dataset.profile === S.profile)); });
  }
  function syncURL() {
    var u = new URL(location.href); u.searchParams.set('lang', S.lang); u.searchParams.set('stack', S.profile);
    history.replaceState(null, '', u);
  }
  document.querySelectorAll('#langSeg button').forEach(function (b) {
    b.addEventListener('click', function () { S.lang = b.dataset.lang; ls('leads.lang', S.lang); applyLang(); syncURL(); if (widget) widget.setLang(S.lang); renderAll(); });
  });
  function bindProfileButtons() {
    document.querySelectorAll('#profileSeg button').forEach(function (b) {
      b.addEventListener('click', function () {
        if (S.profile === b.dataset.profile) return;
        S.profile = b.dataset.profile; ls('leads.profile', S.profile); S.selected = null;
        applyProfile(); syncURL(); if (widget) widget.setProfile(S.profile); refresh();
      });
    });
  }
  bindProfileButtons();

  // ---------- edition (EDITION=ru|intl on the server: one language, one set of integrations, link to the other edition)
  function applyEdition(c) {
    var keys = (c.profiles || []).map(function (p) { return p.key; });
    if (keys.length && keys.indexOf(S.profile) < 0) S.profile = keys[0];
    if (!c.edition) return;
    S.lang = c.edition === 'ru' ? 'ru' : 'en';
    $('#langSeg').hidden = true;
    var seg = $('#profileSeg');
    if (keys.length < 2) seg.hidden = true;
    else {
      seg.innerHTML = c.profiles.map(function (p) {
        return '<button role="radio" data-profile="' + esc(p.key) + '" aria-checked="false"><b>' + esc(p.crm) + '</b><small>' +
          esc(p.payments + ' · ' + p.sheets) + '</small></button>';
      }).join('');
      bindProfileButtons();
    }
    var a = $('#otherEdition');
    if (c.other_edition_url) {
      a.href = c.other_edition_url;
      a.textContent = c.edition === 'ru' ? 'English version' : 'Русская версия';
      a.hidden = false;
    }
    if (c.edition === 'ru') I18N.ru.tagline = 'Форма → CRM → таблица на Яндекс Диске → Telegram и Max → оплата → сделка «Успешно»';
  }

  // ---------- widget (mounted once the edition is known)
  var widget = null;
  function mountWidget() {
    widget = LeadsWidget.mount('#widget', { endpoint: location.origin, profile: S.profile, lang: S.lang, session: S.session, theme: 'auto',
      onSubmitted: function (j) { if (j.lead_id && j.lead_id !== 'L-000') { S.selected = j.lead_id; refresh(); } } });
  }

  // ---------- tools
  $('#chaos').addEventListener('change', function (e) { api('/api/session/chaos', { on: e.target.checked }); });
  $('#reset').addEventListener('click', function () {
    api('/api/session/reset').then(function () { S.events = []; S.selected = null; S.lastSeq = 0; renderLog(); connect(); refresh(); });
  });
  $('#clearLog').addEventListener('click', function () { S.events = []; renderLog(); });
  $('#filterLead').addEventListener('click', function (e) {
    S.onlyLead = !S.onlyLead; e.currentTarget.setAttribute('aria-pressed', String(S.onlyLead)); renderLog();
  });
  document.querySelectorAll('.tabs button').forEach(function (b) {
    b.addEventListener('click', function () {
      document.querySelectorAll('.tabs button').forEach(function (x) { x.setAttribute('aria-selected', String(x === b)); });
      document.querySelectorAll('.tabpanel').forEach(function (p) { p.hidden = p.id !== 'tab-' + b.dataset.tab; });
    });
  });

  // ---------- live log (SSE)
  var es = null;
  function connect() {
    if (es) es.close();
    es = new EventSource('/api/events?s=' + encodeURIComponent(S.session) + '&after=' + S.lastSeq);
    es.addEventListener('open', function () { $('#liveDot').className = 'live-dot on'; $('#liveDot').title = t('liveOn'); });
    es.addEventListener('error', function () { $('#liveDot').className = 'live-dot'; $('#liveDot').title = t('liveOff'); });
    es.addEventListener('reset', function () { S.events = []; S.lastSeq = 0; renderLog(); setTimeout(connect, 300); });
    es.addEventListener('log', function (m) {
      var ev = JSON.parse(m.data);
      if (ev.seq <= S.lastSeq) return;
      S.lastSeq = ev.seq;
      S.events.push(ev);
      if (S.events.length > 400) S.events.shift();
      appendLog(ev);
      if (ev.lead_id && (!S.selected || ev.step === 'form.submit')) S.selected = ev.lead_id;
      if (ev.step === 'settings' && ev.data) $('#chaos').checked = !!ev.data.on;
      scheduleRefresh();
    });
  }
  function logItem(ev) {
    var li = document.createElement('li');
    li.dataset.lead = ev.lead_id || '';
    var time = ev.ts.slice(11, 19);
    var meta = [];
    if (ev.attempt > 1 || ev.status === 'retry') meta.push('attempt ' + ev.attempt);
    if (ev.duration_ms) meta.push(ev.duration_ms + ' ms');
    li.innerHTML = '<time>' + time + '</time><span class="badge b-' + esc(ev.status) + '">' + esc(t('badge')[ev.status] || ev.status) + '</span>' +
      '<div class="ev-main"><div class="ev-step">' + (ev.lead_id ? '<span class="lid">' + esc(ev.lead_id) + '</span>' : '') + esc(undot(ev.step)) +
      (ev.provider ? '<span class="prov">· ' + esc(ev.provider) + '</span>' : '') + '</div>' +
      '<div class="ev-msg">' + esc(undot(ev.message)) + '</div>' +
      (meta.length ? '<div class="ev-meta">' + esc(meta.join(' · ')) + '</div>' : '') +
      (ev.data ? '<details class="ev-data"><summary>' + t('payload') + '</summary><pre>' + esc(JSON.stringify(ev.data, null, 2)) + '</pre></details>' : '') + '</div>';
    if (S.onlyLead && S.selected && ev.lead_id && ev.lead_id !== S.selected) li.classList.add('dim');
    return li;
  }
  function appendLog(ev) {
    var log = $('#log'), stick = log.scrollTop + log.clientHeight >= log.scrollHeight - 40;
    log.appendChild(logItem(ev));
    $('#logEmpty').hidden = true;
    if (stick) log.scrollTop = log.scrollHeight;
  }
  function renderLog() {
    var log = $('#log'); log.innerHTML = '';
    S.events.forEach(function (ev) { log.appendChild(logItem(ev)); });
    $('#logEmpty').hidden = S.events.length > 0;
    log.scrollTop = log.scrollHeight;
  }

  // ---------- state panels
  var timer = null;
  function scheduleRefresh() { clearTimeout(timer); timer = setTimeout(refresh, 120); }
  function refresh() {
    return fetch('/api/state?s=' + encodeURIComponent(S.session) + '&profile=' + S.profile).then(function (r) { return r.json(); })
      .then(function (st) { if (st.error) return; S.state = st; $('#chaos').checked = !!st.chaos; renderAll(); }).catch(function () {});
  }
  function lead() {
    if (!S.state) return null;
    var ls_ = S.state.leads || [];
    return ls_.filter(function (l) { return l.id === S.selected; })[0] || ls_[0] || null;
  }
  function renderAll() {
    var p = profile(), tab = document.querySelector('.tabs [data-tab="inbox"]');
    if (p && tab) tab.textContent = notifyNames(p).filter(function (n) { return n !== 'Email'; }).join(' / ');
    renderFlow(); renderCRM(); renderSheet(); renderInbox(); renderPay(); renderModes(); }

  function renderFlow() {
    var p = profile(), l = lead(), steps = t('steps');
    if (!p) return;
    var notifyName = notifyNames(p).join(' + ');
    var nodes = [
      ['form.submit', steps['form.submit'], S.lang === 'ru' ? 'JS-виджет' : 'JS widget'],
      ['crm.upsert_contact', steps['crm.upsert_contact'], p.crm],
      ['crm.create_deal', steps['crm.create_deal'], p.crm],
      ['sheets.append_rows', steps['sheets.append_rows'], p.sheets],
      ['notify', steps.notify, notifyName],
      ['payments.create_payment', steps['payments.create_payment'], p.payments],
      ['webhook.payment', steps['webhook.payment'], p.payments + ' → CRM'],
      ['crm.update_deal', steps['crm.update_deal'], p.crm]
    ];
    var st = function (k) {
      if (!l) return 0;
      if (k === 'notify') {
        var v = 0; p.notify.forEach(function (c) { v = Math.max(v, l.steps['notify.' + c] || 0); }); return v;
      }
      if (k === 'webhook.payment') return (l.status === 'paid' || l.status === 'canceled') ? 1 : (l.status === 'awaiting_payment' ? 4 : 0);
      return l.steps[k] || 0;
    };
    var html = '', firstPending = true;
    nodes.forEach(function (n, i) {
      var s = st(n[0]);
      if (s === 0 && l && l.status === 'processing' && firstPending) { s = 4; firstPending = false; }
      if (s === 4 && n[0] !== 'webhook.payment') firstPending = false;
      html += '<li data-s="' + s + '"><span class="n">' + (i + 1) + '</span><b>' + esc(n[1]) + '</b><small>' + esc(n[2]) + '</small></li>';
    });
    $('#flow').innerHTML = html;
    $('#flowLead').textContent = l ? t('selected') + ': ' + l.id + ' · ' + l.name + ' · ' + money(l.amount) + ' · ' + (t('st')[l.status] || l.status) : '';
  }
  function sys(name, mode) {
    return '<div class="sys"><span>' + esc(name) + '</span><span class="mode ' + mode + '">' + t(mode) + '</span></div>';
  }
  function renderCRM() {
    var p = profile(), el = $('#tab-crm');
    if (!p) return;
    var leads = (S.state && S.state.leads) || [], cols = { New: [], Won: [], Lost: [] };
    leads.forEach(function (l) { if (l.deal_id) (cols[l.deal_stage] || cols.New).push(l); });
    var html = sys(p.crm + ' · ' + (S.lang === 'ru' ? 'воронка продаж' : 'sales pipeline'), p.modes.crm) + '<div class="kanban">';
    ['New', 'Won', 'Lost'].forEach(function (k) {
      html += '<div class="kcol"><h4>' + t('stages')[k] + '<span>' + cols[k].length + '</span></h4>';
      cols[k].forEach(function (l) {
        html += '<div class="deal' + (lead() && lead().id === l.id ? ' sel' : '') + '" data-id="' + esc(l.id) + '"><b>' + esc(l.name) + '</b>' +
          '<span class="amt">' + money(l.amount) + '</span><div class="id">' + esc(l.deal_id) + ' · ' + esc(l.id) + '</div></div>';
      });
      html += '</div>';
    });
    el.innerHTML = html + '</div>' + (leads.length ? '' : '<p class="empty">' + t('noDeals') + '</p>');
    el.querySelectorAll('.deal').forEach(function (d) { d.addEventListener('click', function () { S.selected = d.dataset.id; renderAll(); if (S.onlyLead) renderLog(); }); });
  }
  function renderSheet() {
    var p = profile(), el = $('#tab-sheet');
    if (!p) return;
    var rows = (S.state && S.state.sheet) || [];
    var html = sys(p.sheets + (S.cfg.edition === 'ru' ? ' · leads.xlsx' : ' · Leads'), p.modes.sheets);
    if (!rows.length) { el.innerHTML = html + '<p class="empty">' + t('noRows') + '</p>'; return; }
    var cols = Math.max.apply(null, rows.map(function (r) { return r.length; }));
    html += '<div class="sheet-wrap"><table class="sheet"><thead><tr><th class="rn"></th>';
    for (var c = 0; c < cols; c++) html += '<th>' + String.fromCharCode(65 + c) + '</th>';
    html += '</tr></thead><tbody>';
    rows.forEach(function (r, i) {
      html += '<tr class="' + (i === 0 ? 'hdr' : '') + (i === rows.length - 1 && i > 0 ? ' flash' : '') + '"><td class="rn">' + (i + 1) + '</td>';
      for (var c = 0; c < cols; c++) {
        var v = r[c] || '', cls = v === 'Won' ? 'won' : v === 'Lost' ? 'lost' : '';
        html += '<td class="' + cls + '" title="' + esc(v) + '">' + esc(v) + '</td>';
      }
      html += '</tr>';
    });
    el.innerHTML = html + '</tbody></table></div>';
  }
  function renderInbox() {
    var p = profile(), el = $('#tab-inbox');
    if (!p) return;
    var msgs = (S.state && S.state.messages) || [];
    var html = sys(notifyNames(p).join(' + ') + ' · notify', p.modes.notify) +
      '<div class="chat"><div class="chat-h"><span class="av">S</span><div><b>' + t('chatName') + '</b><br>' + t('bot') + '</div></div>';
    if (!msgs.length) html += '<p class="empty">' + t('noMsgs') + '</p>';
    msgs.forEach(function (m) {
      html += '<div class="bubble ' + esc(m.channel) + '">' + (m.channel !== 'telegram' ? '<span class="ch">' + esc(m.channel) + ' → ' + esc(m.to) + '</span>\n' : '') +
        esc(m.text) + '<span class="meta">' + esc(m.id) + ' · ' + esc((m.created_at || '').slice(11, 16)) + ' UTC</span></div>';
    });
    el.innerHTML = html + '</div>';
    el.scrollTop = el.scrollHeight;
  }
  function renderPay() {
    var p = profile(), el = $('#tab-pay');
    if (!p) return;
    var leads = ((S.state && S.state.leads) || []).filter(function (l) { return l.payment_id; });
    var html = sys(p.payments, p.modes.payments);
    if (!leads.length) { el.innerHTML = html + '<p class="empty">' + t('noPays') + '</p>'; return; }
    leads.forEach(function (l) {
      var ps = l.payment_status || 'pending', done = ps !== 'pending';
      html += '<div class="pay"><div class="pay-h"><b>' + esc(l.id) + ' · ' + esc(l.name) + '</b><span class="pst ' + esc(ps) + '">' + esc(t('st')[ps] || ps) + '</span></div>' +
        '<div><b>' + money(l.amount) + '</b> <span class="id">' + esc(l.payment_id) + '</span></div><div class="pay-actions">' +
        (!done && p.modes.payments === 'mock' ? '<button class="primary" data-act="pay" data-id="' + esc(l.id) + '">' + t('pay') + '</button>' : '') +
        (!done && p.modes.payments === 'live' ? '<a class="primary" href="' + esc(l.confirmation_url) + '" target="_blank" rel="noopener">' + t('pay') + '</a>' : '') +
        (done && p.modes.payments === 'mock' ? '<button class="ghost" data-act="replay-webhook" data-id="' + esc(l.id) + '">↻ ' + t('replay') + '</button>' : '') +
        '<button class="ghost warnbtn" data-act="forge-webhook" data-id="' + esc(l.id) + '">⚠ ' + t('forge') + '</button></div></div>';
    });
    el.innerHTML = html + '<p class="hint">' + t('replayHint') + '</p>';
    el.querySelectorAll('[data-act]').forEach(function (b) {
      b.addEventListener('click', function () {
        var l = leads.filter(function (x) { return x.id === b.dataset.id; })[0];
        S.selected = l.id;
        if (b.dataset.act === 'pay') return openCheckout(l);
        api('/api/leads/' + l.id + '/' + b.dataset.act);
      });
    });
  }
  function renderModes() {
    var p = profile();
    if (!p) return;
    $('#modes').innerHTML = [[p.crm, p.modes.crm], [p.payments, p.modes.payments], [p.sheets, p.modes.sheets], ['notify', p.modes.notify]]
      .map(function (x) { return '<span>' + esc(x[0]) + ' <span class="mode ' + x[1] + '">' + t(x[1]) + '</span></span>'; }).join('');
  }

  // ---------- checkout dialog
  var dlg = $('#checkout'), coLead = null;
  function openCheckout(l) {
    coLead = l;
    $('#coTitle').textContent = (S.lang === 'ru' ? 'Заказ ' : 'Order ') + l.id + ' · ' + profile().payments;
    $('#coAmount').textContent = money(l.amount);
    if (dlg.showModal) dlg.showModal(); else dlg.setAttribute('open', '');
  }
  dlg.addEventListener('close', function () {
    if (coLead && (dlg.returnValue === 'succeeded' || dlg.returnValue === 'canceled')) {
      api('/api/leads/' + coLead.id + '/pay', { outcome: dlg.returnValue });
      document.querySelector('.tabs [data-tab="crm"]').click();
    }
    coLead = null;
  });

  // ---------- boot
  applyLang(); applyProfile();
  fetch('/api/config').then(function (r) { return r.json(); }).then(function (c) {
    S.cfg = c;
    applyEdition(c); applyLang(); applyProfile(); syncURL(); mountWidget(); refresh();
  });
  connect();
  window.leadsDemo = S; // for e2e tests
})();
