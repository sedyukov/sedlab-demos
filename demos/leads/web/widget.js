/*! leads widget — embeddable lead form (vanilla JS, no deps, Shadow DOM, ~6 KB).
 *
 * Auto-mount:
 *   <div data-leads-widget data-profile="ru" data-lang="ru"></div>
 *   <script src="https://leads.demo.sedlab.dev/widget.js" defer></script>
 *
 * Manual:
 *   const w = LeadsWidget.mount(el, { endpoint, profile: 'intl', lang: 'en', session, theme: 'auto',
 *                                     onSubmitted: ({lead_id}) => … });
 *   w.fillDemo(); w.setProfile('ru'); w.setLang('en');
 *
 * Posts JSON to `${endpoint}/api/leads`. Anti-spam built in: honeypot field, fill-time trap,
 * idempotent submission_id (double clicks / retries never create two leads). Fires `leads:submitted` on the host.
 */
(function () {
  'use strict';
  var SCRIPT = document.currentScript;
  var DEFAULT_ENDPOINT = SCRIPT ? new URL(SCRIPT.src, location.href).origin : location.origin;

  var T = {
    ru: {
      title: 'Оставьте заявку', sub: 'Ответим в течение часа', name: 'Имя', email: 'Email', phone: 'Телефон',
      company: 'Компания', message: 'Задача', plan: 'Тариф', send: 'Отправить заявку', sending: 'Отправляем…',
      ok: 'Заявка {id} принята. Смотрите, как она проходит по системам.', again: 'Ещё одна заявка',
      need: 'Укажите имя и email или телефон', err: 'Не получилось отправить', rate: 'Слишком часто, подождите несколько секунд',
      consent: 'Согласен на обработку данных (демо: данные удаляются раз в сутки)',
      plans: { start: 'Старт', business: 'Бизнес', pro: 'Про' }, demo: 'Заполнить тестовыми данными', testContact: 'тестовый', anyName: 'Любое вымышленное имя',
      sandbox: 'Демо: email и телефон не запрашиваются, подставляется тестовый контакт. Не вводите реальные данные.'
    },
    en: {
      title: 'Request a quote', sub: 'We reply within an hour', name: 'Name', email: 'Email', phone: 'Phone',
      company: 'Company', message: 'What do you need?', plan: 'Plan', send: 'Send request', sending: 'Sending…',
      ok: 'Request {id} received. Watch it flow through the systems.', again: 'Send another',
      need: 'Enter a name and an email or phone', err: 'Could not send', rate: 'Too many requests, wait a few seconds',
      consent: 'I agree to data processing (demo: data is wiped daily)',
      plans: { start: 'Starter', business: 'Business', pro: 'Pro' }, demo: 'Fill with test data', testContact: 'test', anyName: 'Any made-up name',
      sandbox: 'Demo: no email or phone is collected, a test contact is used. Please do not enter real data.'
    }
  };
  var DEMO = {
    // no emails/phones: the server makes up a sandbox contact (demo_contact) — nothing contact-like on screen
    ru: [['Анна (тест)', 'example-shop', 'Нужно, чтобы заявки сами попадали в CRM'],
         ['Иван (тест)', 'example-service', 'Заявки теряются, нужен учёт'],
         ['Ольга (тест)', 'example-school', 'Оплата курсов онлайн']],
    en: [['Emma (test)', 'example-shop', 'Connect our site form to HubSpot'],
         ['Lucas (test)', 'example-gym', 'Online payments for memberships'],
         ['Sofia (test)', 'example-studio', 'Leads into a Google Sheet + Telegram']]
  };

  function uid() {
    if (window.crypto && crypto.randomUUID) return crypto.randomUUID().replace(/-/g, '');
    var a = new Uint8Array(16); crypto.getRandomValues(a);
    return Array.prototype.map.call(a, function (b) { return ('0' + b.toString(16)).slice(-2); }).join('');
  }
  function storedSession() {
    try {
      var s = localStorage.getItem('leads.session');
      if (!s) { s = uid(); localStorage.setItem('leads.session', s); }
      return s;
    } catch (e) { return uid(); }
  }
  function money(v, cur, lang) {
    try { return new Intl.NumberFormat(lang === 'ru' ? 'ru-RU' : 'en-US', { style: 'currency', currency: cur, maximumFractionDigits: 0 }).format(v / 100); }
    catch (e) { return (v / 100) + ' ' + cur; }
  }

  var CSS = [
    ':host{all:initial;display:block;font:15px/1.45 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--lw-fg,#14161a);',
    '--lw-bg:#fff;--lw-fg:#14161a;--lw-mut:#5d6470;--lw-line:#dfe3e8;--lw-acc:#2f6fed;--lw-acc-fg:#fff;--lw-ok:#11865a;--lw-err:#c2362b;--lw-field:#f7f8fa}',
    ':host([data-theme=dark]){--lw-bg:#16191e;--lw-fg:#e8eaed;--lw-mut:#9aa1ab;--lw-line:#2b3038;--lw-field:#1d2127;--lw-acc:#5b8cff}',
    '@media (prefers-color-scheme:dark){:host([data-theme=auto]){--lw-bg:#16191e;--lw-fg:#e8eaed;--lw-mut:#9aa1ab;--lw-line:#2b3038;--lw-field:#1d2127;--lw-acc:#5b8cff}}',
    '.box{background:var(--lw-bg);color:var(--lw-fg);border:1px solid var(--lw-line);border-radius:14px;padding:20px}',
    'h3{margin:0;font-size:18px;font-weight:650;letter-spacing:-.01em}.sub{margin:2px 0 14px;color:var(--lw-mut);font-size:13px}',
    'label{display:block;font-size:12px;color:var(--lw-mut);margin:10px 0 4px}',
    'input,select,textarea{box-sizing:border-box;width:100%;font:inherit;color:inherit;background:var(--lw-field);border:1px solid var(--lw-line);border-radius:9px;padding:9px 11px;outline:none}',
    'input:focus,select:focus,textarea:focus{border-color:var(--lw-acc);box-shadow:0 0 0 3px color-mix(in srgb,var(--lw-acc) 22%,transparent)}',
    'textarea{resize:vertical;min-height:58px}.row{display:grid;grid-template-columns:1fr 1fr;gap:0 10px}',
    '@media (max-width:420px){.row{grid-template-columns:1fr}}',
    '.hp{position:absolute!important;left:-10000px!important;width:1px;height:1px;overflow:hidden}',
    '.consent{display:flex;gap:8px;align-items:flex-start;font-size:12px;color:var(--lw-mut);margin:12px 0 0}.consent input{width:auto;margin:2px 0 0}',
    'button{font:inherit;font-weight:600;cursor:pointer;border-radius:9px;border:0;padding:11px 14px}',
    '.send{width:100%;margin-top:14px;background:var(--lw-acc);color:var(--lw-acc-fg)}.send:disabled{opacity:.6;cursor:wait}',
    '.link{background:none;color:var(--lw-acc);padding:6px 0;font-weight:500;font-size:13px}',
    '.msg{margin-top:10px;font-size:13px;min-height:1em}.msg.err{color:var(--lw-err)}',
    '.note{margin:12px 0 0;font-size:12px;color:var(--lw-mut);background:var(--lw-field);border:1px dashed var(--lw-line);border-radius:9px;padding:8px 10px}',
    '.done{padding:8px 0}.done p{margin:0 0 10px;color:var(--lw-ok);font-weight:550}'
  ].join('');

  function Widget(host, opts) {
    this.host = host;
    var o = { endpoint: DEFAULT_ENDPOINT, profile: 'ru', lang: 'ru', theme: 'auto' };
    Object.keys(opts || {}).forEach(function (k) { if (opts[k] !== undefined && opts[k] !== '') o[k] = opts[k]; });
    this.o = o;
    this.o.session = this.o.session || storedSession();
    this.cfg = null;
    this.root = host.shadowRoot || host.attachShadow({ mode: 'open' });
    host.setAttribute('data-theme', this.o.theme);
    this.demoIdx = 0;
    var self = this;
    fetch(this.o.endpoint + '/api/config').then(function (r) { return r.json(); })
      .then(function (c) { self.cfg = c; self.render(); }).catch(function () { self.render(); });
    this.render();
  }
  Widget.prototype.t = function (k) { return (T[this.o.lang] || T.en)[k]; };
  Widget.prototype.profileCfg = function () {
    var p = this.o.profile;
    return this.cfg && this.cfg.profiles ? this.cfg.profiles.filter(function (x) { return x.key === p; })[0] : null;
  };
  Widget.prototype.render = function () {
    var t = this.t.bind(this), self = this, pc = this.profileCfg(), lang = this.o.lang;
    var keep = this.form && this.form.elements.name ? this.values() : null;
    var plans = (pc ? pc.plans : [{ key: 'start' }, { key: 'business' }, { key: 'pro' }]).map(function (p) {
      var price = pc ? ', ' + money(p.amount, pc.currency, lang) : '';
      return '<option value="' + p.key + '"' + (p.key === 'business' ? ' selected' : '') + '>' + t('plans')[p.key] + price + '</option>';
    }).join('');
    var sandbox = !!(this.cfg && this.cfg.sandbox_contacts);
    this.sandbox = sandbox;
    var contacts = sandbox
      ? '<input type="hidden" name="email"><input type="hidden" name="phone">'
      : '<div class="row"><div><label for="e">' + t('email') + '</label><input id="e" name="email" type="email" autocomplete="email" maxlength="120"></div>' +
        '<div><label for="p">' + t('phone') + '</label><input id="p" name="phone" type="tel" autocomplete="tel" maxlength="32"></div></div>';
    this.root.innerHTML = '<style>' + CSS + '</style><form class="box" novalidate>' +
      '<h3>' + t('title') + '</h3><p class="sub">' + t('sub') + '</p>' +
      '<label for="n">' + t('name') + '</label><input id="n" name="name" autocomplete="off" maxlength="80" required' +
        (sandbox ? ' placeholder="' + t('anyName') + '"' : '') + '>' +
      contacts +
      '<div class="row"><div><label for="c">' + t('company') + '</label><input id="c" name="company" autocomplete="organization" maxlength="80"></div>' +
      '<div><label for="pl">' + t('plan') + '</label><select id="pl" name="plan">' + plans + '</select></div></div>' +
      '<label for="m">' + t('message') + '</label><textarea id="m" name="message" maxlength="500"></textarea>' +
      '<div class="hp" aria-hidden="true"><label>Website<input name="website" tabindex="-1" autocomplete="off"></label></div>' +
      (sandbox ? '<p class="note">' + t('sandbox') + '</p><input type="hidden" name="consent" value="">'
               : '<label class="consent"><input type="checkbox" name="consent" checked> <span>' + t('consent') + '</span></label>') +
      '<button class="send" type="submit">' + t('send') + '</button>' +
      '<button class="link" type="button" data-demo>' + t('demo') + '</button>' +
      '<div class="msg" role="status" aria-live="polite"></div></form>';
    this.form = this.root.querySelector('form');
    this.rendered = Date.now();
    this.subId = uid();
    if (keep) this.fill(keep);
    if (this.demoContact) this.markDemoContact();
    this.form.addEventListener('submit', function (e) { e.preventDefault(); self.submit(); });
    this.form.addEventListener('input', function (e) {
      self.subId = self.subId || uid();
      if (e.target.name === 'email' || e.target.name === 'phone') self.demoContact = false;
    });
    this.root.querySelector('[data-demo]').addEventListener('click', function () { self.fillDemo(); });
  };
  Widget.prototype.values = function () {
    var f = this.form, v = {};
    ['name', 'email', 'phone', 'company', 'message', 'plan', 'website'].forEach(function (k) { v[k] = f.elements[k].value.trim(); });
    v.consent = !!f.elements.consent.checked;
    return v;
  };
  Widget.prototype.fill = function (v) {
    var f = this.form;
    Object.keys(v).forEach(function (k) { if (f.elements[k] && k !== 'consent' && k !== 'website') f.elements[k].value = v[k]; });
  };
  Widget.prototype.fillDemo = function () {
    var list = DEMO[this.o.lang] || DEMO.en, d = list[this.demoIdx++ % list.length];
    this.fill({ name: d[0], email: '', phone: '', company: d[1], message: d[2] });
    this.demoContact = true;
    this.markDemoContact();
    this.rendered = Math.min(this.rendered, Date.now() - 3000); // a human did "type" it
  };
  Widget.prototype.markDemoContact = function () {
    var f = this.form, t = this.t('testContact');
    if (this.sandbox) return;
    f.elements.email.placeholder = t; f.elements.phone.placeholder = t;
  };
  Widget.prototype.msg = function (text, err) {
    var m = this.root.querySelector('.msg'); m.textContent = text || ''; m.className = 'msg' + (err ? ' err' : '');
  };
  Widget.prototype.submit = function () {
    var v = this.values(), self = this, btn = this.root.querySelector('.send');
    if (!v.name || (!v.email && !v.phone && !this.demoContact && !this.sandbox)) { this.msg(this.t('need'), true); return; }
    btn.disabled = true; btn.textContent = this.t('sending'); this.msg('');
    var body = Object.assign(v, { session: this.o.session, profile: this.o.profile, lang: this.o.lang,
      submission_id: this.subId, elapsed_ms: Date.now() - this.rendered, demo_contact: !!this.demoContact && !v.email && !v.phone });
    var attempt = 0;
    (function go() {
      attempt++;
      fetch(self.o.endpoint + '/api/leads', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
        .then(function (r) { return r.json().then(function (j) { return { s: r.status, j: j }; }); })
        .then(function (res) {
          if (res.s === 202) return self.done(res.j);
          btn.disabled = false; btn.textContent = self.t('send');
          self.msg(res.s === 429 ? self.t('rate') : self.t('err') + ': ' + ((res.j.error && res.j.error.message) || res.s), true);
        })
        .catch(function () {
          // network blip: retry with the SAME submission_id — the server dedupes, no double lead
          if (attempt < 3) return setTimeout(go, 600 * attempt);
          btn.disabled = false; btn.textContent = self.t('send'); self.msg(self.t('err'), true);
        });
    })();
  };
  Widget.prototype.done = function (j) {
    var self = this;
    this.subId = null;
    this.demoContact = false;
    this.form.innerHTML = '<div class="done"><h3>✓</h3><p>' + this.t('ok').replace('{id}', j.lead_id) + '</p>' +
      '<button class="link" type="button">' + this.t('again') + '</button></div>';
    this.form.querySelector('button').addEventListener('click', function () { self.render(); });
    this.host.dispatchEvent(new CustomEvent('leads:submitted', { detail: j, bubbles: true }));
    if (typeof this.o.onSubmitted === 'function') this.o.onSubmitted(j);
  };
  Widget.prototype.setProfile = function (p) { this.o.profile = p; this.render(); };
  Widget.prototype.setLang = function (l) { this.o.lang = l; this.render(); };

  var api = {
    mount: function (el, opts) { if (typeof el === 'string') el = document.querySelector(el); return (el.__leads = new Widget(el, opts)); },
    version: '1.0.0'
  };
  window.LeadsWidget = api;
  function auto() {
    Array.prototype.forEach.call(document.querySelectorAll('[data-leads-widget]'), function (el) {
      if (el.__leads) return;
      var d = el.dataset;
      api.mount(el, { profile: d.profile, lang: d.lang, endpoint: d.endpoint, session: d.session, theme: d.theme });
    });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', auto); else auto();
})();
