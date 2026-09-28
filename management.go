package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const resourcePath = "/combos"

type managementRouteSet struct {
	Routes []managementRoute `json:"routes"`
}

type managementRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Menu        string `json:"menu,omitempty"`
	Description string `json:"description,omitempty"`
}

type managementResponsePayload struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func managementRouteSetResponse() managementRouteSet {
	return managementRouteSet{
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: resourcePath, Menu: "Combos", Description: "Named model fallback chains"},
			{Method: http.MethodGet, Path: resourcePath + "/api", Description: "List combos"},
			{Method: http.MethodPost, Path: resourcePath + "/api", Description: "Create combo"},
			{Method: http.MethodPut, Path: resourcePath + "/api", Description: "Replace combos"},
			{Method: http.MethodDelete, Path: resourcePath + "/api", Description: "Delete combo"},
		},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req rpcManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error()), nil
	}

	path := normalizePath(req.Path)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	logInfo("management.handle method=" + method + " rawPath=" + req.Path + " normalized=" + path)
	if method == "" {
		method = http.MethodGet
	}

	switch {
	case path == resourcePath && method == http.MethodGet:
		return serveCombosPage(req.ManagementRequest)
	case path == resourcePath+"/api" && method == http.MethodGet:
		return serveCombosAPI(req.ManagementRequest, nil)
	case path == resourcePath+"/api" && method == http.MethodPut:
		return serveCombosAPI(req.ManagementRequest, req.Body)
	case path == resourcePath+"/api" && method == http.MethodPost:
		return serveCombosCreate(req.ManagementRequest)
	case path == resourcePath+"/api" && method == http.MethodDelete:
		return serveCombosDelete(req.ManagementRequest)
	default:
		return okEnvelope(managementResponse(404, map[string]any{"error": "not found"}))
	}
}

func normalizePath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return resourcePath
	}
	if strings.HasPrefix(trimmed, "/v0/resource/plugins/") {
		rest := trimmed[len("/v0/resource/plugins/"):]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			trimmed = rest[slash:]
		} else {
			trimmed = "/"
		}
	} else if strings.HasPrefix(trimmed, "/v0/management/") {
		trimmed = trimmed[len("/v0/management/"):]
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	if len(trimmed) > 1 {
		trimmed = strings.TrimRight(trimmed, "/")
	}
	return trimmed
}

