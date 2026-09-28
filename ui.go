package main

import "net/http"

const comboPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Combos</title>
<link rel="stylesheet" href="?asset=css">
</head>
<body>
<div class="wrap">
  <header class="head">
    <div>
      <h1>Combos</h1>
      <p class="sub">A combo is an ordered list of models exposed as a single model.
      Requests resolve top to bottom and fall through to the next target on failure.
      A combo answers to its own name, exactly like a model.</p>
    </div>
    <div class="head-actions">
      <span id="lock" class="lock" data-state="locked">
        <span class="dot"></span><span id="lock-label">Read only</span>
      </span>
      <button id="unlock-btn" class="btn ghost" type="button">Unlock</button>
    </div>
  </header>

  <section class="tiles">
    <div class="tile"><span class="k">Combos</span><span class="v" id="stat-combos">0</span></div>
    <div class="tile"><span class="k">Targets</span><span class="v" id="stat-targets">0</span></div>
    <div class="tile"><span class="k">Providers</span><span class="v" id="stat-providers">0</span></div>
    <div class="tile"><span class="k">Strategy</span><span class="v">priority</span></div>
  </section>

  <div id="shadow" class="shadow"></div>
  <div id="filters" class="filters"></div>
  <main id="list" class="list"></main>

  <section class="card create">
    <h2>New combo</h2>
    <div class="field">
      <label for="c-name">Name</label>
      <input id="c-name" placeholder="fast-fallback" autocomplete="off" spellcheck="false">
    </div>
    <div class="field">
      <label for="c-desc">Description <span class="opt">optional</span></label>
      <input id="c-desc" placeholder="codex then glm" autocomplete="off">
    </div>
    <div class="field">
      <label>Targets</label>
      <div class="targets-empty" id="c-targets">No models picked yet.</div>
      <div class="create-actions">
        <button id="c-pick" class="btn" type="button" data-needs-key>Pick models</button>
        <span class="hint">Order decides priority. Top model is tried first.</span>
      </div>
    </div>
    <div class="create-actions">
      <button id="c-submit" class="btn primary" type="button" data-needs-key>Create combo</button>
      <span id="c-msg" class="msg"></span>
    </div>
  </section>
</div>

<dialog id="unlock">
  <form method="dialog">
    <h3>Unlock editing</h3>
    <p>Plugin pages do not inherit the panel session, so paste the panel admin key
    once. It is kept in this browser only and never leaves this host.</p>
    <input id="u-key" type="password" placeholder="panel admin key" autocomplete="off">
    <div class="dlg-actions">
      <button value="cancel" class="btn ghost" type="submit">Cancel</button>
      <button id="u-save" value="ok" class="btn primary" type="submit">Save</button>
    </div>
  </form>
</dialog>

__PICKER__

