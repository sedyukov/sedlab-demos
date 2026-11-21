/* Orders API demo page: backend switch (Go / Python), sandbox token, side-by-side comparison, live job queue, Swagger UI. */
(function () {
  "use strict";

  var T = {
    ru: {
      h1: "REST API на Go и Python: один контракт, одинаковые ответы",
      lead: "Учёт заказов небольшой компании: клиенты, заказы со статусами, фоновые задачи. Один OpenAPI-контракт и две реализации, Go и Python (FastAPI), на общей базе. Переключайте бэкенд и сравнивайте: ответы совпадают.",
      f1: "Авторизация по токену, у каждого посетителя своя песочница с тестовыми данными",
      f2: "CRUD, фильтры, сортировка и постраничный вывод, понятные ошибки",
      f3: "Очередь фоновых задач с повторами при сбоях, её разбирают оба бэкенда",
      f4: "Миграции, контрактные тесты на обе реализации, Docker, Swagger",
      backend: "Бэкенд",
      access: "Доступ",
      getToken: "Получить токен песочницы",
      tokenNone: "Токена нет: нажмите кнопку, и Swagger авторизуется сам",
      tokenOk: "Токен получен, песочница {ws}. Swagger авторизован.",
      tokenAcc: "Вход по аккаунту, рабочее пространство {ws}. Swagger авторизован.",
      accTitle: "Вход через аккаунт sedlab (Max, Яндекс ID, VK ID)",
      accToken: "Получить API-токен по аккаунту",
      accHint: "Сервис входа выдаёт сессию, API меняет её на свой токен. Рабочее пространство сохраняется за аккаунтом.",
      accNeedLogin: "Сначала войдите через аккаунт",
      cmpTitle: "Go и Python отвечают одинаково",
      cmpHint: "Одни и те же запросы уходят в обе реализации: успешные ответы, ошибки валидации, 404. Сравниваются код ответа и JSON.",
      cmpBtn: "Сравнить ответы",
      cmpReq: "Запрос",
      cmpSame: "Совпадают все {n} ответов: те же коды и тот же JSON.",
      cmpDiff: "Есть расхождения: {n}",
      jobsTitle: "Фоновые задачи и повторы",
      jobsHint: "Задача ставится в общую очередь. Её забирает свободный воркер, Go или Python. При сбое повтор через 1, 2, 4 секунды, после 4 попыток задача уходит в «dead».",
      jobOk: "Выставить счёт",
      jobRetry: "Синхронизация с CRM (2 сбоя)",
      jobDead: "Уведомление (все попытки неудачны)",
      jobsEmpty: "Пока задач нет. Нажмите кнопку выше.",
      next: "повтор через {s} с",
      swTitle: "Swagger: попробуйте любой метод",
      swHint: "Сервер в Swagger следует за переключателем бэкенда",
      foot: "Данные в песочницах тестовые и удаляются каждый день. Не вводите реальные данные.",
      other: "English version",
      needToken: "Сначала получите токен"
    },
    en: {
      h1: "REST API in Go and Python: one contract, identical answers",
      lead: "An order desk for a small business: customers, orders with a status workflow, background jobs. One OpenAPI contract, two implementations, Go and Python (FastAPI), on one shared database. Switch the backend and compare: the answers match.",
      f1: "Token auth, every visitor gets a private sandbox with sample data",
      f2: "CRUD, filters, sorting, cursor pagination, clear errors",
      f3: "Background job queue with retries on failures, served by both backends",
      f4: "Migrations, one contract test suite for both implementations, Docker, Swagger",
      backend: "Backend",
      access: "Access",
      getToken: "Get a sandbox token",
      tokenNone: "No token yet: press the button and Swagger will authorize itself",
      tokenOk: "Token issued, sandbox {ws}. Swagger is authorized.",
      tokenAcc: "Signed in with an account, workspace {ws}. Swagger is authorized.",
      accTitle: "Sign in with a sedlab account (Max, Yandex ID, VK ID)",
      accToken: "Get an API token for my account",
      accHint: "The sign-in service issues a session, the API exchanges it for its own token. The workspace stays with the account.",
      accNeedLogin: "Sign in with an account first",
      cmpTitle: "Go and Python answer the same",
      cmpHint: "The same requests go to both implementations: successful answers, validation errors, 404. Status codes and JSON are compared.",
      cmpBtn: "Compare answers",
      cmpReq: "Request",
      cmpSame: "All {n} answers match: the same status codes and the same JSON.",
      cmpDiff: "Differences found: {n}",
      jobsTitle: "Background jobs and retries",
      jobsHint: "A job goes into the shared queue. A free worker, Go or Python, picks it up. On failure it retries after 1, 2, 4 seconds; after 4 attempts the job is dead.",
      jobOk: "Send invoice",
      jobRetry: "Sync to CRM (2 failures)",
      jobDead: "Notify customer (every attempt fails)",
      jobsEmpty: "No jobs yet. Press a button above.",
      next: "retry in {s} s",
      swTitle: "Swagger: try any endpoint",
      swHint: "The Swagger server follows the backend switch",
      foot: "Sandbox data is synthetic and is wiped daily. Do not enter real data.",
      other: "Русская версия",
      needToken: "Get a token first"
    }
  };

  var store = {
    get: function (k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
    set: function (k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } }
  };

  var host = location.hostname;
  var isEn = /^api-en\./.test(host);
  var state = {
    lang: store.get("api.lang") || (isEn ? "en" : "ru"),
    backend: store.get("api.backend") || "go",
    token: store.get("api.token"),
    tokenKind: store.get("api.tokenKind") || "sandbox",
    ws: store.get("api.ws"),
    order: null,
    jobIds: [],
    config: { edition: isEn ? "intl" : "ru", accounts: false }
  };
  var ui = null;

  function $(s) { return document.querySelector(s); }
  function $$(s) { return Array.prototype.slice.call(document.querySelectorAll(s)); }
  function t(k, vars) {
    var s = (T[state.lang] || T.ru)[k] || k;
    for (var v in vars || {}) s = s.replace("{" + v + "}", vars[v]);
    return s;
  }
  function esc(s) { return String(s).replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }

  // ---- language / edition ----
  function applyLang() {
    document.documentElement.lang = state.lang;
    $$("[data-i18n]").forEach(function (el) { el.textContent = t(el.getAttribute("data-i18n")); });
    $$("#lang button").forEach(function (b) { b.classList.toggle("on", b.dataset.lang === state.lang); });
    var other = $("#other-edition");
    if (/demo\.sedlab\.dev$/.test(host)) {
      other.href = "https://" + (isEn ? host.replace(/^api-en\./, "api.") : host.replace(/^api\./, "api-en.")) + "/";
      other.textContent = isEn ? T.ru.other : T.en.other;
    } else { other.hidden = true; }
    renderTokenState();
    renderJobs(lastJobs);
  }
  $$("#lang button").forEach(function (b) {
    b.onclick = function () { state.lang = b.dataset.lang; store.set("api.lang", state.lang); applyLang(); };
  });

  // ---- backend switch ----
  function base(b) { return "/" + (b || state.backend); }
  function applyBackend() {
    $$("#backend button").forEach(function (b) { b.classList.toggle("on", b.dataset.b === state.backend); });
    if (ui && ui.oas3Actions && ui.oas3Actions.setSelectedServer) {
      try { ui.oas3Actions.setSelectedServer({ selectedServerUrl: base(), namespace: "" }); } catch (e) { /* older UI */ }
    }
  }
  $$("#backend button").forEach(function (b) {
    b.onclick = function () { state.backend = b.dataset.b; store.set("api.backend", state.backend); applyBackend(); };
  });

  // ---- HTTP ----
  function call(method, path, body, b, extra) {
    var headers = { "Accept": "application/json" };
    if (state.token) headers["Authorization"] = "Bearer " + state.token;
    if (body !== undefined) headers["Content-Type"] = "application/json";
    var t0 = performance.now();
    return fetch(base(b) + path, Object.assign({ method: method, headers: headers, body: body === undefined ? undefined : JSON.stringify(body) }, extra || {}))
      .then(function (r) {
        return r.text().then(function (txt) {
          var json = null;
          try { json = txt ? JSON.parse(txt) : null; } catch (e) { json = txt; }
          return { status: r.status, json: json, ms: Math.round(performance.now() - t0), backend: r.headers.get("X-Backend") };
        });
      });
  }

  // ---- token ----
  function setToken(resp, kind) {
    state.token = resp.access_token; state.ws = resp.workspace.id; state.tokenKind = kind; state.order = null;
    store.set("api.token", state.token); store.set("api.ws", state.ws); store.set("api.tokenKind", kind);
    authorizeSwagger();
    renderTokenState();
  }
  function renderTokenState() {
    var el = $("#token-state");
    if (!el) return;
    el.textContent = state.token ? t(state.tokenKind === "accounts" ? "tokenAcc" : "tokenOk", { ws: state.ws }) : t("tokenNone");
    el.classList.toggle("ok", !!state.token);
  }
  function authorizeSwagger() {
    if (ui && state.token) { try { ui.preauthorizeApiKey("bearerAuth", state.token); } catch (e) { /* ignore */ } }
  }
  function ensureToken() {
    if (!state.token) return newSandbox();
    return call("GET", "/v1/me").then(function (r) { return r.status === 401 ? newSandbox() : null; });
  }
  function newSandbox() {
    return call("POST", "/v1/auth/sandbox").then(function (r) {
      if (r.status !== 201) { alert((r.json && r.json.error && r.json.error.message) || ("HTTP " + r.status)); throw new Error("sandbox"); }
      setToken(r.json, "sandbox");
    });
  }
  $("#get-token").onclick = function () { newSandbox().catch(function () {}); };

  $("#acc-token").onclick = function () {
    call("POST", "/v1/auth/accounts", undefined, undefined, { credentials: "include" }).then(function (r) {
      if (r.status === 200) setToken(r.json, "accounts");
      else alert(r.status === 401 ? t("accNeedLogin") : ((r.json && r.json.error && r.json.error.message) || r.status));
    });
  };

  // ---- comparison ----
  var CMP = [
    ["GET", "/v1/orders?status=new,confirmed&sort=-total&limit=5"],
    ["GET", "/v1/customers?sort=name&limit=3"],
    ["GET", "/v1/me"],
    ["GET", "/v1/orders?limit=0&sort=price"],
    ["POST", "/v1/orders", { customer_id: "cus_unknown", title: "", total: -5, tags: ["Bad tag"] }],
    ["GET", "/v1/orders/ord_missing"]
  ];
  function norm(v) {
    if (Array.isArray(v)) return v.map(norm);
    if (v && typeof v === "object") {
      var o = {};
      Object.keys(v).sort().forEach(function (k) { if (k !== "backend") o[k] = norm(v[k]); });
      return o;
    }
    return v;
  }
  $("#compare").onclick = function () {
    var btn = this; btn.disabled = true;
    ensureToken().then(function () {
      var tbody = $("#cmp-table tbody"); tbody.innerHTML = "";
      $("#cmp-table").hidden = false;
      var diffs = 0;
      var seq = Promise.resolve();
      CMP.forEach(function (c) {
        seq = seq.then(function () {
          return call(c[0], c[1], c[2], "go").then(function (g) {
            return call(c[0], c[1], c[2], "py").then(function (p) {
              var same = g.status === p.status && JSON.stringify(norm(g.json)) === JSON.stringify(norm(p.json));
              if (!same) diffs++;
              var tr = document.createElement("tr");
              tr.innerHTML = "<td><code>" + esc(c[0] + " " + c[1].replace(/\?.*/, function (q) { return q.length > 30 ? q.slice(0, 30) + "…" : q; })) + "</code></td>" +
                "<td>" + chip(g.status) + " <span class='muted ms'>" + g.ms + " ms</span></td>" +
                "<td>" + chip(p.status) + " <span class='muted ms'>" + p.ms + " ms</span></td>" +
                "<td class=" + (same ? "same" : "diff") + ">" + (same ? "✓" : "✗") + "</td>";
              tbody.appendChild(tr);
            });
          });
        });
      });
      return seq.then(function () {
        var s = $("#cmp-summary"); s.hidden = false;
        s.className = "summary " + (diffs ? "bad" : "good");
        s.textContent = diffs ? t("cmpDiff", { n: diffs }) : t("cmpSame", { n: CMP.length });
      });
    }).catch(function () {}).then(function () { btn.disabled = false; });
  };
  function chip(status) {
    var cls = status < 300 ? "ok" : status < 500 ? "warn" : "bad";
    return "<span class='chip " + cls + "'>" + status + "</span>";
  }

  // ---- jobs ----
  var lastJobs = [];
  var polling = null;
  $$("[data-job]").forEach(function (b) {
    b.onclick = function () {
      b.disabled = true;
      ensureToken().then(function () {
        return state.order ? null : call("GET", "/v1/orders?sort=number&limit=1").then(function (r) { state.order = r.json.items[0]; });
      }).then(function () {
        return call("POST", "/v1/orders/" + state.order.id + "/jobs", { type: b.dataset.job, simulate_failures: +b.dataset.fail });
      }).then(function (r) {
        if (r.status !== 202) { alert((r.json && r.json.error && r.json.error.message) || r.status); return; }
        startPolling();
      }).catch(function () {}).then(function () { b.disabled = false; });
    };
  });
  function startPolling() {
    if (polling) return;
    var idle = 0;
    polling = setInterval(function () {
      call("GET", "/v1/jobs?limit=6").then(function (r) {
        if (r.status !== 200) return;
        lastJobs = r.json.items;
        renderJobs(lastJobs);
        var active = lastJobs.some(function (j) { return j.status === "queued" || j.status === "running" || j.status === "retrying"; });
        idle = active ? 0 : idle + 1;
        if (idle > 3) { clearInterval(polling); polling = null; }
      });
    }, 700);
  }
  var NAMES = { send_invoice: ["Счёт", "Invoice"], sync_crm: ["CRM", "CRM"], notify_customer: ["Уведомление", "Notify"] };
  function renderJobs(items) {
    var box = $("#jobs");
    if (!box) return;
    if (!items || !items.length) { box.innerHTML = "<div class='muted small'>" + esc(t("jobsEmpty")) + "</div>"; return; }
    var now = Date.now();
    box.innerHTML = items.map(function (j) {
      var hist = j.history.map(function (h) {
        return "<span class='att " + (h.outcome === "ok" ? "ok" : "err") + "' title='" + esc(h.error || "ok") + "'>" + h.attempt +
          " <span class='dot " + (h.worker === "go" ? "go" : "py") + "'></span>" + (h.worker === "go" ? "Go" : "Py") + (h.outcome === "ok" ? " ✓" : " ✗") + "</span>";
      }).join("");
      var extra = "";
      if (j.status === "retrying" && j.next_run_at) {
        var s = Math.max(0, Math.ceil((Date.parse(j.next_run_at) - now) / 1000));
        extra = "<span class='muted small'>" + esc(t("next", { s: s })) + "</span>";
      }
      if (j.status === "succeeded" && j.result) {
        var r = j.result;
        extra = "<code class='small'>" + esc(r.invoice_number || r.crm_deal_id || r.template || "") + "</code>";
      }
      return "<div class='job'><div class='row between'><b>" + esc(NAMES[j.type][state.lang === "ru" ? 0 : 1]) + "</b>" +
        "<span class='st " + j.status + "'>" + j.status + "</span></div>" +
        "<div class='row gap wrapline'>" + hist + extra + "</div></div>";
    }).join("");
  }

  // ---- Swagger ----
  function initSwagger() {
    ui = SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger",
      deepLinking: false,
      tryItOutEnabled: true,
      displayRequestDuration: true,
      defaultModelsExpandDepth: 0,
      docExpansion: "list",
      requestInterceptor: function (req) {
        // the switch above decides which backend serves Swagger's requests
        req.url = req.url.replace(/\/(go|py)\/(v1|health)/, base() + "/$2");
        return req;
      },
      onComplete: function () { applyBackend(); authorizeSwagger(); }
    });
  }

  // ---- accounts widget (Russian edition only) ----
  function initAccounts(cfg) {
    if (!cfg.accounts || !cfg.auth_widget_url) return;
    $("#accounts-box").hidden = false;
    var s = document.createElement("script");
    s.src = cfg.auth_widget_url; s.defer = true;
    s.setAttribute("data-demo", "api");
    s.setAttribute("data-providers", "max,yandex,vk");
    document.body.appendChild(s);
  }

  fetch("/ui-config.json").then(function (r) { return r.json(); }).then(function (cfg) {
    state.config = cfg;
    if (!store.get("api.lang")) state.lang = cfg.edition === "ru" ? "ru" : "en";
    initAccounts(cfg);
  }).catch(function () {}).then(function () {
    applyLang();
    applyBackend();
    initSwagger();
    if (state.token) { call("GET", "/v1/jobs?limit=6").then(function (r) { if (r.status === 200) { lastJobs = r.json.items; renderJobs(lastJobs); } else if (r.status === 401) { state.token = null; renderTokenState(); } }); }
  });
})();
