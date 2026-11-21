(function () {
  'use strict';

  var I18N = {
    ru: {
      title: 'Мониторинг цен WB и Ozon',
      tagline: 'Цены и остатки конкурентов, графики, рекомендации, отчёт в мессенджер и таблицу, ответы на отзывы',
      asOf: 'Данные на', refresh: 'Обновить сейчас', reset: 'Сбросить',
      schedule: 'Сбор по расписанию ежедневно в {t}. В демо каждое нажатие «Обновить» = следующий день.',
      tabPrices: 'Цены', tabReport: 'Отчёт', tabSheet: 'Таблица', tabReviews: 'Отзывы', tabLog: 'Журнал сбора',
      kProducts: 'товаров на мониторинге', kComps: 'карточек конкурентов', kChanges: 'изменений цен за сутки',
      kExpensive: 'где вы дороже рынка', kReviews: 'отзывов без ответа',
      yours: 'Ваша цена', cheapest: 'Мин. у конкурентов', diff: 'Разница',
      art: 'арт.', stock: 'остаток', inStock: 'в наличии', oos: 'нет в наличии', moreThan: 'больше {n}',
      recLower: 'Вы дороже самого дешёвого конкурента ({who}). Рекомендуем {p} (на 1% ниже, но не ниже минимальной цены).',
      recRaise: 'Вы дешевле всех больше чем на 10%. Можно поднять до {p} и остаться самым дешёвым.',
      recOk: 'Цена в рынке: рядом с самым дешёвым конкурентом ({who}).',
      recNone: 'Все конкуренты сейчас без наличия.',
      recFloor: 'Самый дешёвый конкурент ({who}) ниже вашей минимальной цены. Ниже не опускаем: ждём, когда акция закончится.',
      delivered: 'доставлено',
      apply: 'Применить {p}', applied: 'Цена {p} отправлена на маркетплейс',
      chartPrice: 'Цена за 30 дней, ₽', chartStock: 'Остатки конкурентов, шт', you: 'Вы',
      colComp: 'Конкурент', colPrice: 'Цена', colDelta: 'За сутки', colStock: 'Остаток', colRating: 'Рейтинг',
      fetchFailed: 'не ответил, данные за вчера',
      addPh: 'Ссылка на товар WB или Ozon, артикул WB, ozon:SKU', add: 'Добавить конкурента',
      addHint: 'В демо данные добавленной карточки смоделированы по артикулу. В рабочей версии модуль берёт их с публичной страницы товара.',
      addBad: 'Не похоже на ссылку на товар Wildberries или Ozon. Короткие ссылки ozon.ru/t/… откройте и скопируйте полный адрес.',
      addDup: 'Эта карточка уже отслеживается.', addMany: 'В демо можно добавить до {n} карточек.', added: 'Карточка добавлена',
      remove: 'Убрать',
      reportTitle: 'Ежедневный отчёт',
      reportHint: 'Уходит каждое утро после сбора. В демо отправка идёт в тестовый режим: сообщение никуда не уходит, видно, что получил бы менеджер.',
      send: 'Отправить в {c}', sentTitle: 'Отправленные', noSent: 'Пока ничего не отправляли.', sent: 'Отчёт отправлен в {c} (тестовый режим)',
      sheetTitle: 'Таблица: {s}', sheetHint: 'Каждый сбор дописывает строки: дата, товар, площадка, конкурент, цена, наличие, ваша цена, разница. Всего строк: {n}, ниже последние 20.',
      csv: 'Скачать CSV', xlsx: 'Скачать XLSX',
      reviewsHint: 'Отзывы покупателей из кабинета продавца. Черновик ответа пишет нейросеть ({llm}), вы правите и публикуете. В демо модель работает в тестовом режиме.',
      draft: 'Черновик ответа', redraft: 'Ещё вариант', publish: 'Опубликовать ответ', published: 'Ответ опубликован',
      needLogin: 'Войдите, чтобы черновик писала настоящая нейросеть. Сейчас показан тестовый вариант.',
      liveLimit: 'Живой лимит на сегодня исчерпан: показываем демо-режим.',
      yourAnswer: 'Ответ магазина', draftBy: 'Черновик: {p}', testMode: 'тестовый режим',
      logTitle: 'Последний сбор',
      logSummary: 'Собрано {ok} из {all} карточек за {s} с · повторов после ошибок: {r} · не ответили: {f} · лимит {rate} запросов в секунду на сайт, robots.txt соблюдается.',
      logNone: 'Нажмите «Обновить сейчас», чтобы запустить сбор и увидеть каждый запрос.',
      colMarket: 'Площадка', colUrl: 'Запрос', colStatus: 'Ответ', colTry: 'Попыток', colMs: 'мс', colResult: 'Результат',
      robots: 'правила robots.txt', kept: 'оставили вчерашние данные',
      done: 'Сбор за {d}: {n} карточек, изменений цен: {c}',
      limit: 'Лимит демо: {n} сборов. Нажмите «Сбросить».', slow: 'Слишком часто, подождите пару секунд.', err: 'Что-то пошло не так, попробуйте ещё раз.',
      resetDone: 'Песочница сброшена',
      foot: 'Демо sedlab.dev. Данные товаров и конкурентов вымышленные, песочница своя у каждого посетителя и очищается раз в сутки.',
      logNote: 'В публичном демо страницы витрин смоделированы и отдаются модулю парсинга через тестовый транспорт: тот же код запросов, robots.txt, лимитов, повторов и разбора, но без обращений к wildberries.ru и ozon.ru.',
      chanNames: { max: 'Max', telegram: 'Telegram', email: 'Email' },
      other: 'English version'
    },
    en: {
      title: 'WB & Ozon price monitoring',
      tagline: 'Competitor prices and stock, charts, price suggestions, a daily report to a messenger and a sheet, replies to reviews',
      asOf: 'Data as of', refresh: 'Update now', reset: 'Reset',
      schedule: 'Scheduled collection every day at {t}. In the demo every "Update" click is the next day.',
      tabPrices: 'Prices', tabReport: 'Report', tabSheet: 'Sheet', tabReviews: 'Reviews', tabLog: 'Collection log',
      kProducts: 'products monitored', kComps: 'competitor cards', kChanges: 'price changes in 24 h',
      kExpensive: 'above the market', kReviews: 'reviews without a reply',
      yours: 'Your price', cheapest: 'Cheapest rival', diff: 'Difference',
      art: 'art.', stock: 'stock', inStock: 'in stock', oos: 'out of stock', moreThan: 'over {n}',
      recLower: 'You are above the cheapest rival ({who}). Suggested: {p} (1% lower, never below your floor price).',
      recRaise: 'You are more than 10% below everyone. You can raise to {p} and stay the cheapest.',
      recOk: 'In line with the market: close to the cheapest rival ({who}).',
      recNone: 'All rivals are out of stock right now.',
      recFloor: 'The cheapest rival ({who}) is below your floor price. We do not go lower: waiting for the promo to end.',
      delivered: 'delivered',
      apply: 'Apply {p}', applied: 'Price {p} sent to the marketplace',
      chartPrice: 'Price over 30 days, RUB', chartStock: 'Rivals’ stock, pcs', you: 'You',
      colComp: 'Competitor', colPrice: 'Price', colDelta: '24 h', colStock: 'Stock', colRating: 'Rating',
      fetchFailed: 'no response, yesterday’s data',
      addPh: 'Wildberries or Ozon product link, WB article, ozon:SKU', add: 'Add competitor',
      addHint: 'In the demo the added card is modelled from its article. In production the module reads the public product page.',
      addBad: 'This does not look like a Wildberries or Ozon product link. Open short ozon.ru/t/… links and copy the full URL.',
      addDup: 'This card is already tracked.', addMany: 'The demo allows up to {n} added cards.', added: 'Card added',
      remove: 'Remove',
      reportTitle: 'Daily report',
      reportHint: 'Sent every morning after collection. In the demo delivery is in test mode: nothing leaves the server, you see what the manager would get.',
      send: 'Send to {c}', sentTitle: 'Sent', noSent: 'Nothing sent yet.', sent: 'Report sent to {c} (test mode)',
      sheetTitle: 'Sheet: {s}', sheetHint: 'Every collection appends rows: date, product, marketplace, rival, price, availability, your price, difference. Rows: {n}, the last 20 below.',
      csv: 'Download CSV', xlsx: 'Download XLSX',
      reviewsHint: 'Buyer reviews from the seller account. An AI model ({llm}) drafts the reply, you edit and publish it. In the demo the model runs in test mode.',
      draft: 'Draft a reply', redraft: 'Another draft', publish: 'Publish reply', published: 'Reply published',
      needLogin: 'Sign in to get a draft from the real model. A test draft is shown.',
      liveLimit: 'Today\u2019s live limit is used up: showing the demo mode.',
      yourAnswer: 'Store reply', draftBy: 'Draft: {p}', testMode: 'test mode',
      logTitle: 'Last collection',
      logSummary: 'Collected {ok} of {all} cards in {s} s · retries after errors: {r} · no response: {f} · limit {rate} requests per second per site, robots.txt respected.',
      logNone: 'Click "Update now" to run a collection and see every request.',
      colMarket: 'Marketplace', colUrl: 'Request', colStatus: 'Status', colTry: 'Attempts', colMs: 'ms', colResult: 'Result',
      robots: 'robots.txt rules', kept: 'kept yesterday’s data',
      done: 'Collected {d}: {n} cards, price changes: {c}',
      limit: 'Demo limit: {n} collections. Click "Reset".', slow: 'Too fast, wait a couple of seconds.', err: 'Something went wrong, please try again.',
      resetDone: 'Sandbox reset',
      foot: 'A sedlab.dev demo. Products and competitors are fictitious; every visitor gets an own sandbox, wiped daily.',
      logNote: 'In the public demo storefront pages are modelled and served to the scraping module through its test transport: the same request, robots.txt, rate limit, retry and parsing code, without requests to wildberries.ru or ozon.ru.',
      chanNames: { max: 'Max', telegram: 'Telegram', email: 'Email' },
      other: 'Русская версия'
    }
  };
  var PALETTE = ['#e0457b', '#e08a00', '#11865a', '#0e8fa8', '#8a5cf5', '#b5651d', '#5d6470', '#c2362b', '#2b8a3e', '#d6336c', '#1c7ed6', '#7048e8'];

  var S = { lang: 'ru', cfg: {}, st: null, tab: 'prices', open: {}, busy: false, drafts: {} };
  var $ = function (id) { return document.getElementById(id); };
  function t(k, vars) {
    var s = (I18N[S.lang] || I18N.ru)[k];
    if (s == null) s = I18N.ru[k] || k;
    if (vars) Object.keys(vars).forEach(function (v) { s = s.split('{' + v + '}').join(vars[v]); });
    return s;
  }
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function rub(k, sign) {
    var v = Math.round(Math.abs(k) / 100), s = String(v), out = '';
    while (s.length > 3) { out = (S.lang === 'en' ? ',' : ' ') + s.slice(-3) + out; s = s.slice(0, -3); }
    out = s + out;
    var pre = k < 0 ? '−' : (sign && k > 0 ? '+' : '');
    return pre + out + ' ₽';
  }
  function pct(v) { var s = (Math.abs(v) < 10 ? v.toFixed(1) : Math.round(v).toString()).replace('.', S.lang === 'en' ? '.' : ','); return (v > 0 ? '+' : v < 0 ? '−' : '') + s.replace('-', '') + '%'; }
  function fmtDate(iso, short) {
    var d = new Date(iso + 'T00:00:00Z');
    var loc = S.lang === 'en' ? 'en-US' : 'ru-RU';
    var o = short ? { day: 'numeric', month: 'short', timeZone: 'UTC' } : { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' };
    return d.toLocaleDateString(loc, o).replace(' г.', '').replace('.', '');
  }
  function plural(n, one, few, many) {
    var a = n % 10, b = n % 100;
    return a === 1 && b !== 11 ? one : a >= 2 && a <= 4 && (b < 12 || b > 14) ? few : many;
  }
  function mk(m) { return '<span class="mk ' + m + '">' + (m === 'wb' ? 'WB' : 'OZON') + '</span>'; }
  function toast(msg) {
    var el = $('toast'); el.textContent = msg; el.hidden = false;
    clearTimeout(toast.t); toast.t = setTimeout(function () { el.hidden = true; }, 3200);
  }
  function api(method, path, body) {
    var q = S.cfg.edition ? '' : (path.indexOf('?') < 0 ? '?' : '&') + 'lang=' + S.lang;
    return fetch(path + q, { method: method, headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined, credentials: 'same-origin' })
      .then(function (r) { return r.json().catch(function () { return {}; }).then(function (j) { j.__status = r.status; return j; }); });
  }

  // ---------- static texts ----------
  function applyI18n() {
    document.documentElement.lang = S.lang;
    document.querySelectorAll('[data-i18n]').forEach(function (el) { el.textContent = t(el.getAttribute('data-i18n')); });
    document.title = t('title') + (S.lang === 'en' ? ' · demo' : ' · демо');
    $('schedule').textContent = t('schedule', { t: S.cfg.schedule || '09:00' });
    $('addHintTpl') && ($('addHintTpl').textContent = t('addHint'));
    document.querySelectorAll('#langSeg button').forEach(function (b) { b.setAttribute('aria-checked', String(b.dataset.lang === S.lang)); });
  }

  // ---------- charts (inline SVG, no libraries) ----------
  function chart(el, dates, series, opts) {
    var W = 560, H = opts.h || 200, L = 46, R = 8, T = 8, B = 22;
    var vals = [];
    series.forEach(function (s) { s.values.forEach(function (v) { if (v != null) vals.push(v); }); });
    if (!vals.length) { el.innerHTML = ''; return; }
    var lo = Math.min.apply(null, vals), hi = Math.max.apply(null, vals);
    if (opts.zero) lo = 0;
    if (hi === lo) { hi += Math.max(1, hi * 0.05); lo = opts.zero ? 0 : lo - Math.max(1, lo * 0.05); }
    var raw = (hi - lo) / 4, mag = Math.pow(10, Math.floor(Math.log10(raw))), f = raw / mag;
    var step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * mag;
    lo = Math.floor(lo / step) * step; hi = Math.ceil(hi / step) * step;
    var ticks = Math.round((hi - lo) / step);
    var n = dates.length, x = function (i) { return L + (W - L - R) * (n === 1 ? 0.5 : i / (n - 1)); };
    var y = function (v) { return T + (H - T - B) * (1 - (v - lo) / (hi - lo)); };
    var svg = '<svg viewBox="0 0 ' + W + ' ' + H + '" role="img" aria-label="' + esc(opts.label) + '"><g class="grid">';
    var i;
    for (i = 0; i <= ticks; i++) {
      var v = lo + (hi - lo) * i / ticks;
      svg += '<line x1="' + L + '" x2="' + (W - R) + '" y1="' + y(v).toFixed(1) + '" y2="' + y(v).toFixed(1) + '"/>';
    }
    svg += '</g><g class="axis">';
    for (i = 0; i <= ticks; i++) {
      var tv = lo + (hi - lo) * i / ticks;
      svg += '<text x="' + (L - 6) + '" y="' + (y(tv) + 3).toFixed(1) + '" text-anchor="end">' + esc(opts.fmt(tv)) + '</text>';
    }
    for (i = 0; i < n; i += 7) svg += '<text x="' + x(i).toFixed(1) + '" y="' + (H - 6) + '" text-anchor="middle">' + esc(fmtDate(dates[i], true)) + '</text>';
    svg += '</g>';
    series.forEach(function (s) {
      var d = '', pen = false;
      s.values.forEach(function (v, j) {
        if (v == null) { pen = false; return; }
        var px = x(j).toFixed(1), py = y(v).toFixed(1);
        if (s.step && pen) d += 'H' + px + 'V' + py; else d += (pen ? 'L' : 'M') + px + ' ' + py;
        pen = true;
      });
      svg += '<path d="' + d + '" fill="none" style="stroke:' + s.color + '" stroke-width="' + (s.width || 1.6) + '"' + (s.dash ? ' stroke-dasharray="5 3"' : '') + ' stroke-linejoin="round" stroke-linecap="round"/>';
    });
    svg += '<line class="cursor" y1="' + T + '" y2="' + (H - B) + '" x1="0" x2="0" visibility="hidden"/></svg><div class="tip" hidden></div>';
    el.innerHTML = svg;
    var sv = el.querySelector('svg'), cur = el.querySelector('.cursor'), tip = el.querySelector('.tip');
    function move(ev) {
      var rect = sv.getBoundingClientRect(), cx = (ev.touches ? ev.touches[0].clientX : ev.clientX) - rect.left;
      var sx = cx * W / rect.width, j = Math.round((sx - L) / (W - L - R) * (n - 1));
      if (j < 0 || j >= n) { tip.hidden = true; return; }
      cur.setAttribute('x1', x(j)); cur.setAttribute('x2', x(j)); cur.setAttribute('visibility', 'visible');
      var rows = series.filter(function (s) { return s.values[j] != null; }).map(function (s) {
        return '<div><i style="background:' + s.color + '"></i>' + esc(s.name) + ': <b>' + esc(opts.fmt(s.values[j], true)) + '</b></div>';
      });
      tip.innerHTML = '<div class="hint" style="margin:0 0 4px">' + esc(fmtDate(dates[j])) + '</div>' + rows.join('');
      tip.hidden = false;
      var left = x(j) * rect.width / W + 12;
      if (left + tip.offsetWidth > rect.width) left = x(j) * rect.width / W - tip.offsetWidth - 12;
      tip.style.left = Math.max(0, left) + 'px'; tip.style.top = '6px';
    }
    sv.addEventListener('mousemove', move);
    sv.addEventListener('touchstart', move, { passive: true });
    sv.addEventListener('mouseleave', function () { tip.hidden = true; cur.setAttribute('visibility', 'hidden'); });
  }

  // ---------- render ----------
  function render() {
    var st = S.st; if (!st) return;
    $('asOf').textContent = fmtDate(st.date);
    var kp = [
      [st.products.length, t('kProducts'), ''],
      [st.competitors, t('kComps'), ''],
      [st.changes, t('kChanges'), ''],
      [st.expensive, t('kExpensive'), st.expensive ? 'alert' : 'good'],
      [st.unanswered, t('kReviews'), st.unanswered ? 'alert' : 'good']
    ];
    $('kpis').innerHTML = kp.map(function (k) { return '<div class="kpi ' + k[2] + '"><b>' + k[0] + '</b><span>' + esc(k[1]) + '</span></div>'; }).join('');
    $('revCount').textContent = st.unanswered || '';
    renderProducts(); renderReport(); renderSheet(); renderReviews(); renderLog();
  }

  function stockText(c) {
    if (!c.in_stock) return '<span class="oos">' + esc(t('oos')) + '</span>';
    if (c.stock == null) return esc(t('moreThan', { n: 50 }));
    return c.stock + ' ' + (S.lang === 'en' ? 'pcs' : 'шт');
  }

  function renderProducts() {
    var st = S.st, html = '';
    st.products.forEach(function (p, pi) {
      var open = S.open[p.id] !== undefined ? S.open[p.id] : pi === 0;
      var diffCls = p.diff_pct > 0.05 ? 'up' : p.diff_pct < -0.05 ? 'down' : 'same';
      var cheapest = null;
      p.comps.forEach(function (c) { if (c.key === p.rec.cheapest) cheapest = c; });
      var who = cheapest ? cheapest.seller + ', ' + (cheapest.market === 'wb' ? 'WB' : 'Ozon') + ', ' + rub(cheapest.price) : '';
      var recTxt = p.rec.kind === 'lower' ? t('recLower', { who: who, p: rub(p.rec.price) })
        : p.rec.kind === 'raise' ? t('recRaise', { p: rub(p.rec.price) })
        : p.rec.kind === 'ok' ? t('recOk', { who: who }) : p.rec.kind === 'floor' ? t('recFloor', { who: who }) : t('recNone');
      html += '<article class="card prod' + (open ? ' open' : '') + '" data-pid="' + p.id + '">' +
        '<div class="prod-h" data-toggle="' + p.id + '"><div><div class="prod-name">' + esc(p.name) + '</div>' +
        '<div class="prod-sub">' + esc(p.offer_id) + ' · ' + p.comps.length + ' ' + esc(S.lang === 'en' ? 'rivals on WB and Ozon' : plural(p.comps.length, 'конкурент', 'конкурента', 'конкурентов') + ' на WB и Ozon') + '</div></div>' +
        '<div class="prod-nums"><div><span>' + esc(t('yours')) + '</span><b>' + rub(p.my_price) + '</b></div>' +
        '<div><span>' + esc(t('cheapest')) + '</span><b>' + (p.min_price ? rub(p.min_price) : '–') + '</b></div>' +
        '<div><span>' + esc(t('diff')) + '</span><b><span class="pill ' + diffCls + '">' + (p.min_price ? pct(p.diff_pct) : '–') + '</span></b></div>' +
        '<span class="chev" aria-hidden="true">›</span></div></div>' +
        '<div class="rec ' + p.rec.kind + '"><p>' + esc(recTxt) + '</p>' +
        ((p.rec.kind === 'lower' || p.rec.kind === 'raise') ? '<button class="primary sm" data-apply="' + p.id + '">' + esc(t('apply', { p: rub(p.rec.price) })) + '</button>' : '') + '</div>';
      if (open) {
        html += '<div class="prod-body"><div class="charts">' +
          '<div class="chart-t"><span>' + esc(t('chartPrice')) + '</span></div><div class="chart" id="cp-' + p.id + '"></div>' +
          '<div class="legend" id="lg-' + p.id + '"></div>' +
          '<div class="chart-t"><span>' + esc(t('chartStock')) + '</span></div><div class="chart" id="cs-' + p.id + '"></div></div>' +
          '<div><table class="comps"><thead><tr><th>' + esc(t('colComp')) + '</th><th class="r">' + esc(t('colPrice')) + '</th><th class="r">' + esc(t('colDelta')) +
          '</th><th class="r">' + esc(t('colStock')) + '</th><th class="r hide-sm">' + esc(t('colRating')) + '</th></tr></thead><tbody>' +
          '<tr class="me"><td><span class="nm">' + esc(t('you')) + '</span></td><td class="r price"><b>' + rub(p.my_price) + '</b></td><td></td><td></td><td class="hide-sm"></td></tr>';
        p.comps.slice().sort(function (a, b) { return (b.in_stock - a.in_stock) || a.price - b.price; }).forEach(function (c) {
          var d = c.price - c.prev_price, dp = c.prev_price ? d * 100 / c.prev_price : 0;
          var changed = d !== 0 && S.flash;
          html += '<tr class="' + (c.key === p.rec.cheapest ? 'cheapest' : '') + (changed ? ' flash' : '') + '"><td>' + mk(c.market) + ' <span class="nm">' + esc(c.name) + '</span>' +
            '<div class="sl">' + esc(c.seller) + ' · ' + esc(t('art')) + ' ' + esc(c.id) + (c.failed ? ' · <span class="failed">' + esc(t('fetchFailed')) + '</span>' : '') +
            (c.added ? ' <button class="x" title="' + esc(t('remove')) + '" data-remove="' + esc(c.key) + '">×</button>' : '') + '</div></td>' +
            '<td class="r price"><b>' + rub(c.price) + '</b></td>' +
            '<td class="r">' + (d ? '<span class="pill ' + (d < 0 ? 'down' : 'up') + '">' + pct(dp) + '</span>' : '<span class="hint">0</span>') + '</td>' +
            '<td class="r">' + stockText(c) + '</td>' +
            '<td class="r hide-sm">' + (c.rating ? '★ ' + c.rating.toFixed(1) + '<div class="sl">' + c.reviews + '</div>' : '–') + '</td></tr>';
        });
        html += '</tbody></table>' +
          '<form class="add" data-add="' + p.id + '"><input name="ref" autocomplete="off" placeholder="' + esc(t('addPh')) + '" aria-label="' + esc(t('addPh')) + '">' +
          '<button class="ghost sm" type="submit">' + esc(t('add')) + '</button></form><p class="add-err" id="ae-' + p.id + '"></p>' +
          '<p class="hint">' + esc(t('addHint')) + '</p></div></div>';
      }
      html += '</article>';
    });
    $('products').innerHTML = html;
    st.products.forEach(function (p, pi) {
      var open = S.open[p.id] !== undefined ? S.open[p.id] : pi === 0;
      if (!open) return;
      var series = [{ name: t('you'), color: 'var(--acc)', values: p.my_prices, width: 3, step: true }];
      var stocks = [];
      p.comps.forEach(function (c, i) {
        var col = PALETTE[i % PALETTE.length];
        series.push({ name: c.seller + ' (' + (c.market === 'wb' ? 'WB' : 'Ozon') + ')', color: col, values: c.prices, dash: c.market === 'ozon', step: true });
        stocks.push({ name: c.seller + ' (' + (c.market === 'wb' ? 'WB' : 'Ozon') + ')', color: col, values: c.stocks, dash: c.market === 'ozon' });
      });
      chart($('cp-' + p.id), st.dates, series, { label: t('chartPrice'), fmt: function (v, full) { return full ? rub(v) : Math.round(v / 100).toLocaleString(S.lang === 'en' ? 'en-US' : 'ru-RU'); } });
      chart($('cs-' + p.id), st.dates, stocks, { h: 130, zero: true, label: t('chartStock'), fmt: function (v) { return Math.round(v).toString(); } });
      $('lg-' + p.id).innerHTML = series.map(function (s) {
        return '<span style="color:' + s.color + '"><i class="' + (s.dash ? 'dash' : '') + '" style="background:' + s.color + '"></i><span style="color:var(--fg)">' + esc(s.name) + '</span></span>';
      }).join('');
    });
  }

  function renderReport() {
    var st = S.st;
    $('reportText').textContent = st.report.text;
    $('sendBtns').innerHTML = (S.cfg.channels || []).map(function (c) {
      return '<button class="primary sm" data-send="' + c + '">' + esc(t('send', { c: t('chanNames')[c] })) + '</button>';
    }).join('');
    var ms = (st.messages || []).slice().reverse();
    $('messages').innerHTML = ms.length ? ms.map(function (m) {
      return '<div class="msg"><div class="msg-h"><b>' + esc(t('chanNames')[m.channel]) + '</b> → ' + esc(m.to) +
        ' <span class="badge mock">' + esc(t('testMode')) + '</span> <span class="badge ok">' + esc(m.status === 'sent' ? t('delivered') : m.status) + '</span> <span>' + esc(m.id) + '</span></div><pre>' + esc(m.text) + '</pre></div>';
    }).join('') : '<div class="empty">' + esc(t('noSent')) + '</div>';
  }

  function renderSheet() {
    var sh = S.st.sheet;
    $('sheetTitle').textContent = t('sheetTitle', { s: S.cfg.sheets || 'Google Sheets' });
    $('sheetHint').textContent = t('sheetHint', { n: sh.total });
    var rows = sh.rows || [];
    if (!rows.length) { $('sheet').innerHTML = ''; return; }
    var html = '<thead><tr><th></th>' + rows[0].map(function (h) { return '<th>' + esc(h) + '</th>'; }).join('') + '</tr></thead><tbody>';
    var first = sh.total - (rows.length - 1) + 2;
    rows.slice(1).forEach(function (r, i) {
      html += '<tr><td class="rownum">' + (first + i) + '</td>' + r.map(function (v) { return '<td>' + esc(v) + '</td>'; }).join('') + '</tr>';
    });
    $('sheet').innerHTML = html + '</tbody>';
  }

  function stars(n) { var s = ''; for (var i = 1; i <= 5; i++) s += i <= n ? '★' : '<span class="off">★</span>'; return '<span class="stars" aria-label="' + n + '/5">' + s + '</span>'; }

  function renderReviews() {
    $('reviewsHint').textContent = t('reviewsHint', { llm: S.cfg.llm || 'AI' });
    $('reviews').innerHTML = S.st.reviews.map(function (r) {
      var d = r.draft;
      var body = '';
      if (r.answered) {
        body = '<div class="answer"><small>' + esc(t('yourAnswer')) + '</small>' + esc(r.answer_text) + '</div>';
      } else if (d) {
        body = '<div class="draft"><div class="draft-h"><span>' + esc(t('draftBy', { p: (S.cfg.llm || 'AI') + (d.provider === 'mock' ? ' · ' + t('testMode') : '') })) + '</span></div>' +
          '<textarea id="dt-' + r.id + '" maxlength="1000">' + esc(S.drafts[r.id] != null ? S.drafts[r.id] : d.text) + '</textarea>' +
          '<div class="rev-actions"><button class="primary sm" data-publish="' + r.id + '">' + esc(t('publish')) + '</button>' +
          '<button class="ghost sm" data-draft="' + r.id + '">' + esc(t('redraft')) + '</button></div></div>';
      } else {
        body = '<div class="rev-actions"><button class="ghost sm" data-draft="' + r.id + '">✨ ' + esc(t('draft')) + '</button></div>';
      }
      return '<article class="card rev"><div class="rev-h"><div>' + stars(r.rating) + ' <b>' + esc(r.author) + '</b> <span class="hint">' + esc(fmtDate(r.created_at.slice(0, 10))) + '</span></div>' +
        '<span class="hint">' + esc(r.product_name) + '</span></div><p>' + esc(r.text) + '</p>' + body + '</article>';
    }).join('');
  }

  function renderLog() {
    var run = S.st.last_run;
    if (!run) { $('logSummary').textContent = t('logNone'); $('log').innerHTML = ''; return; }
    var all = run.fetched + run.failed;
    $('logSummary').textContent = t('logSummary', { ok: run.fetched, all: all, s: (run.took_ms / 1000).toFixed(1).replace('.', S.lang === 'en' ? '.' : ','), r: run.retries, f: run.failed, rate: run.host_rate });
    var html = '<thead><tr><th>' + esc(t('colMarket')) + '</th><th>' + esc(t('colUrl')) + '</th><th>' + esc(t('colStatus')) + '</th><th>' + esc(t('colTry')) +
      '</th><th>' + esc(t('colMs')) + '</th><th>' + esc(t('colResult')) + '</th></tr></thead><tbody>';
    run.log.forEach(function (l) {
      var cls = l.error ? 'err' : l.attempts > 1 ? 'retry' : 'ok';
      var status = l.error ? (l.status || '–') + ' ' + l.error : (l.attempts > 1 ? '↻ ' : '') + l.status;
      var res = l.robots ? t('robots') : l.error ? t('kept') : rub(l.price) + ' · ' + (l.in_stock ? (l.stock != null ? l.stock + (S.lang === 'en' ? ' pcs' : ' шт') : t('inStock')) : t('oos'));
      html += '<tr><td>' + mk(l.market) + '</td><td class="url" title="' + esc(l.url) + '">' + esc(l.url.replace(/^https:\/\//, '')) + '</td><td class="st ' + cls + '">' + esc(status) +
        '</td><td>' + (l.attempts || 1) + '</td><td>' + (l.robots ? '' : l.ms) + '</td><td>' + esc(res) + '</td></tr>';
    });
    $('log').innerHTML = html + '</tbody>';
  }

  // ---------- actions ----------
  function setState(j) { if (j && j.products) { S.st = j; render(); } }
  function handleErr(j) {
    var c = j && j.error && j.error.code;
    toast(c === 'rate_limited' ? t('slow') : c === 'runs_limit' ? t('limit', { n: S.cfg.max_runs }) : t('err'));
  }

  function refresh() {
    if (S.busy) return;
    S.busy = true;
    var b = $('refreshBtn'); b.disabled = true; b.classList.add('busy');
    api('POST', '/api/refresh').then(function (j) {
      if (j.__status !== 200) { handleErr(j); return; }
      S.flash = true; setState(j); S.flash = false;
      toast(t('done', { d: fmtDate(j.date), n: j.run.fetched + j.run.failed, c: j.changes }));
    }).catch(function () { toast(t('err')); }).then(function () { S.busy = false; b.disabled = false; b.classList.remove('busy'); });
  }

  function selectTab(name) {
    S.tab = name;
    document.querySelectorAll('#tabs button').forEach(function (b) { b.setAttribute('aria-selected', String(b.dataset.tab === name)); });
    document.querySelectorAll('.panel').forEach(function (p) { p.hidden = p.dataset.panel !== name; });
    try { history.replaceState(null, '', '#' + name); } catch (e) { /* ignore */ }
  }

  document.addEventListener('click', function (ev) {
    var el = ev.target.closest('button, [data-toggle]');
    if (!el) return;
    if (el.closest('#tabs')) { selectTab(el.dataset.tab); return; }
    if (el.id === 'refreshBtn') { refresh(); return; }
    if (el.id === 'resetBtn') {
      api('POST', '/api/reset').then(function () { S.open = {}; S.drafts = {}; return load(); }).then(function () { toast(t('resetDone')); });
      return;
    }
    if (el.dataset.toggle && !ev.target.closest('button')) {
      var pid = el.dataset.toggle, idx = S.st.products.findIndex(function (p) { return p.id === pid; });
      var cur = S.open[pid] !== undefined ? S.open[pid] : idx === 0;
      S.open[pid] = !cur; renderProducts(); return;
    }
    if (el.dataset.apply) {
      el.disabled = true;
      api('POST', '/api/products/' + el.dataset.apply + '/apply').then(function (j) {
        if (j.__status !== 200) { handleErr(j); el.disabled = false; return; }
        setState(j); toast(t('applied', { p: rub(j.applied.price) }));
      });
      return;
    }
    if (el.dataset.remove) {
      api('POST', '/api/competitors/remove', { key: el.dataset.remove }).then(function (j) { if (j.__status === 200) setState(j); });
      return;
    }
    if (el.dataset.send) {
      el.disabled = true;
      api('POST', '/api/report/send', { channel: el.dataset.send }).then(function (j) {
        el.disabled = false;
        if (j.__status !== 200) { handleErr(j); return; }
        S.st.messages = j.messages; renderReport(); toast(t('sent', { c: t('chanNames')[el.dataset.send] }));
      });
      return;
    }
    if (el.dataset.draft) {
      var id = el.dataset.draft; el.disabled = true;
      delete S.drafts[id];
      api('POST', '/api/reviews/' + id + '/draft').then(function (j) {
        if (j.__status !== 200) { handleErr(j); el.disabled = false; return; }
        S.st.reviews.forEach(function (r) { if (r.id === id) r.draft = j.draft; });
        if (j.mode && !j.mode.live && j.mode.reason && j.mode.reason !== 'disabled') toast(j.mode.reason === 'unauthenticated' ? t('needLogin') : t('liveLimit'));
        renderReviews();
        var ta = $('dt-' + id); if (ta) ta.focus();
      });
      return;
    }
    if (el.dataset.publish) {
      var rid = el.dataset.publish, text = $('dt-' + rid).value;
      el.disabled = true;
      api('POST', '/api/reviews/' + rid + '/answer', { text: text }).then(function (j) {
        if (j.__status !== 200) { handleErr(j); el.disabled = false; return; }
        delete S.drafts[rid]; setState(j); toast(t('published'));
      });
    }
  });

  document.addEventListener('input', function (ev) {
    if (ev.target.id && ev.target.id.indexOf('dt-') === 0) S.drafts[ev.target.id.slice(3)] = ev.target.value;
  });

  document.addEventListener('submit', function (ev) {
    var f = ev.target.closest('form[data-add]');
    if (!f) return;
    ev.preventDefault();
    var pid = f.dataset.add, ref = f.ref.value.trim(), err = $('ae-' + pid);
    if (!ref) return;
    api('POST', '/api/competitors', { product_id: pid, ref: ref }).then(function (j) {
      if (j.__status === 200) { S.open[pid] = true; setState(j); toast(t('added')); return; }
      var c = j.error && j.error.code;
      err.textContent = c === 'bad_ref' ? t('addBad') : c === 'duplicate' ? t('addDup') : c === 'too_many' ? t('addMany', { n: S.cfg.max_added }) : t('err');
    });
  });

  $('langSeg').addEventListener('click', function (ev) {
    var b = ev.target.closest('button'); if (!b) return;
    S.lang = b.dataset.lang; applyI18n(); load();
  });

  // RU edition: sign-in widget of the auth service (live AI drafts within a daily limit); off until AUTH_WIDGET_URL is set
  function mountAuth(c) {
    if (!c.auth_widget || !c.auth_widget.url) return;
    var box = document.createElement('div'); box.id = 'sl-account';
    document.querySelector('.controls').insertBefore(box, document.querySelector('.controls').firstChild);
    var sc = document.createElement('script');
    sc.src = c.auth_widget.url; sc.defer = true;
    sc.setAttribute('data-demo', c.auth_widget.demo); sc.setAttribute('data-lang', S.lang);
    sc.setAttribute('data-resource', c.auth_widget.resource);
    sc.setAttribute('data-providers', (c.llm || 'YandexGPT') + ' (Россия)');
    document.body.appendChild(sc);
  }

  function load() { return api('GET', '/api/state').then(function (j) { if (j.__status === 200) setState(j); }); }

  api('GET', '/api/config').then(function (c) {
    S.cfg = c;
    if (c.edition === 'intl') S.lang = 'en';
    else if (c.edition === 'ru') S.lang = 'ru';
    else { S.lang = /[?&]lang=en/.test(location.search) ? 'en' : 'ru'; $('langSeg').hidden = false; }
    if (c.other_edition_url) { var a = $('otherEdition'); a.href = c.other_edition_url; a.hidden = false; }
    applyI18n();
    if (c.other_edition_url) $('otherEdition').textContent = t('other');
    $('csvBtn').textContent = t('csv');
    var x = document.createElement('a'); x.className = 'ghost btn'; x.href = '/api/export.xlsx'; x.setAttribute('download', ''); x.id = 'xlsxBtn'; x.textContent = t('xlsx');
    $('csvBtn').after(x);
    mountAuth(c);
    var h = (location.hash || '').slice(1);
    if (['prices', 'report', 'sheet', 'reviews', 'log'].indexOf(h) >= 0) selectTab(h);
    return load();
  });
})();