<script src="?asset=js"></script>
<script src="?asset=picker"></script>
</body>
</html>
`

const comboPageCSS = `
:root{
  --bg: var(--app-bg, #f5f7fa);
  --surface: var(--app-surface, #ffffff);
  --surface-2: var(--app-surface-muted, #f7f9fc);
  --border: var(--app-border, rgba(15,23,42,.09));
  --border-2: color-mix(in srgb, var(--border) 55%, transparent);
  --text: var(--app-text-primary, #2c3e50);
  --text-2: var(--app-text-regular, #5f6c7b);
  --muted: var(--app-text-muted, #8b95a6);
  --primary: var(--primary-color, #409eff);
  --primary-soft: color-mix(in srgb, var(--primary) 12%, transparent);
  --primary-line: color-mix(in srgb, var(--primary) 32%, transparent);
  --danger: var(--danger-color, #f56c6c);
  --ok: var(--success-color, #67c23a);
  --r: var(--app-radius-md, 12px);
  --shadow: 0 1px 2px rgba(15,23,42,.05), 0 4px 16px rgba(15,23,42,.05);
}
.theme-dark{
  --bg: var(--app-bg, #12161c);
  --surface: var(--app-surface, #1a1f27);
  --surface-2: var(--app-surface-muted, #161b22);
  --border: var(--app-border, rgba(255,255,255,.10));
  --text: var(--app-text-primary, #e6e9ef);
  --text-2: var(--app-text-regular, #b6bdc9);
  --muted: var(--app-text-muted, #8b95a6);
  --shadow: 0 1px 2px rgba(0,0,0,.4), 0 8px 24px rgba(0,0,0,.32);
}
@media (prefers-color-scheme: dark){
  html:not(.theme-light):not(.theme-dark){
    --bg:#12161c; --surface:#1a1f27; --surface-2:#161b22;
    --border:rgba(255,255,255,.10); --text:#e6e9ef; --text-2:#b6bdc9;
    --shadow: 0 1px 2px rgba(0,0,0,.4), 0 8px 24px rgba(0,0,0,.32);
  }
}
*{box-sizing:border-box}
html,body{margin:0;padding:0}
body{
  background:var(--bg); color:var(--text);
  font:14px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",sans-serif;
  -webkit-font-smoothing:antialiased;
}
.wrap{max-width:1080px;margin:0 auto;padding:28px 20px 56px}

.head{display:flex;flex-wrap:wrap;gap:16px;align-items:flex-start;justify-content:space-between;margin-bottom:22px}
h1{margin:0;font-size:22px;font-weight:650;letter-spacing:-.01em}
.sub{margin:5px 0 0;color:var(--muted);font-size:13px;max-width:64ch}
.head-actions{display:flex;align-items:center;gap:10px}
.lock{display:inline-flex;align-items:center;gap:6px;font-size:12px;color:var(--muted);
  background:var(--surface);border:1px solid var(--border);border-radius:999px;padding:5px 11px}
.lock .dot{width:7px;height:7px;border-radius:50%;background:var(--muted)}
.lock[data-state="unlocked"]{color:var(--ok);border-color:color-mix(in srgb,var(--ok) 40%,transparent)}
.lock[data-state="unlocked"] .dot{background:var(--ok)}

.tiles{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px;margin-bottom:18px}
@media(min-width:760px){.tiles{grid-template-columns:repeat(4,minmax(0,1fr))}}
.tile{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);padding:12px 14px}
.tile .k{display:block;font-size:10px;text-transform:uppercase;letter-spacing:.07em;color:var(--muted)}
.tile .v{display:block;margin-top:5px;font-size:17px;font-weight:650;word-break:break-all}

.filters{display:flex;flex-wrap:wrap;gap:6px;margin-bottom:16px;padding:4px;
  background:var(--surface);border:1px solid var(--border);border-radius:var(--r)}
.filters button{display:inline-flex;align-items:center;gap:7px;border:0;background:transparent;
  color:var(--text-2);font:inherit;font-size:13px;padding:7px 13px;border-radius:9px;cursor:pointer}
.filters button:hover{background:var(--surface-2);color:var(--text)}
.filters button[aria-pressed="true"]{background:var(--primary-soft);color:var(--primary);font-weight:600}
.filters .n{font-size:11px;opacity:.7}

.list{display:flex;flex-direction:column;gap:14px;margin-bottom:22px}
.card{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);
  padding:16px 18px;box-shadow:var(--shadow)}
.combo-head{display:flex;flex-wrap:wrap;gap:10px;align-items:center;justify-content:space-between}
.combo-id{display:flex;flex-wrap:wrap;align-items:baseline;gap:9px;min-width:0}
.combo-name{font-size:16px;font-weight:650;letter-spacing:-.01em}
.badge{font-size:10px;text-transform:uppercase;letter-spacing:.06em;font-weight:700;
  color:var(--primary);background:var(--primary-soft);border:1px solid var(--primary-line);
  border-radius:999px;padding:2px 8px}
.combo-model{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;color:var(--muted)}
.combo-desc{margin:7px 0 0;color:var(--text-2);font-size:13px}
.combo-actions{display:flex;gap:6px}

.targets{margin-top:13px;display:flex;flex-direction:column;gap:7px}
.target{display:flex;align-items:center;gap:10px;padding:8px 11px;border-radius:10px;
  background:var(--surface-2);border:1px solid var(--border-2)}
.target .idx{flex:0 0 20px;height:20px;display:grid;place-items:center;border-radius:6px;
  background:var(--primary-soft);color:var(--primary);font-size:11px;font-weight:700}
.target .meta{min-width:0;flex:1}
.target .m{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12.5px;
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.target .p{font-size:11px;color:var(--muted);margin-top:1px}
.acct{max-width:190px;font-size:11.5px;padding:4px 7px;border-radius:8px;
  border:1px solid var(--border);background:var(--surface-2);color:var(--text)}
.acct:disabled{opacity:.55}
.target .arrow{flex:0 0 auto;color:var(--muted);font-size:11px;padding:0 2px}
.target[data-first] .idx{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}

.shadow:not(:empty){display:flex;flex-direction:column;gap:6px;margin-bottom:14px}
.warn{padding:9px 12px;border-radius:10px;font-size:12.5px;
  background:color-mix(in srgb,var(--warning-color,#e6a23c) 12%,transparent);
  border:1px solid color-mix(in srgb,var(--warning-color,#e6a23c) 34%,transparent)}
.warn strong{font-weight:700}

.empty{padding:44px 20px;text-align:center;color:var(--muted);
  border:1px dashed var(--border);border-radius:var(--r);background:var(--surface)}

.btn{font:inherit;font-size:13px;padding:7px 13px;border-radius:9px;cursor:pointer;
  border:1px solid var(--border);background:var(--surface);color:var(--text)}
.btn:hover{border-color:var(--primary);color:var(--primary)}
.btn.primary{background:var(--primary);border-color:var(--primary);color:#fff;font-weight:600}
.btn.primary:hover{filter:brightness(1.06);color:#fff}
.btn.ghost{background:transparent}
.btn.sm{padding:5px 10px;font-size:12px}
.btn.danger:hover{border-color:var(--danger);color:var(--danger)}
.btn[disabled]{opacity:.45;cursor:not-allowed}

.field{margin-top:12px}
.field label{display:block;font-size:11px;text-transform:uppercase;letter-spacing:.06em;
  color:var(--muted);font-weight:700;margin-bottom:5px}
.field .opt{text-transform:none;letter-spacing:0;font-weight:400;opacity:.7}
input,textarea{width:100%;padding:9px 11px;border-radius:9px;border:1px solid var(--border);
  background:var(--bg);color:var(--text);font:inherit}
textarea{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12.5px;resize:vertical}
input:focus,textarea:focus{outline:2px solid var(--primary-line);border-color:var(--primary)}
.hint{margin:6px 0 0;font-size:12px;color:var(--muted)}
.hint code{background:var(--surface-2);padding:1px 5px;border-radius:4px;font-size:11px}
.create-actions{display:flex;align-items:center;gap:12px;margin-top:16px}
.field .create-actions{margin-top:8px}
.targets-empty{padding:14px;border:1px dashed var(--border);border-radius:10px;
  color:var(--muted);font-size:13px;text-align:center;background:var(--surface-2)}
__PICKER_CSS__
.msg{font-size:12.5px}
.msg.err{color:var(--danger)}
.msg.ok{color:var(--ok)}

dialog{border:1px solid var(--border);border-radius:var(--r);background:var(--surface);
  color:var(--text);padding:20px;max-width:420px;box-shadow:var(--shadow)}
dialog::backdrop{background:rgba(0,0,0,.35)}
dialog h3{margin:0 0 8px;font-size:16px}
dialog p{margin:0 0 12px;font-size:13px;color:var(--text-2)}
.dlg-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:14px}
`

func comboPageRoutes() []managementRoute {
	return []managementRoute{
		{Method: http.MethodGet, Path: resourcePath, Menu: "Combos", Description: "Named model fallback chains"},
		{Method: http.MethodGet, Path: resourcePath + "/api", Description: "List combos"},
		{Method: http.MethodPost, Path: resourcePath + "/api", Description: "Create combo"},
		{Method: http.MethodPut, Path: resourcePath + "/api", Description: "Replace combos"},
		{Method: http.MethodDelete, Path: resourcePath + "/api", Description: "Delete combo"},
		{Method: http.MethodGet, Path: resourcePath + "/api/accounts", Description: "List credential accounts"},
	}
}
