package main

const comboPickerHTML = `
<dialog id="picker" class="picker">
  <div class="picker-head">
    <div>
      <h3 id="picker-title">Add models</h3>
      <p class="picker-sub" id="picker-sub">Search the models this host can reach.</p>
    </div>
    <button class="btn ghost sm" id="picker-close" type="button">Close</button>
  </div>

  <input id="picker-search" class="picker-search" placeholder="Search models" autocomplete="off" spellcheck="false">

  <div class="presets" id="picker-presets"></div>

  <div class="picker-bar">
    <span class="picker-count" id="picker-count">0 models</span>
    <button class="btn sm" id="picker-addall" type="button">Add all</button>
  </div>

  <div class="picker-list" id="picker-list"></div>

  <div class="picker-foot">
    <span class="picker-hint">Order in the combo decides priority. Reorder after adding.</span>
    <button class="btn primary" id="picker-done" type="button">Done</button>
  </div>
</dialog>
`

const comboPickerCSS = `
.picker{
  width:min(680px,94vw); max-height:min(760px,88vh);
  padding:0; border:1px solid var(--border); border-radius:var(--r);
  background:var(--surface); color:var(--text); box-shadow:var(--shadow);
  overflow:hidden;
}
.picker::backdrop{background:rgba(0,0,0,.35)}
.picker[open]{display:flex;flex-direction:column}
.picker-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;
  padding:16px 18px 12px;border-bottom:1px solid var(--border)}
.picker h3{margin:0;font-size:15px;font-weight:650}
.picker-sub{margin:3px 0 0;font-size:12px;color:var(--muted)}
.picker-search{margin:12px 18px 0}
.presets{display:flex;flex-wrap:wrap;gap:6px;padding:10px 18px 0}
.presets button{border:1px solid var(--border);background:var(--surface-2);color:var(--text-2);
  font:inherit;font-size:12px;padding:4px 10px;border-radius:999px;cursor:pointer}
.presets button:hover{border-color:var(--primary);color:var(--primary)}
.picker-bar{display:flex;align-items:center;justify-content:space-between;gap:10px;
  padding:12px 18px 8px}
.picker-count{font-size:11px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted);font-weight:700}
.picker-list{flex:1;overflow-y:auto;padding:0 12px 8px;min-height:120px}
.prow{display:flex;align-items:center;gap:10px;padding:8px 10px;border-radius:9px;
  border:1px solid transparent}
.prow:hover{background:var(--surface-2);border-color:var(--border-2)}
.prow .pmeta{min-width:0;flex:1}
.prow .pm{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.prow .pp{font-size:11px;color:var(--muted);margin-top:1px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.prow .pact{flex:0 0 auto}
.pgroup{padding:12px 10px 5px;font-size:10px;text-transform:uppercase;letter-spacing:.07em;
  color:var(--muted);font-weight:700}
.pempty{padding:34px 12px;text-align:center;color:var(--muted);font-size:13px}
.pfoot,.picker-foot{display:flex;align-items:center;justify-content:space-between;gap:12px;
  padding:12px 18px;border-top:1px solid var(--border)}
.picker-hint{font-size:11.5px;color:var(--muted)}
`

