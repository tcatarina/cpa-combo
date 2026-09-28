package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const resourcePath = "/combos"

type managementRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Menu        string `json:"menu,omitempty"`
	Description string `json:"description,omitempty"`
}

type managementRouteSet struct {
	Routes []managementRoute `json:"routes"`
}

func managementRouteSetResponse() managementRouteSet {
	return managementRouteSet{Routes: comboPageRoutes()}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req rpcManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error()), nil
	}

	path := normalizePath(req.Path)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	apiPath := resourcePath + "/api"

	switch {
	case path == resourcePath && method == http.MethodGet:
		switch req.Query.Get("asset") {
		case "css":
			return okEnvelope(assetResponse("text/css; charset=utf-8", comboPageCSS+comboPickerCSS))
		case "js":
			return okEnvelope(assetResponse("text/javascript; charset=utf-8",
				strings.ReplaceAll(comboPageJS, "__BASE__", resourcePath)))
		case "picker.js":
			return okEnvelope(assetResponse("text/javascript; charset=utf-8", comboPickerJS))
		case "data":
			_, combos := runtime.snapshot()
			return okEnvelope(jsonResponse(200, map[string]any{"combos": combos}))
		}
		html := strings.ReplaceAll(comboPageHTML, "__PICKER__", comboPickerHTML)
		html = strings.ReplaceAll(html, "__PICKER_CSS__", comboPickerCSS)
		html = strings.ReplaceAll(html, "src=\"?asset=picker\"", "src=\"?asset=picker.js\"")
		return okEnvelope(assetResponse("text/html; charset=utf-8", html))
	case path == apiPath && method == http.MethodGet:
		return serveCombosAPI(req.ManagementRequest, nil)
	case path == apiPath && method == http.MethodPut:
		return serveCombosAPI(req.ManagementRequest, req.Body)
	case path == apiPath && method == http.MethodPost:
		return serveCombosCreate(req.ManagementRequest)
	case path == apiPath && method == http.MethodDelete:
		return serveCombosDelete(req.ManagementRequest)
	default:
		return okEnvelope(jsonResponse(404, map[string]any{"error": "not found"}))
	}
}

func assetResponse(contentType, body string) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: 200,
		Headers: http.Header{
			"Content-Type":           []string{contentType},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
		},
		Body: []byte(body),
	}
}

func jsonResponse(status int, payload any) pluginapi.ManagementResponse {
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

func serveCombosAPI(req pluginapi.ManagementRequest, body []byte) ([]byte, error) {
	if len(body) > 0 {
		var file comboFile
		if err := json.Unmarshal(body, &file); err != nil {
			return okEnvelope(jsonResponse(400, map[string]any{"error": err.Error()}))
		}
		if err := runtime.replaceAll(file.Combos); err != nil {
			return okEnvelope(jsonResponse(400, map[string]any{"error": err.Error()}))
		}
	}
	enabled, combos := runtime.snapshot()
	return okEnvelope(jsonResponse(200, map[string]any{
		"enabled": enabled,
		"combos":  combos,
		"count":   len(combos),
	}))
}

func serveCombosCreate(req pluginapi.ManagementRequest) ([]byte, error) {
	var incoming combo
	if len(req.Body) == 0 {
		return okEnvelope(jsonResponse(400, map[string]any{"error": "body is required"}))
	}
	if err := json.Unmarshal(req.Body, &incoming); err != nil {
		return okEnvelope(jsonResponse(400, map[string]any{"error": err.Error()}))
	}
	if _, exists := runtime.find(incoming.Name); exists {
		return okEnvelope(jsonResponse(409, map[string]any{"error": "combo already exists: " + incoming.Name}))
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
		return okEnvelope(jsonResponse(400, map[string]any{"error": err.Error()}))
	}
	return okEnvelope(jsonResponse(201, map[string]any{"ok": true, "name": incoming.Name}))
}

func serveCombosDelete(req pluginapi.ManagementRequest) ([]byte, error) {
	name := strings.TrimSpace(req.Query.Get("name"))
	if name == "" {
		return okEnvelope(jsonResponse(400, map[string]any{"error": "name is required"}))
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
		return okEnvelope(jsonResponse(404, map[string]any{"error": "combo not found: " + name}))
	}
	if err := runtime.replaceAll(kept); err != nil {
		return okEnvelope(jsonResponse(400, map[string]any{"error": err.Error()}))
	}
	return okEnvelope(jsonResponse(200, map[string]any{"ok": true}))
}
