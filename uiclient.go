package main

const comboPageJS = `
(function () {
  "use strict";

  var PAGE = location.pathname.replace(/\/+$/, "");
  var MARK = "/v0/resource/plugins/";
  var CUT = location.pathname.indexOf(MARK);
  var PREFIX = CUT >= 0 ? location.pathname.slice(0, CUT) : "";
  var API = PREFIX + "/v0/management/combos/api";
  var KEY_STORE = "cpamp-combos-management-key";
  var state = { combos: [], filter: "all", key: null, draft: [], accounts: [] };
  try { state.key = localStorage.getItem(KEY_STORE) || null; } catch (e) { state.key = null; }

  var $ = function (id) { return document.getElementById(id); };
  var list = $("list");

  function authHeaders() {
    return state.key ? { Authorization: "Bearer " + state.key } : {};
  }

  function esc(v) {
    return String(v == null ? "" : v)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  function setMsg(text, cls) {
    var el = $("c-msg");
    el.textContent = text;
    el.className = "msg " + (cls || "");
  }

  function applyLock() {
    var unlocked = !!state.key;
    var lock = $("lock");
    lock.dataset.state = unlocked ? "unlocked" : "locked";
    $("lock-label").textContent = unlocked ? "Editing" : "Read only";
    $("unlock-btn").textContent = unlocked ? "Lock" : "Unlock";
    $("c-submit").disabled = !unlocked;
    Array.prototype.forEach.call(document.querySelectorAll("[data-needs-key]"), function (el) {
      el.disabled = !unlocked;
    });
  }

  function api(path, options) {
    options = options || {};
    return fetch(API + path, {
      method: options.method || "GET",
      headers: Object.assign({ "Content-Type": "application/json" }, authHeaders()),
      body: options.body ? JSON.stringify(options.body) : undefined
    }).then(function (res) {
      return res.text().then(function (text) {
        var data = null;
        try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
        if (!res.ok) {
          var msg = (data && data.error) || ("HTTP " + res.status);
          throw new Error(msg);
        }
        return data;
      });
    });
  }

  function load() {
    return fetch(PAGE + "?asset=data").then(function (r) { return r.json(); })
      .then(function (d) {
        state.combos = (d && d.combos) || [];
        render();
      })
      .catch(function () {
        list.innerHTML = '<div class="empty">Could not load combos.</div>';
      });
  }

  function save(next, verb) {
    return api("", { method: verb, body: { version: 1, combos: next } })
      .then(function () { return load(); })
      .then(function () { setMsg("Saved.", "ok"); })
      .catch(function (e) { setMsg("Save failed: " + e.message, "err"); });
  }

  function shadowCheck() {
    var box = $("shadow");
    if (!box) return;
    var catalog = (window.__comboCatalog || []);
    if (!catalog.length) { box.innerHTML = ""; return; }
    var real = {};
    catalog.forEach(function (m) {
      var id = String(m.id || "");
      if (!id) return;
      if (m.owned_by === "combo" || state.combos.some(function (c) { return c.name === id; })) return;
      real[id] = m;
    });
    var clashes = state.combos.filter(function (c) { return real[c.name]; });
    if (!clashes.length) { box.innerHTML = ""; return; }
    box.innerHTML = clashes.map(function (c) {
      return '<div class="warn"><strong>' + esc(c.name) + "</strong> shadows the real model " +
        esc(c.name) + " (" + esc(real[c.name].display_name || c.name) +
        "). Requests for that name go to the combo, not the original model.</div>";
    }).join("");
  }

  function stat() {
    var targets = 0, providers = {};
    state.combos.forEach(function (c) {
      targets += c.targets.length;
      c.targets.forEach(function (t) { if (t.provider) providers[t.provider] = 1; });
    });
    $("stat-combos").textContent = state.combos.length;
    $("stat-targets").textContent = targets;
    $("stat-providers").textContent = Object.keys(providers).length;
  }

  function renderFilters() {
    var groups = {
      all: state.combos.length,
      single: state.combos.filter(function (c) { return c.targets.length === 1; }).length,
      multi: state.combos.filter(function (c) { return c.targets.length > 1; }).length
    };
    var labels = { all: "All", single: "Single target", multi: "Multi target" };
    $("filters").innerHTML = Object.keys(groups).map(function (k) {
      return '<button type="button" data-f="' + k + '" aria-pressed="' +
        (state.filter === k) + '">' + labels[k] + ' <span class="n">' + groups[k] + "</span></button>";
    }).join("");
    Array.prototype.forEach.call($("filters").querySelectorAll("button"), function (b) {
      b.addEventListener("click", function () {
        state.filter = b.dataset.f;
        render();
      });
    });
  }

  function visible() {
    if (state.filter === "single") {
      return state.combos.filter(function (c) { return c.targets.length === 1; });
    }
    if (state.filter === "multi") {
      return state.combos.filter(function (c) { return c.targets.length > 1; });
    }
    return state.combos.slice();
  }

  function loadAccounts() {
    if (!state.key || state.accounts.length) return Promise.resolve();
    return api("/accounts").then(function (d) {
      state.accounts = (d && d.accounts) || [];
    }).catch(function () {
      state.accounts = [];
    });
  }

  function accountGroups() {
    var byProvider = {};
    (state.accounts || []).forEach(function (a) {
      var key = a.provider || "other";
      (byProvider[key] = byProvider[key] || []).push(a);
    });
    return Object.keys(byProvider).sort().map(function (key) {
      return { provider: key, accounts: byProvider[key] };
    });
  }

  function accountSelect(t, i) {
    if (!state.accounts.length) return "";
    var groups = accountGroups();
    var opts = '<option value=""' + (t.auth_id ? "" : " selected") + ">Any (auto)</option>";
    if (t.auth_id && !(state.accounts || []).some(function (a) { return a.id === t.auth_id; })) {
      opts += '<option value="' + esc(t.auth_id) + '" selected>' +
        esc(t.account || t.auth_id) + " (missing)</option>";
    }
    groups.forEach(function (g) {
      opts += '<optgroup label="' + esc(g.provider) + '">';
      g.accounts.forEach(function (a) {
        var tag = a.label || a.name;
        if (a.disabled) tag += " (disabled)";
        else if (a.unavailable) tag += " (unavailable)";
        opts += '<option value="' + esc(a.id) + '"' + (t.auth_id === a.id ? " selected" : "") + ">" +
          esc(tag) + "</option>";
      });
      opts += "</optgroup>";
    });
    return '<select class="acct" data-acct="' + i + '">' + opts + "</select>";
  }

  function applyAccount(i, id) {
    var t = state.draft[i];
    if (!t) return;
    if (!id) { delete t.auth_id; delete t.account; return; }
    var hit = (state.accounts || []).filter(function (a) { return a.id === id; })[0];
    t.auth_id = id;
    t.account = hit ? (hit.label || hit.name) : id;
  }

  function targetRow(combo, t, i) {
    var label = t.label || t.model;
    var sub = t.provider ? t.provider : "resolved by model name";
    if (t.account) sub += " · " + t.account;
    return '<div class="target" data-first="' + (i === 0 ? "1" : "") + '" draggable="true" data-i="' + i + '">' +
      '<span class="idx">' + (i + 1) + "</span>" +
      '<span class="meta"><span class="m">' + esc(label) + "</span>" +
      '<span class="p">' + esc(sub) + "</span></span>" +
      '<button class="btn sm ghost" data-up="' + i + '" title="Move up"' + (i === 0 ? " disabled" : "") + ">&uarr;</button>" +
      '<button class="btn sm ghost" data-down="' + i + '" title="Move down"' +
        (i === combo.targets.length - 1 ? " disabled" : "") + ">&darr;</button>" +
      "</div>";
  }

  function card(c) {
    var id = c.name;
    var rows = c.targets.map(function (t, i) { return targetRow(c, t, i); }).join("");
    return '<section class="card" data-name="' + esc(c.name) + '">' +
      '<div class="combo-head"><div class="combo-id">' +
        '<span class="combo-name">' + esc(c.name) + "</span>" +
        '<span class="badge">priority</span>' +
        '<code class="combo-model">' + esc(id) + "</code>" +
      "</div>" +
      '<div class="combo-actions">' +
        '<button class="btn sm" data-addmodels="' + esc(c.name) + '" data-needs-key>Add models</button>' +
        '<button class="btn sm" data-copy="' + esc(id) + '">Copy id</button>' +
        '<button class="btn sm danger" data-del="' + esc(c.name) + '" data-needs-key>Delete</button>' +
      "</div></div>" +
      (c.description ? '<p class="combo-desc">' + esc(c.description) + "</p>" : "") +
      '<div class="targets">' + rows + "</div>" +
      "</section>";
  }

  function render() {
    stat();
    renderFilters();
    shadowCheck();
    var items = visible();
    if (!items.length) {
      list.innerHTML = state.combos.length
        ? '<div class="empty">No combos in this filter.</div>'
        : '<div class="empty">No combos yet. Create one below to expose a fallback chain as a single model.</div>';
      return;
    }
    list.innerHTML = items.map(card).join("");
    wire();
    applyLock();
  }

  function wire() {
    Array.prototype.forEach.call(list.querySelectorAll("[data-del]"), function (b) {
      b.addEventListener("click", function () {
        var name = b.dataset.del;
        if (!confirm('Delete combo "' + name + '"?')) return;
        var next = state.combos.filter(function (c) { return c.name !== name; });
        save(next, "PUT");
      });
    });

    Array.prototype.forEach.call(list.querySelectorAll("[data-addmodels]"), function (b) {
      b.addEventListener("click", function () {
        window.__comboEditing = b.dataset.addmodels;
        window.__comboState = state;
        window.__comboOpenPicker();
      });
    });

    Array.prototype.forEach.call(list.querySelectorAll("[data-copy]"), function (b) {
      b.addEventListener("click", function () {
        var text = b.dataset.copy;
        if (navigator.clipboard) {
          navigator.clipboard.writeText(text).then(function () {
            b.textContent = "Copied";
            setTimeout(function () { b.textContent = "Copy id"; }, 1200);
          });
        }
      });
    });

    Array.prototype.forEach.call(list.querySelectorAll("[data-up],[data-down]"), function (b) {
      b.addEventListener("click", function () {
        var card = b.closest(".card");
        var combo = state.combos.filter(function (c) { return c.name === card.dataset.name; })[0];
        if (!combo) return;
        var i = b.dataset.up !== undefined ? Number(b.dataset.up) : Number(b.dataset.down) + 1;
        var j = b.dataset.up !== undefined ? i - 1 : i + 1;
        if (j < 0 || j >= combo.targets.length) return;
        var tmp = combo.targets[i];
        combo.targets[i] = combo.targets[j];
        combo.targets[j] = tmp;
        var next = state.combos.map(function (c) {
          return c.name === combo.name
            ? { name: c.name, description: c.description, targets: c.targets }
            : { name: c.name, description: c.description, targets: c.targets };
        });
        save(next, "PUT");
      });
    });

    Array.prototype.forEach.call(list.querySelectorAll(".target"), function (row) {
      row.addEventListener("dragstart", function (e) {
        row.dataset.drag = "1";
        e.dataTransfer.effectAllowed = "move";
        try { e.dataTransfer.setData("text/plain", row.dataset.i); } catch (err) {}
      });
      row.addEventListener("dragend", function () { delete row.dataset.drag; });
      row.addEventListener("dragover", function (e) { e.preventDefault(); });
      row.addEventListener("drop", function (e) {
        e.preventDefault();
        var card = row.closest(".card");
        var combo = state.combos.filter(function (c) { return c.name === card.dataset.name; })[0];
        if (!combo) return;
        var from = Number(row.dataset.i);
        var to = Number(e.dataTransfer.getData("text/plain") || from);
        if (isNaN(to) || from === to) return;
        var moved = combo.targets.splice(from, 1)[0];
        combo.targets.splice(to, 0, moved);
        save(state.combos.map(function (c) {
          return { name: c.name, description: c.description, targets: c.targets };
        }), "PUT");
      });
    });
  }

  function renderDraft() {
    var box = $("c-targets");
    if (!state.draft.length) {
      box.className = "targets-empty";
      box.textContent = "No models picked yet.";
      return;
    }
    box.className = "draft";
    box.innerHTML = state.draft.map(function (t, i) {
      var label = t.model;
      var sub = t.provider || "resolved by model name";
      return '<div class="target" data-first="' + (i === 0 ? "1" : "") + '">' +
        '<span class="idx">' + (i + 1) + "</span>" +
        '<span class="meta"><span class="m">' + esc(label) + "</span>" +
        '<span class="p">' + esc(sub) + "</span></span>" +
        accountSelect(t, i) +
        '<button class="btn sm ghost" data-clone="' + i + '" title="Add the same model again on another account">+acct</button>' +
        '<button class="btn sm ghost" data-dup="' + i + '" title="Move up"' + (i === 0 ? " disabled" : "") + ">&uarr;</button>" +
        '<button class="btn sm ghost" data-ddn="' + i + '" title="Move down"' +
          (i === state.draft.length - 1 ? " disabled" : "") + ">&darr;</button>" +
        '<button class="btn sm ghost" data-rm="' + i + '" title="Remove">Remove</button>' +
        "</div>";
    }).join("");
    Array.prototype.forEach.call(box.querySelectorAll("[data-acct]"), function (s) {
      s.addEventListener("change", function () {
        applyAccount(Number(s.dataset.acct), s.value);
        renderDraft();
      });
    });
    Array.prototype.forEach.call(box.querySelectorAll("[data-clone]"), function (b) {
      b.addEventListener("click", function () {
        var i = Number(b.dataset.clone);
        var src = state.draft[i];
        if (!src) return;
        var copy = { provider: src.provider, model: src.model };
        state.draft.splice(i + 1, 0, copy);
        renderDraft();
        setMsg("Copied " + src.model + ". Pick a different account for the new row.", "ok");
      });
    });
    Array.prototype.forEach.call(box.querySelectorAll("[data-rm]"), function (b) {
      b.addEventListener("click", function () { state.draft.splice(Number(b.dataset.rm), 1); renderDraft(); });
    });
    Array.prototype.forEach.call(box.querySelectorAll("[data-dup]"), function (b) {
      b.addEventListener("click", function () { move(Number(b.dataset.dup), -1); });
    });
    Array.prototype.forEach.call(box.querySelectorAll("[data-ddn]"), function (b) {
      b.addEventListener("click", function () { move(Number(b.dataset.ddn), 1); });
    });
  }

  function move(i, d) {
    var j = i + d;
    if (j < 0 || j >= state.draft.length) return;
    var t = state.draft[i];
    state.draft[i] = state.draft[j];
    state.draft[j] = t;
    renderDraft();
  }

  window.__comboCatalog = [];
  window.__comboReload = function () { return load(); };
  window.__comboRenderDraft = function () { renderDraft(); };

  $("c-pick").addEventListener("click", function () {
    window.__comboEditing = null;
    window.__comboState = state;
    window.__comboOpenPicker();
  });

  $("c-submit").addEventListener("click", function () {
    var name = $("c-name").value.trim();
    if (!name) return setMsg("Name is required.", "err");
    if (!state.draft.length) return setMsg("Pick at least one model.", "err");
    if (state.combos.some(function (c) { return c.name === name; })) {
      return setMsg("A combo named " + name + " already exists.", "err");
    }
    var desc = $("c-desc").value.trim();
    api("", { method: "POST", body: { name: name, description: desc, targets: state.draft } })
      .then(function () { return load(); })
      .then(function () {
        $("c-name").value = ""; $("c-desc").value = ""; state.draft = [];
        renderDraft();
        setMsg("Created " + name + ".", "ok");
      })
      .catch(function (e) { setMsg("Create failed: " + e.message, "err"); });
  });

  var dlg = $("unlock");
  $("unlock-btn").addEventListener("click", function () {
    if (state.key) {
      state.key = null;
      try { localStorage.removeItem(KEY_STORE); } catch (e) {}
      applyLock();
      return;
    }
    $("u-key").value = "";
    dlg.showModal();
  });
  dlg.addEventListener("close", function () {
    var v = dlg.returnValue;
    var key = $("u-key").value.trim();
    if (v !== "ok" || !key) return;
    state.key = key;
    try { localStorage.setItem(KEY_STORE, key); } catch (e) {}
    applyLock();
    setMsg("Unlocked.", "ok");
    loadAccounts().then(function () { renderDraft(); });
  });

  applyLock();
  renderDraft();
  load();
  if (state.key) loadAccounts().then(function () { renderDraft(); });
})();
`