const comboPickerJS = `
(function () {
  "use strict";

  var PAGE = location.pathname.replace(/\/+$/, "");
  var MARK = "/v0/resource/plugins/";
  var CUT = location.pathname.indexOf(MARK);
  var PREFIX = CUT >= 0 ? location.pathname.slice(0, CUT) : "";
  var API = PREFIX + "/v0/management/combos/api";
  var MGMT = PREFIX + "/v0/management";
  var PRESETS = ["opus", "sonnet", "deepseek", "kimi", "qwen", "gemini", "flash", "gpt", "glm", "grok"];
  var state = { catalog: [], query: "", apiKey: null, busy: false };

  var $ = function (id) { return document.getElementById(id); };
  var dlg = $("picker");

  function esc(v) {
    return String(v == null ? "" : v)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  function key() { var p = window.__comboState || {}; return p.key || null; }

  function jfetch(url, options) {
    options = options || {};
    var h = Object.assign({ "Content-Type": "application/json" }, options.headers || {});
    if (key()) h.Authorization = "Bearer " + key();
    return fetch(url, {
      method: options.method || "GET",
      headers: h,
      body: options.body ? JSON.stringify(options.body) : undefined
    }).then(function (r) {
      return r.text().then(function (t) {
        var d = null;
        try { d = t ? JSON.parse(t) : null; } catch (e) {}
        if (!r.ok) throw new Error((d && (d.error || d.message)) || ("HTTP " + r.status));
        return d;
      });
    });
  }

  function loadCatalog() {
    if (state.catalog.length) return Promise.resolve();
    $("picker-list").innerHTML = '<div class="pempty">Loading models...</div>';
    return jfetch(MGMT + "/config")
      .then(function (d) {
        var cfg = (d && (d.config || d)) || {};
        var keys = cfg["api-keys"] || cfg.access?.["api-keys"] || [];
        if (!keys.length) throw new Error("no api key available to read the model catalog");
        state.apiKey = keys[0];
        return fetch(MGMT.replace(/\/v0\/management$/, "") + "/../v1/models", {
          headers: { Authorization: "Bearer " + state.apiKey }
        }).then(function (r) {
          if (!r.ok) throw new Error("model catalog unavailable (HTTP " + r.status + ")");
          return r.json();
        });
      })
      .then(function (d) {
        state.catalog = (d && d.data) || [];
        state.apiKey = null;
        window.__comboCatalog = state.catalog;
        render();
      })
      .catch(function (e) {
        $("picker-list").innerHTML = '<div class="pempty">' + esc(e.message) +
          "<br>Unlock editing to browse models.</div>";
        $("picker-count").textContent = "0 models";
      });
  }

  function groupOf(m) {
    var id = String(m.id || "");
    var slash = id.indexOf("/");
    return slash > 0 ? id.slice(0, slash) : (m.owned_by || "host");
  }

  function matches(m) {
    if (!state.query) return true;
    var q = state.query.toLowerCase();
    return String(m.id || "").toLowerCase().indexOf(q) >= 0 ||
      String(m.owned_by || "").toLowerCase().indexOf(q) >= 0;
  }

  function pageState() { return window.__comboState || { combos: [], draft: [] }; }

  function isNew() { return !window.__comboEditing; }

  function currentTargets() {
    var p = pageState();
    if (isNew()) return p.draft || [];
    var name = window.__comboEditing;
    var found = (p.combos || []).filter(function (c) { return c.name === name; })[0];
    return (found && found.targets) || [];
  }

  function alreadyIn(provider, model) {
    return currentTargets().some(function (t) {
      return String(t.model) === model && String(t.provider || "") === String(provider || "");
    });
  }

  function filtered() {
    return state.catalog.filter(matches).map(function (m) {
      var id = String(m.id || "");
      var slash = id.indexOf("/");
      return {
        model: id,
        provider: slash > 0 ? id.slice(0, slash) : "",
        display: m.display_name || m.id || id
      };
    });
  }

  function render() {
    $("picker-presets").innerHTML = PRESETS.map(function (p) {
      return '<button type="button" data-q="' + p + '">' + p + "</button>";
    }).join("");
    Array.prototype.forEach.call($("picker-presets").querySelectorAll("button"), function (b) {
      b.addEventListener("click", function () {
        state.query = b.dataset.q;
        $("picker-search").value = b.dataset.q;
        render();
      });
    });

    var rows = filtered();
    $("picker-count").textContent = rows.length + (rows.length === 1 ? " model" : " models");
    if (!rows.length) {
      $("picker-list").innerHTML = '<div class="pempty">No models match.</div>';
      return;
    }
    var lastGroup = null, html = "";
    rows.forEach(function (r) {
      if (r.provider !== lastGroup) {
        lastGroup = r.provider;
        html += '<div class="pgroup">' + esc(r.provider || "host") + "</div>";
      }
      var add = alreadyIn(r.provider, r.model);
      html += '<div class="prow"><span class="pmeta"><span class="pm">' + esc(r.display) +
        "</span>" + (r.model !== r.display ? '<span class="pp">' + esc(r.model) + "</span>" : "") +
        '</span><span class="pact"><button class="btn sm" data-add="' + esc(r.model) +
        '" data-provider="' + esc(r.provider) + '"' + (add ? " disabled" : "") + ">" +
        (add ? "Added" : "Add") + "</button></span></div>";
    });
    $("picker-list").innerHTML = html;
    Array.prototype.forEach.call($("picker-list").querySelectorAll("[data-add]"), function (b) {
      b.addEventListener("click", function () { addTarget(b.dataset.provider, b.dataset.add, b); });
    });
  }

  function addTarget(provider, model, btn) {
    if (state.busy) return;
    if (alreadyIn(provider, model)) return;
    if (btn) btn.disabled = true;
    var p = pageState();
    if (isNew()) {
      p.draft = (p.draft || []).concat([{ provider: provider || "", model: model }]);
      window.__comboRenderDraft();
      render();
      return;
    }
    state.busy = true;
    var name = window.__comboEditing;
    var targets = currentTargets().concat([{ provider: provider || "", model: model }]);
    jfetch(API, {
      method: "PUT",
      body: {
        version: 1,
        combos: (p.combos || []).map(function (c) {
          return {
            name: c.name,
            description: c.description,
            targets: c.name === name ? targets : c.targets
          };
        })
      }
    }).then(function () {
      return window.__comboReload();
    }).then(function () {
      render();
    }).catch(function (e) {
      if (btn) btn.disabled = false;
      $("picker-list").insertAdjacentHTML("afterbegin",
        '<div class="pempty">' + esc(e.message) + "</div>");
    }).then(function () { state.busy = false; });
  }

  $("picker-search").addEventListener("input", function () {
    state.query = $("picker-search").value.trim();
    render();
  });
  $("picker-addall").addEventListener("click", function () {
    var rows = filtered();
    if (!rows.length) return;
    var p = pageState();
    var fresh = rows.filter(function (r) { return !alreadyIn(r.provider, r.model); })
      .map(function (r) { return { provider: r.provider || "", model: r.model }; });
    if (!fresh.length) return;
    if (isNew()) {
      p.draft = (p.draft || []).concat(fresh);
      window.__comboRenderDraft();
      render();
      return;
    }
    state.busy = true;
    var name = window.__comboEditing;
    var targets = currentTargets().concat(fresh);
    jfetch(API, {
      method: "PUT",
      body: {
        version: 1,
        combos: (p.combos || []).map(function (c) {
          return {
            name: c.name,
            description: c.description,
            targets: c.name === name ? targets : c.targets
          };
        })
      }
    }).then(function () { return window.__comboReload(); })
      .then(function () { render(); })
      .catch(function (e) {
        $("picker-list").insertAdjacentHTML("afterbegin", '<div class="pempty">' + esc(e.message) + "</div>");
      })
      .then(function () { state.busy = false; });
  });
  $("picker-close").addEventListener("click", function () { dlg.close(); });
  $("picker-done").addEventListener("click", function () { dlg.close(); });

  window.__comboOpenPicker = function () {
    dlg.showModal();
    $("picker-search").value = "";
    state.query = "";
    loadCatalog();
  };
})();
`