func managementResponse(status int, payload any) pluginapi.ManagementResponse {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{"error":"marshal failed"}`)
		status = 500
	}
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       raw,
	}
}

func serveCombosAPI(req pluginapi.ManagementRequest, body []byte) ([]byte, error) {
	if len(body) > 0 {
		var file comboFile
		if err := json.Unmarshal(body, &file); err != nil {
			return okEnvelope(managementResponse(400, map[string]any{"error": err.Error()}))
		}
		if err := runtime.replaceAll(file.Combos); err != nil {
			return okEnvelope(managementResponse(400, map[string]any{"error": err.Error()}))
		}
	}
	enabled, combos := runtime.snapshot()
	return okEnvelope(managementResponse(200, map[string]any{
		"enabled": enabled,
		"combos":  combos,
		"count":   len(combos),
	}))
}

func serveCombosCreate(req pluginapi.ManagementRequest) ([]byte, error) {
	var incoming combo
	if err := json.Unmarshal(req.Body, &incoming); err != nil {
		return okEnvelope(managementResponse(400, map[string]any{"error": err.Error()}))
	}
	_, existing := runtime.snapshot()
	now := time.Now().UTC()
	created := append([]combo(nil), existing...)
	created = append(created, combo{
		Name:        incoming.Name,
		Description: incoming.Description,
		Targets:     incoming.Targets,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	})
	if err := runtime.replaceAll(created); err != nil {
		return okEnvelope(managementResponse(400, map[string]any{"error": err.Error()}))
	}
	return okEnvelope(managementResponse(201, map[string]any{"ok": true, "name": incoming.Name}))
}

func serveCombosDelete(req pluginapi.ManagementRequest) ([]byte, error) {
	name := strings.TrimSpace(req.Query.Get("name"))
	if name == "" {
		return okEnvelope(managementResponse(400, map[string]any{"error": "name is required"}))
	}
	_, existing := runtime.snapshot()
	kept := make([]combo, 0, len(existing))
	found := false
	for _, c := range existing {
		if strings.EqualFold(c.Name, name) {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		return okEnvelope(managementResponse(404, map[string]any{"error": "combo not found: " + name}))
	}
	if err := runtime.replaceAll(kept); err != nil {
		return okEnvelope(managementResponse(400, map[string]any{"error": err.Error()}))
	}
	return okEnvelope(managementResponse(200, map[string]any{"ok": true}))
}

func serveCombosPage(req pluginapi.ManagementRequest) ([]byte, error) {
	html := renderCombosPage()
	return okEnvelope(pluginapi.ManagementResponse{
		StatusCode: 200,
		Headers: http.Header{
			"Content-Type":            []string{"text/html; charset=utf-8"},
			"Content-Security-Policy": []string{"default-src 'self' 'unsafe-inline' 'unsafe-eval'; connect-src *; frame-ancestors *"},
			"Cache-Control":           []string{"no-store"},
			"X-Content-Type-Options":  []string{"nosniff"},
		},
		Body: []byte(html),
	})
}

func renderCombosPage() string {
	_, combos := runtime.snapshot()
	sort.SliceStable(combos, func(i, j int) bool { return combos[i].Name < combos[j].Name })

	rows := make([]string, 0, len(combos))
	for _, c := range combos {
		targets := make([]string, 0, len(c.Targets))
		for i, t := range c.Targets {
			label := t.Label
			if label == "" {
				label = t.Model
			}
			if t.Provider != "" {
				label = t.Provider + "/" + label
			}
			arrow := "→"
			if i < len(c.Targets)-1 {
				arrow = "→"
			}
			targets = append(targets, fmt.Sprintf(
				`<span class="t"><span class="i">%d</span>%s <span class="dim">%s</span></span>`,
				i+1, htmlEscape(label), arrow))
		}
		rows = append(rows, fmt.Sprintf(
			`<div class="row" data-name="%s">
  <div class="head">
    <div><span class="name">%s</span><span class="mid">%s</span></div>
    <button class="del" onclick="del('%s')">Delete</button>
  </div>
  <div class="targets">%s</div>
</div>`, htmlEscape(c.Name), htmlEscape(c.Name), htmlEscape(comboModelID(c.Name)),
			htmlEscape(c.Name), strings.Join(targets, "")))
	}
	if len(rows) == 0 {
		rows = append(rows, `<div class="empty">No combos defined. Create one to expose a fallback chain as a single model.</div>`)
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Combos</title>
<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#16181d;--dim:#6b7280;--line:#e5e7eb;--card:#fafafa;--accent:#2563eb;--danger:#dc2626}
@media (prefers-color-scheme:dark){:root{--bg:#0f1115;--fg:#e8eaed;--dim:#9aa0a6;--line:#2a2e37;--card:#161a21;--accent:#60a5fa;--danger:#f87171}}
*{box-sizing:border-box}
body{margin:0;padding:20px;background:var(--bg);color:var(--fg);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif}
h1{font-size:17px;margin:0 0 4px}
.sub{color:var(--dim);font-size:12px;margin-bottom:18px}
.row{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:12px 14px;margin-bottom:10px}
.head{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:8px}
.name{font-weight:600}
.mid{color:var(--dim);font-size:12px;margin-left:8px;font-family:ui-monospace,monospace}
.targets{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
.t{border:1px solid var(--line);border-radius:6px;padding:3px 8px;font-size:12px;font-family:ui-monospace,monospace;background:var(--bg)}
.i{display:inline-block;min-width:16px;color:var(--accent);font-weight:700;margin-right:4px}
.dim{color:var(--dim)}
button{font:inherit;cursor:pointer;border-radius:6px;padding:4px 10px;border:1px solid var(--line);background:var(--bg);color:var(--fg)}
button:hover{border-color:var(--accent)}
.del{color:var(--danger)}
.empty{color:var(--dim);padding:20px;text-align:center;border:1px dashed var(--line);border-radius:8px}
form{margin-top:18px;padding-top:16px;border-top:1px solid var(--line)}
label{display:block;margin:10px 0 4px;font-size:12px;color:var(--dim);font-weight:600}
input{width:100%%;padding:7px 9px;border:1px solid var(--line);border-radius:6px;background:var(--bg);color:var(--fg);font:inherit}
textarea{min-height:88px;font-family:ui-monospace,monospace;font-size:12px}
.rowbtn{margin-top:12px;background:var(--accent);color:#fff;border-color:transparent;font-weight:600}
.msg{margin-top:10px;font-size:12px;min-height:16px}
.err{color:var(--danger)}
.ok{color:var(--accent)}
code{font-family:ui-monospace,monospace;background:var(--card);padding:1px 4px;border-radius:4px}
</style>
</head>
<body>
<h1>Combos</h1>
<div class="sub">Each combo is exposed as the model <code>%s/&lt;name&gt;</code> and resolves in listed order, falling through on failure. A new combo is callable immediately; it joins the model list after the next CPA config change or restart.</div>
<div id="list">%s</div>
<form onsubmit="return save(event)">
  <label>Name</label>
  <input id="name" placeholder="fast-fallback" autocomplete="off">
  <label>Targets (one per line, <code>provider/model</code> or bare <code>model</code>)</label>
  <textarea id="targets" placeholder="codex/gpt-5.6-sol&#10;zai/glm-5.3"></textarea>
  <button class="rowbtn" type="submit">Create combo</button>
  <div class="msg" id="msg"></div>
</form>
<script>
const API=%q;
function say(text,cls){const el=document.getElementById('msg');el.textContent=text;el.className='msg '+(cls||'');}
async function del(name){
  if(!confirm('Delete combo "'+name+'"?'))return;
  const r=await fetch(API+'?name='+encodeURIComponent(name),{method:'DELETE'});
  if(r.ok){location.reload();}else{say('Delete failed','err');}
}
function parseTargets(text){
  return text.split('\n').map(function(l){return l.trim();}).filter(Boolean).map(function(line){
    const slash=line.indexOf('/');
    if(slash<0)return {model:line};
    return {provider:line.slice(0,slash),model:line.slice(slash+1)};
  });
}
async function save(ev){
  ev.preventDefault();
  const name=document.getElementById('name').value.trim();
  const targets=parseTargets(document.getElementById('targets').value);
  if(!name){say('Name is required','err');return false;}
  if(!targets.length){say('At least one target is required','err');return false;}
  const r=await fetch(API,{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({name:name,targets:targets})});
  if(r.status===201){location.reload();return false;}
  let detail='';
  try{detail=(await r.json()).error||'';}catch(e){}
  say('Create failed: '+detail,'err');
  return false;
}
</script>
</body>
</html>`, comboNamespace, strings.Join(rows, ""), resourcePath+"/api")
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

var _ = strconv.Itoa
