package main

const comboPageJS = `
(function () {
  "use strict";

  var PAGE = location.pathname.replace(/\/+$/, "");
  var API = PAGE.replace(/^\/v0\/resource\/plugins\/[^/]+/, "/v0/management") + "/api";
  var KEY_STORE = "cpamp-combos-management-key";
  var state = { combos: [], filter: "all", key: null };
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

  function targetRow(combo, t, i) {
    var label = t.label || t.model;
    var sub = t.provider ? t.provider : "resolved by model name";
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
    var id = "combo/" + c.name;
    var rows = c.targets.map(function (t, i) { return targetRow(c, t, i); }).join("");
    return '<section class="card" data-name="' + esc(c.name) + '">' +
      '<div class="combo-head"><div class="combo-id">' +
        '<span class="combo-name">' + esc(c.name) + "</span>" +
        '<span class="badge">priority</span>' +
        '<code class="combo-model">' + esc(id) + "</code>" +
      "</div>" +
      '<div class="combo-actions">' +
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

  function parseTargets(text) {
    return text.split("\n").map(function (l) { return l.trim(); }).filter(Boolean)
      .map(function (line) {
        var slash = line.indexOf("/");
        if (slash < 0) return { model: line };
        return { provider: line.slice(0, slash), model: line.slice(slash + 1) };
      });
  }

  $("c-submit").addEventListener("click", function () {
    var name = $("c-name").value.trim();
    var targets = parseTargets($("c-targets").value);
    if (!name) return setMsg("Name is required.", "err");
    if (!targets.length) return setMsg("Add at least one target.", "err");
    if (state.combos.some(function (c) { return c.name === name; })) {
      return setMsg("A combo named " + name + " already exists.", "err");
    }
    var next = state.combos.concat([{ name: name, description: $("c-desc").value.trim(), targets: targets }]);
    api("", { method: "POST", body: { name: name, description: $("c-desc").value.trim(), targets: targets } })
      .then(function () { return load(); })
      .then(function () {
        $("c-name").value = ""; $("c-desc").value = ""; $("c-targets").value = "";
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
  });

  applyLock();
  load();
})();
`
