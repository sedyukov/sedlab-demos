(function () {
  'use strict';
  var I18N = {
    ru: {
      title: 'Заявки с сайта: сразу в CRM, таблицу и мессенджер',
      tagline: 'Отправьте тестовую заявку и посмотрите, как она проходит путь до оплаты. Это займёт 30 секунд.',
      ruStack: 'RU-стек', intlStack: 'International', site: '1. Форма на вашем сайте', feed: '2. Что происходит', where: '3. Где это видит ваша команда',
      retention: 'Данные демо удаляются автоматически, не позже чем через сутки.',
      feedEmpty: 'Нажмите «Отправить заявку» слева: здесь по шагам появится, что система делает с заявкой.',
      sheetTab: 'Таблица', inboxTab: 'Мессенджер', payTab: 'Оплата', crmQ: 'Какая у вас CRM?',
      offer: 'Такую же связку для вашего сайта: от 20 тыс. ₽, срок 2-5 дней. Подключаю вашу CRM, оплату, таблицу и мессенджер.', contact: 'Обсудить задачу',
      techTitle: 'Как это устроено (для технических специалистов)',
      chaos: 'Симулировать сбои API', chaosHint: 'CRM 503, таблица 429 с Retry-After, мессенджер 502: смотрите повторы в журнале',
      reset: 'Начать заново', log: 'Журнал событий', onlyLead: 'Только выбранная', clear: 'Очистить', logEmpty: 'Пока пусто.',
      foot: 'Демо собрано из модулей crm · payments · sheets-storage · notify (Go). Повторы с паузой и Retry-After, ключи идемпотентности, вебхуки с подписью HMAC. Форма: встраиваемый JS-виджет.',
      sandboxCheckout: 'Имитация оплаты', payBtn: 'Оплатить', declineBtn: 'Отказаться',
      coHint: 'Так клиент оплачивает заказ. В настоящей интеграции здесь откроется страница платёжной системы, а она сама сообщит нам об оплате.',
      flow: ['Заявка с сайта', 'Сделка в CRM', 'Строка в таблице', 'Уведомление менеджеру', 'Оплата и итог сделки'],
      formNode: 'ваш сайт', sim: 'имитация', real: 'настоящий сервис',
      svc: { crm: 'CRM', sheets: 'Таблица', notify: 'Мессенджер', payments: 'Оплата' },
      honestAll: 'Честно о демо: CRM, таблица, мессенджер и оплата здесь показаны в режиме имитации. Так это будет выглядеть у вас, но настоящие сервисы к этой странице пока не подключены.',
      honestSome: 'Честно о демо: настоящие сервисы отмечены зелёным ({live}). Остальное показано в режиме имитации ({sim}).',
      honestNone: 'Все сервисы на этой странице настоящие (тестовые аккаунты).',
      simBanner: { crm: 'Имитация: так сделка выглядит в воронке CRM. Настоящая CRM к демо не подключена.',
        sheets: 'Имитация: так заявки складываются в таблицу. Настоящая таблица к демо не подключена.',
        notify: 'Имитация: такое сообщение получает менеджер. Настоящий мессенджер к демо не подключён.',
        payments: 'Имитация: так выглядят платежи по заявкам. Настоящая платёжная система к демо не подключена.' },
      realBanner: 'Настоящий сервис: {name}, тестовый аккаунт.',
      ev: {
        accepted: 'Заявка принята: {name}, тариф «{plan}», {amount}',
        crmOk: 'В CRM создана сделка {deal}, этап «Новая»', crmRetry: 'CRM не ответила, система повторяет попытку', crmErr: 'CRM недоступна. Заявка не потеряна, её можно отправить повторно',
        sheetOk: 'Заявка записана строкой в таблицу', sheetRetry: 'Таблица попросила подождать, повторяем', sheetErr: 'Не удалось записать в таблицу',
        notifyOk: 'Менеджер получил уведомление в мессенджер', notifyRetry: 'Мессенджер не ответил, повторяем', notifyErr: 'Уведомление не доставлено',
        payWait: 'Клиенту готова ссылка на оплату. Ждём оплату', payOk: 'Оплата получена: сделка в CRM закрыта как успешная, таблица обновлена',
        payNo: 'Клиент отказался от оплаты: сделка закрыта как проигранная', failed: 'Обработка остановлена на шаге с ошибкой',
        payAs: 'Оплатить как клиент', after: 'через {s} с', instant: 'за доли секунды'
      },
      stages: { New: 'Новая', Won: 'Успешно', Lost: 'Закрыто' }, noDeals: 'Сделок пока нет', noRows: 'Таблица пуста', noMsgs: 'Сообщений пока нет',
      noPays: 'Платежей пока нет. Отправьте заявку', pay: 'Оплатить…', replay: 'Повторить вебхук оплаты', forge: 'Отправить поддельный вебхук',
      replayHint: 'Повтор: тот же вебхук ещё раз (идемпотентность, станет «дубль»). Подделка: чужая подпись (отклоняется).',
      chatName: 'Отдел продаж', bot: 'бот уведомлений', selected: 'Выбрана заявка', liveOn: 'онлайн', liveOff: 'переподключение…',
      st: { processing: 'обработка', awaiting_payment: 'ждёт оплаты', paid: 'оплачено', canceled: 'отменено', failed: 'ошибка',
        pending: 'ожидает', succeeded: 'оплачен' }, mock: 'имитация', live: 'настоящий', fakeUrl: 'example-shop', payload: 'данные',
      badge: { ok: 'успех', retry: 'повтор', error: 'ошибка', duplicate: 'дубль', rejected: 'отклонён', info: 'инфо', start: 'старт' }
    },
    en: {
      title: 'Website leads straight into CRM, a sheet and a messenger',
      tagline: 'Send a test lead and watch it travel all the way to payment. Takes 30 seconds.',
      ruStack: 'RU stack', intlStack: 'International', site: '1. The form on your website', feed: '2. What happens', where: '3. Where your team sees it',
      retention: 'Demo data is deleted automatically within a day.',
      feedEmpty: 'Press “Send request” on the left: every step the system takes shows up here.',
      sheetTab: 'Sheet', inboxTab: 'Messenger', payTab: 'Payments', crmQ: 'Which CRM do you use?',
      offer: 'The same setup for your website in 2-5 days. Your CRM, payments, sheet and messenger.', contact: 'Discuss your task',
      techTitle: 'How it works (for engineers)',
      chaos: 'Simulate API failures', chaosHint: 'CRM 503, Sheets 429 with Retry-After, messenger 502: watch the retries in the log',
      reset: 'Start over', log: 'Event log', onlyLead: 'Selected only', clear: 'Clear', logEmpty: 'Nothing yet.',
      foot: 'Built from the crm · payments · sheets-storage · notify modules (Go). Backoff with Retry-After, idempotency keys, HMAC-signed webhooks. The form is an embeddable JS widget.',
      sandboxCheckout: 'Simulated payment', payBtn: 'Pay', declineBtn: 'Decline',
      coHint: 'This is how the customer pays. In a real integration the payment provider page opens here and the provider notifies us.',
      flow: ['Website form', 'Deal in CRM', 'Row in a sheet', 'Manager notified', 'Payment and result'],
      formNode: 'your website', sim: 'simulation', real: 'real service',
      svc: { crm: 'CRM', sheets: 'Sheet', notify: 'Messenger', payments: 'Payments' },
      honestAll: 'Honest note: CRM, sheet, messenger and payments are simulated here. This is how it will look for you, but no real services are connected to this page yet.',
      honestSome: 'Honest note: real services are marked green ({live}). The rest is simulated ({sim}).',
      honestNone: 'Every service on this page is real (test accounts).',
      simBanner: { crm: 'Simulation: this is how the deal looks in a CRM pipeline. No real CRM is connected.',
        sheets: 'Simulation: this is how leads land in a sheet. No real sheet is connected.',
        notify: 'Simulation: this is the message a manager gets. No real messenger is connected.',
        payments: 'Simulation: payments per lead. No real payment provider is connected.' },
      realBanner: 'Real service: {name}, test account.',
      ev: {
        accepted: 'Lead received: {name}, plan “{plan}”, {amount}',
        crmOk: 'Deal {deal} created in the CRM, stage “New”', crmRetry: 'The CRM did not answer, retrying', crmErr: 'The CRM is down. The lead is not lost and can be resent',
        sheetOk: 'The lead is written as a row in the sheet', sheetRetry: 'The sheet asked to wait, retrying', sheetErr: 'Could not write to the sheet',
        notifyOk: 'The manager got a messenger notification', notifyRetry: 'The messenger did not answer, retrying', notifyErr: 'Notification not delivered',
        payWait: 'Payment link is ready. Waiting for payment', payOk: 'Payment received: the deal is won, the sheet is updated',
        payNo: 'The customer declined: the deal is closed as lost', failed: 'Processing stopped at the failed step',
        payAs: 'Pay as the customer', after: 'after {s} s', instant: 'in a split second'
      },
      stages: { New: 'New', Won: 'Won', Lost: 'Lost' }, noDeals: 'No deals yet', noRows: 'Sheet is empty', noMsgs: 'No messages yet',
      noPays: 'No payments yet. Submit a lead', pay: 'Pay…', replay: 'Replay payment webhook', forge: 'Send a forged webhook',
      replayHint: 'Replay: the same webhook again (idempotency, becomes “duplicate”). Forged: wrong signature (rejected).',
      chatName: 'Sales team', bot: 'notification bot', selected: 'Selected lead', liveOn: 'live', liveOff: 'reconnecting…',
      st: { processing: 'processing', awaiting_payment: 'awaiting payment', paid: 'paid', canceled: 'canceled', failed: 'failed',
        pending: 'pending', succeeded: 'succeeded' }, mock: 'simulation', live: 'real', fakeUrl: 'example-shop', payload: 'payload',
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
  function fill(str, o) { return String(str).replace(/\{(\w+)\}/g, function (_, k) { return o[k] == null ? '' : o[k]; }); }
  function dealNo(id) { return '№' + String(id || '').replace(/^[^0-9]+/, ''); }
  function profile() { return S.cfg ? S.cfg.profiles.filter(function (p) { return p.key === S.profile; })[0] : null; }
  // honest naming: a service shows its real name only when it is really connected; a simulation is just "CRM", "Оплата"…
  function isLive(k) { var l = lead(); if (l && l.live) { var p = profile(); return !!(p && p.modes[k] === 'live'); } return eff(k) === 'live'; }
  function realName(k) { var p = profile(); if (!p) return ''; return k === 'notify' ? notifyChans(p).join(' / ') : p[k]; }
  function svcName(k) { return isLive(k) ? realName(k) : t('svc')[k]; }
  function notifyChans(p) { return p.notify.filter(function (c) { return c !== 'email'; }).map(function (c) { return { telegram: 'Telegram', max: 'Max' }[c] || c; }); }

  // ---------- i18n + switches
  function applyLang() {
    document.documentElement.lang = S.lang;
    document.querySelectorAll('[data-i18n]').forEach(function (el) { var v = t(el.dataset.i18n); if (typeof v === 'string') el.textContent = v; });
    document.querySelectorAll('#langSeg button').forEach(function (b) { b.setAttribute('aria-checked', String(b.dataset.lang === S.lang)); });
    $('#fakeUrl').textContent = t('fakeUrl');
    document.title = S.lang === 'ru' ? 'Заявки с сайта в CRM: демо' : 'Website leads into CRM: demo';
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
        applyProfile(); syncURL(); if (widget) widget.setProfile(S.profile); refresh(); refreshMode();
      });
    });
  }
  bindProfileButtons();

  // ---------- edition (EDITION=ru|intl on the server: one language, one set of integrations, link to the other edition)
  function applyEdition(c) {
    var keys = (c.profiles || []).map(function (p) { return p.key; });
    if (keys.length && keys.indexOf(S.profile) < 0) S.profile = keys[0];
    if (c.contact_url) { var ct = $('#contact'); ct.href = c.contact_url; ct.target = '_blank'; ct.rel = 'noopener'; ct.hidden = false; }
    if (!c.edition) return;
    S.lang = c.edition === 'ru' ? 'ru' : 'en';
    $('#langSeg').hidden = true;
    // a CRM choice only makes sense between really connected CRMs; simulations all look the same
    var real = (c.profiles || []).filter(function (p) { return p.modes.crm === 'live'; });
    if (real.length && !real.some(function (p) { return p.key === S.profile; })) S.profile = real[0].key;
    var seg = $('#profileSeg');
    if (real.length < 2) $('#profileWrap').hidden = true;
    else {
      seg.innerHTML = real.map(function (p) {
        return '<button role="radio" data-profile="' + esc(p.key) + '" aria-checked="false"><b>' + esc(p.crm) + '</b></button>';
      }).join('');
      $('#profileLabel').textContent = I18N[S.lang].crmQ; $('#profileLabel').hidden = false;
      bindProfileButtons();
    }
    var a = $('#otherEdition');
    if (c.other_edition_url) {
      a.href = c.other_edition_url;
      a.textContent = c.edition === 'ru' ? 'English version' : 'Русская версия';
      a.hidden = false;
    }
  }

  // ---------- sign-in (RU edition): auth widget renders the button and limits; the banner says simulation or real
  function mountAuth(c) {
    if (!c.auth_widget || !c.auth_widget.url) return;
    var box = document.createElement('div'); box.id = 'sl-account'; box.className = 'acct';
    $('.controls').insertBefore(box, $('.controls').firstChild);
    var sc = document.createElement('script');
    sc.src = c.auth_widget.url; sc.defer = true;
    sc.setAttribute('data-demo', c.auth_widget.demo); sc.setAttribute('data-lang', S.lang);
    var p = profile();
    if (p) {
      sc.setAttribute('data-resource', p.resource);
      var live = ['crm', 'payments'].filter(function (k) { return p.modes[k] === 'live'; }).map(function (k) { return p[k]; });
      if (live.length) sc.setAttribute('data-providers', live.join(', ') + ' (Россия)');
    }
    document.body.appendChild(sc);
    window.addEventListener('sedlab:account', function () { refreshMode(); });
  }
  function refreshMode() {
    if (!S.cfg || !S.cfg.auth_widget) return;
    fetch('/api/mode?profile=' + S.profile, { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (m) {
      var b = $('#modeBanner');
      if (!m.gated) { b.hidden = true; return; }
      b.hidden = false; b.className = 'mode-banner ' + (m.live ? 'on' : 'off');
      b.textContent = m.live ? (S.lang === 'ru' ? 'Заявка уйдёт в настоящую CRM (тестовый аккаунт). Осталось сегодня: ' : 'The lead goes to the real CRM (test account). Left today: ') + m.user_left : m.message;
    }).catch(function () {});
  }

  // ---------- widget (mounted once the edition is known)
  var widget = null;
  function mountWidget() {
    widget = LeadsWidget.mount('#widget', { endpoint: location.origin, profile: S.profile, lang: S.lang, session: S.session, theme: 'auto',
      onSubmitted: function (j) { if (j.lead_id && j.lead_id !== 'L-000') { S.selected = j.lead_id; refresh(); setTimeout(refreshMode, 1500); } } });
  }

  // ---------- tools (inside the engineers' block)
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

  // ---------- event stream (SSE): feeds the people's timeline and the engineers' log
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
    if (ev.attempt > 1 || ev.status === 'retry') meta.push((S.lang === 'ru' ? 'попытка ' : 'attempt ') + ev.attempt);
    if (ev.duration_ms) meta.push(ev.duration_ms + (S.lang === 'ru' ? ' мс' : ' ms'));
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
    var tab = document.querySelector('.tabs [data-tab="inbox"]');
    if (tab) tab.textContent = svcName('notify');
    var tc = document.querySelector('.tabs [data-tab="crm"]');
    if (tc) tc.textContent = svcName('crm');
    renderHonesty(); renderFlow(); renderFeed(); renderCRM(); renderSheet(); renderInbox(); renderPay(); renderTech(); renderModes();
  }

  function renderHonesty() {
    var keys = ['crm', 'sheets', 'notify', 'payments'];
    var live = keys.filter(isLive), sim = keys.filter(function (k) { return !isLive(k); });
    var el = $('#honesty');
    el.className = 'honesty' + (live.length ? ' some' : '');
    el.textContent = !live.length ? t('honestAll') : !sim.length ? t('honestNone') :
      fill(t('honestSome'), { live: live.map(realName).join(', '), sim: sim.map(function (k) { return t('svc')[k]; }).join(', ') });
  }

  // step state for the people's view: 0 waiting, 1 done, 2 retrying, 3 failed, 4 in progress
  function combine(vals) {
    if (vals.indexOf(3) >= 0) return 3;
    if (vals.indexOf(2) >= 0) return 2;
    return vals.every(function (v) { return v === 1; }) ? 1 : 0;
  }
  function stepState(l, k) {
    if (!l) return 0;
    var p = profile();
    if (k === 'crm') return combine([l.steps['crm.upsert_contact'] || 0, l.steps['crm.create_deal'] || 0]);
    if (k === 'notify') return combine(p.notify.map(function (c) { return l.steps['notify.' + c] || 0; }));
    if (k === 'payments') return (l.status === 'paid' || l.status === 'canceled') ? 1 : (l.status === 'awaiting_payment' ? 4 : 0);
    return l.steps[k] || 0;
  }
  function renderFlow() {
    var p = profile(), l = lead(), names = t('flow');
    if (!p) return;
    var nodes = [[null, 'form.submit'], ['crm', 'crm'], ['sheets', 'sheets.append_rows'], ['notify', 'notify'], ['payments', 'payments']];
    var html = '', firstPending = true;
    nodes.forEach(function (n, i) {
      var k = n[0], s = k ? stepState(l, n[1]) : (l ? 1 : 0);
      if (s === 0 && l && l.status === 'processing' && firstPending) { s = 4; firstPending = false; }
      if (s === 4 && k !== 'payments') firstPending = false;
      var sub = k ? (isLive(k) ? '<span class="tagl real">' + esc(realName(k)) + '</span>' : '<span class="tagl sim">' + esc(t('sim')) + '</span>') : esc(t('formNode'));
      html += '<li data-s="' + s + '"><span class="n">' + (i + 1) + '</span><b>' + esc(names[i]) + '</b><small>' + sub + '</small></li>';
    });
    $('#flow').innerHTML = html;
  }

  function firstOk(l, step) {
    for (var i = 0; i < S.events.length; i++) {
      var e = S.events[i];
      if (e.lead_id === l.id && e.status === 'ok' && (e.step === step || (step === 'notify' && e.step.indexOf('notify.') === 0))) return e;
    }
    return null;
  }
  function renderFeed() {
    var l = lead(), el = $('#feed');
    $('#feedEmpty').hidden = !!l;
    if (!l) { el.innerHTML = ''; return; }
    var ev = t('ev'), p = profile(), t0 = firstOk(l, 'form.submit');
    var planLabel = S.lang === 'ru' ? ({ start: 'Старт', business: 'Бизнес', pro: 'Про' }[l.plan] || l.plan) : ({ start: 'Starter', business: 'Business', pro: 'Pro' }[l.plan] || l.plan);
    function when(step) {
      var e = firstOk(l, step);
      if (!e || !t0) return '';
      var d = (Date.parse(e.ts) - Date.parse(t0.ts)) / 1000;
      return d < 0.1 ? ev.instant : fill(ev.after, { s: d.toLocaleString(S.lang === 'ru' ? 'ru-RU' : 'en-US', { maximumFractionDigits: 1 }) });
    }
    var items = [];
    function add(state, text, svc, step) { items.push({ s: state, text: text, svc: svc, when: state === 1 ? when(step) : '' }); }
    add(1, fill(ev.accepted, { name: l.name, plan: planLabel, amount: money(l.amount) }), null, 'form.submit');
    var c = stepState(l, 'crm');
    if (c) add(c, c === 1 ? fill(ev.crmOk, { deal: l.deal_id ? dealNo(l.deal_id) : '' }).replace('  ', ' ') : c === 3 ? ev.crmErr : ev.crmRetry, 'crm', 'crm.create_deal');
    var sh = stepState(l, 'sheets.append_rows');
    if (sh) add(sh, sh === 1 ? ev.sheetOk : sh === 3 ? ev.sheetErr : ev.sheetRetry, 'sheets', 'sheets.append_rows');
    var n = stepState(l, 'notify');
    if (n) add(n, n === 1 ? ev.notifyOk : n === 3 ? ev.notifyErr : ev.notifyRetry, 'notify', 'notify');
    if (l.status === 'awaiting_payment') add(4, ev.payWait, 'payments', '');
    if (l.status === 'paid') add(1, ev.payOk, 'payments', 'crm.update_deal');
    if (l.status === 'canceled') add(1, ev.payNo, 'payments', 'crm.update_deal');
    if (l.status === 'failed') add(3, ev.failed, null, '');
    var mark = { 1: '✓', 2: '↻', 3: '!', 4: '…' };
    el.innerHTML = items.map(function (it) {
      var tag = it.svc ? (isLive(it.svc) ? '<span class="tagl real">' + esc(realName(it.svc)) + '</span>' : '<span class="tagl sim">' + esc(t('sim')) + '</span>') : '';
      return '<li data-s="' + it.s + '"><span class="mk">' + mark[it.s] + '</span><div><div class="ft">' + esc(it.text) + '</div>' +
        '<div class="fm">' + tag + (it.when ? '<span>' + esc(it.when) + '</span>' : '') + '</div></div></li>';
    }).join('');
    if (l.status === 'awaiting_payment') {
      var payLive = l.live ? p.modes.payments === 'live' : eff('payments') === 'live';
      var li = document.createElement('li'); li.className = 'feed-act';
      li.innerHTML = payLive ? '<a class="primary" href="' + esc(l.confirmation_url) + '" target="_blank" rel="noopener">' + esc(ev.payAs) + '</a>'
        : '<button class="primary" type="button" data-pay="' + esc(l.id) + '">' + esc(ev.payAs) + '</button>';
      el.appendChild(li);
      var b = li.querySelector('[data-pay]');
      if (b) b.addEventListener('click', function () { openCheckout(l); });
    }
  }

  function eff(k) { var p = profile(); return (S.state && S.state.modes && S.state.modes[k]) || (p && p.modes[k]) || 'mock'; }
  function sys(k) {
    var live = isLive(k);
    return '<p class="sysb ' + (live ? 'real' : 'sim') + '">' + esc(live ? fill(t('realBanner'), { name: realName(k) }) : t('simBanner')[k]) + '</p>';
  }
  function renderCRM() {
    var p = profile(), el = $('#tab-crm');
    if (!p) return;
    var leads = (S.state && S.state.leads) || [], cols = { New: [], Won: [], Lost: [] };
    leads.forEach(function (l) { if (l.deal_id) (cols[l.deal_stage] || cols.New).push(l); });
    var html = sys('crm') + '<div class="kanban">';
    ['New', 'Won', 'Lost'].forEach(function (k) {
      html += '<div class="kcol"><h4>' + t('stages')[k] + '<span>' + cols[k].length + '</span></h4>';
      cols[k].forEach(function (l) {
        html += '<div class="deal' + (lead() && lead().id === l.id ? ' sel' : '') + '" data-id="' + esc(l.id) + '"><b>' + esc(l.name) + '</b>' +
          '<span class="amt">' + money(l.amount) + '</span><div class="id">' + esc(dealNo(l.deal_id)) + '</div></div>';
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
    var html = sys('sheets');
    if (!rows.length) { el.innerHTML = html + '<p class="empty">' + t('noRows') + '</p>'; return; }
    var cols = Math.max.apply(null, rows.map(function (r) { return r.length; }));
    html += '<div class="sheet-wrap"><table class="sheet"><thead><tr><th class="rn"></th>';
    for (var c = 0; c < cols; c++) html += '<th>' + String.fromCharCode(65 + c) + '</th>';
    html += '</tr></thead><tbody>';
    rows.forEach(function (r, i) {
      html += '<tr class="' + (i === 0 ? 'hdr' : '') + (i === rows.length - 1 && i > 0 ? ' flash' : '') + '"><td class="rn">' + (i + 1) + '</td>';
      for (var c = 0; c < cols; c++) {
        var v = r[c] || '', cls = (v === 'Won' || v === 'Успешно') ? 'won' : (v === 'Lost' || v === 'Закрыто') ? 'lost' : '';
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
    // a simulation shows one generic chat; real channels are named
    if (!isLive('notify')) { var first = p.notify[0]; msgs = msgs.filter(function (m) { return m.channel === first; }); }
    var html = sys('notify') + '<div class="chat"><div class="chat-h"><span class="av">S</span><div><b>' + t('chatName') + '</b><br>' + t('bot') + '</div></div>';
    if (!msgs.length) html += '<p class="empty">' + t('noMsgs') + '</p>';
    msgs.forEach(function (m) {
      html += '<div class="bubble ' + esc(m.channel) + '">' + (isLive('notify') && m.channel !== 'telegram' ? '<span class="ch">' + esc(m.channel) + '</span>\n' : '') +
        esc(m.text) + '<span class="meta">' + esc((m.created_at || '').slice(11, 16)) + ' UTC</span></div>';
    });
    el.innerHTML = html + '</div>';
    el.scrollTop = el.scrollHeight;
  }
  function renderPay() {
    var p = profile(), el = $('#tab-pay');
    if (!p) return;
    var leads = ((S.state && S.state.leads) || []).filter(function (l) { return l.payment_id; });
    var html = sys('payments');
    if (!leads.length) { el.innerHTML = html + '<p class="empty">' + t('noPays') + '</p>'; return; }
    leads.forEach(function (l) {
      var ps = l.payment_status || 'pending', done = ps !== 'pending';
      var payLive = l.live ? p.modes.payments === 'live' : eff('payments') === 'live';
      html += '<div class="pay"><div class="pay-h"><b>' + esc(l.id) + ' · ' + esc(l.name) + '</b><span class="pst ' + esc(ps) + '">' + esc(t('st')[ps] || ps) + '</span></div>' +
        '<div><b>' + money(l.amount) + '</b></div>' +
        (!done ? '<div class="pay-actions">' + (payLive ? '<a class="primary" href="' + esc(l.confirmation_url) + '" target="_blank" rel="noopener">' + t('pay') + '</a>'
          : '<button class="primary" data-act="pay" data-id="' + esc(l.id) + '">' + t('pay') + '</button>') + '</div>' : '') + '</div>';
    });
    el.innerHTML = html;
    el.querySelectorAll('[data-act="pay"]').forEach(function (b) {
      b.addEventListener('click', function () {
        var l = leads.filter(function (x) { return x.id === b.dataset.id; })[0];
        S.selected = l.id; openCheckout(l);
      });
    });
  }
  // engineers' block: webhook replay / forgery for the selected paid lead
  function renderTech() {
    var p = profile(), l = lead(), el = $('#techActs');
    if (!p || !l || !l.payment_id) { el.innerHTML = ''; return; }
    var done = (l.payment_status || 'pending') !== 'pending', payLive = l.live ? p.modes.payments === 'live' : eff('payments') === 'live';
    el.innerHTML = '<span class="hint">' + esc(l.id) + ' · ' + esc(l.payment_id) + '</span> ' +
      (done && !payLive ? '<button class="ghost xs" data-act="replay-webhook" data-id="' + esc(l.id) + '">↻ ' + t('replay') + '</button> ' : '') +
      '<button class="ghost xs warnbtn" data-act="forge-webhook" data-id="' + esc(l.id) + '">⚠ ' + t('forge') + '</button>' +
      '<p class="hint">' + t('replayHint') + '</p>';
    el.querySelectorAll('[data-act]').forEach(function (b) {
      b.addEventListener('click', function () { api('/api/leads/' + b.dataset.id + '/' + b.dataset.act); });
    });
  }
  function renderModes() {
    var p = profile();
    if (!p) return;
    $('#modes').innerHTML = [['crm', 'CRM'], ['payments', 'payments'], ['sheets', 'sheets'], ['notify', 'notify']]
      .map(function (x) { var m = eff(x[0]); return '<span>' + esc(m === 'live' ? realName(x[0]) : x[1]) + ' <span class="mode ' + m + '">' + t(m) + '</span></span>'; }).join('');
  }

  // ---------- checkout dialog (simulation only; a real provider opens its own page)
  var dlg = $('#checkout'), coLead = null;
  function openCheckout(l) {
    coLead = l;
    $('#coTitle').textContent = (S.lang === 'ru' ? 'Заказ ' : 'Order ') + l.id;
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
    applyEdition(c); applyLang(); applyProfile(); syncURL(); mountWidget(); mountAuth(c); refreshMode(); refresh();
  });
  connect();
  window.leadsDemo = S; // for e2e tests
})();
