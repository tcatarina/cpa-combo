package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func handleModelRoute(raw []byte) ([]byte, error) {
	var req rpcModelRouteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error()), nil
	}
	enabled, combos := runtime.snapshot()
	logInfo("model.route requested=" + req.RequestedModel + " source=" + req.SourceFormat +
		" enabled=" + strconv.FormatBool(enabled) + " combos=" + strconv.Itoa(len(combos)) +
		" availableProviders=" + strings.Join(req.AvailableProviders, ","))
	if !enabled || len(combos) == 0 {
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false})
	}
	providers.set(req.AvailableProviders)
	name, ok := splitComboModel(req.RequestedModel)
	if !ok {
		if combo, found := runtime.find(req.RequestedModel); found {
			return okEnvelope(pluginapi.ModelRouteResponse{
				Handled:    true,
				TargetKind: pluginapi.ModelRouteTargetSelf,
				Reason:     "combo:" + combo.Name,
			})
		}
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false})
	}
	if _, found := runtime.find(name); !found {
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false})
	}
	return okEnvelope(pluginapi.ModelRouteResponse{
		Handled:    true,
		TargetKind: pluginapi.ModelRouteTargetSelf,
		Reason:     "combo:" + name,
	})
}

func handleModelRegister(raw []byte) ([]byte, error) {
	var req pluginapi.ModelRegistrationRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return errorEnvelope("bad_request", err.Error()), nil
		}
	}
	enabled, combos := runtime.snapshot()
	models := make([]pluginapi.ModelInfo, 0, len(combos))
	if enabled {
		now := time.Now().Unix()
		for _, c := range combos {
			models = append(models, pluginapi.ModelInfo{
				ID:          c.Name,
				Object:      "model",
				Created:     now,
				OwnedBy:     comboNamespace,
				Type:        "model",
				DisplayName: c.Name,
				Name:        c.Name,
				Description: comboDescription(c),
			})
		}
	}
	return okEnvelope(pluginapi.ModelRegistrationResponse{
		Provider: comboNamespace,
		Models:   models,
	})
}

func comboDescription(c combo) string {
	parts := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		label := t.Label
		if label == "" {
			label = t.Model
		}
		if t.Provider != "" {
			label = t.Provider + "/" + label
		}
		parts = append(parts, label)
	}
	desc := "priority: " + strings.Join(parts, " -> ")
	if c.Description != "" {
		desc = c.Description + " (" + desc + ")"
	}
	return desc
}

func handleExecute(raw []byte, stream bool) ([]byte, error) {
	var req rpcExecutorRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error()), nil
	}

	enabled, _ := runtime.snapshot()
	logInfo("executor.execute model=" + req.Model + " format=" + req.Format +
		" source=" + req.SourceFormat + " payload=" + strconv.Itoa(len(req.Payload)) +
		" original=" + strconv.Itoa(len(req.OriginalRequest)) + " enabled=" + strconv.FormatBool(enabled))
	if !enabled {
		return errorEnvelope("disabled", "combos plugin is disabled"), nil
	}

	name, ok := splitComboModel(req.Model)
	if !ok {
		if _, found := runtime.find(req.Model); found {
			name = normalizeModelName(req.Model)
		} else {
			return errorEnvelope("no_target", "combo not found: "+req.Model), nil
		}
	}

	c, found := runtime.find(name)
	if !found {
		return errorEnvelope("no_target", "combo not found: "+name), nil
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = req.OriginalRequest
	}

	entry := entryProtocol(req.SourceFormat, req.Format)
	body, headers, err := runCombo(context.Background(), c, payload, entry, stream, req.HostCallbackID)
	if err != nil {
		return errorEnvelope("combo_failed", err.Error()), nil
	}

	if stream {
		return okEnvelope(executorStreamResponse{
			Headers: headers,
			Chunks:  []pluginapi.ExecutorStreamChunk{{Payload: body}},
		})
	}
	return okEnvelope(pluginapi.ExecutorResponse{Payload: body, Headers: headers})
}

func entryProtocol(sourceFormat, format string) string {
	for _, candidate := range []string{sourceFormat, format} {
		v := strings.ToLower(strings.TrimSpace(candidate))
		switch v {
		case "openai", "claude", "gemini", "responses":
			return v
		}
	}
	return "openai"
}

type comboAttemptFailure struct {
	target comboTarget
	err    error
	status int
}

func runCombo(ctx context.Context, c combo, body []byte, entry string, stream bool, hostCallbackID string) ([]byte, http.Header, error) {
	if len(c.Targets) == 0 {
		return nil, nil, errors.New("combo " + c.Name + " has no targets")
	}
	var failures []comboAttemptFailure
	var lastHeaders http.Header
	for _, target := range c.Targets {
		payload, headers, err := hostModelExecute(ctx, hostCallbackID, target, body, entry, stream)
		if err == nil {
			return payload, headers, nil
		}
		lastHeaders = headers
		status := statusFromError(err)
		logWarn("combo " + c.Name + ": target " + describeTarget(target) + " failed (" +
			err.Error() + "); trying next target")
		failures = append(failures, comboAttemptFailure{target: target, err: err, status: status})
	}
	parts := make([]string, 0, len(failures))
	for _, f := range failures {
		parts = append(parts, describeTarget(f.target)+": "+f.err.Error())
	}
	return nil, lastHeaders, fmt.Errorf("combo %s exhausted all %d targets (%s)",
		c.Name, len(c.Targets), strings.Join(parts, "; "))
}

func describeTarget(t comboTarget) string {
	if t.Provider == "" {
		return t.Model
	}
	return t.Provider + "/" + t.Model
}

type hostModelExecutionRequest struct {
	pluginapi.HostModelExecutionRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

func hostModelExecute(ctx context.Context, hostCallbackID string, target comboTarget, body []byte, entry string, stream bool) ([]byte, http.Header, error) {
	method := pluginabi.MethodHostModelExecute
	if stream {
		method = pluginabi.MethodHostModelExecuteStream
	}
	req := hostModelExecutionRequest{
		HostModelExecutionRequest: pluginapi.HostModelExecutionRequest{
			EntryProtocol:  entry,
			ExitProtocol:   entry,
			Model:          target.Model,
			Stream:         stream,
			Body:           body,
			ForcedProvider: providers.resolve(target.Provider),
		},
		HostCallbackID: hostCallbackID,
	}
	if stream {
		return drainHostStream(req)
	}
	var resp pluginapi.HostModelExecutionResponse
	if err := callHostResult(method, req, &resp); err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, cloneHeader(resp.Headers), fmt.Errorf("host model status %d: %s",
			resp.StatusCode, snippet(resp.Body))
	}
	return resp.Body, cloneHeader(resp.Headers), nil
}

func drainHostStream(req hostModelExecutionRequest) ([]byte, http.Header, error) {
	var resp pluginapi.HostModelStreamResponse
	if err := callHostResult(pluginabi.MethodHostModelExecuteStream, req, &resp); err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, cloneHeader(resp.Headers), fmt.Errorf("host stream status %d", resp.StatusCode)
	}
	streamID := strings.TrimSpace(resp.StreamID)
	if streamID == "" {
		return nil, cloneHeader(resp.Headers), errors.New("host stream returned no stream id")
	}
	var out bytes.Buffer
	for {
		var read pluginapi.HostModelStreamReadResponse
		if err := callHostResult(pluginabi.MethodHostModelStreamRead,
			pluginapi.HostModelStreamReadRequest{StreamID: streamID}, &read); err != nil {
			return nil, nil, err
		}
		if len(read.Payload) > 0 {
			out.Write(read.Payload)
		}
		if read.Done {
			_ = callHostResult(pluginabi.MethodHostModelStreamClose,
				pluginapi.HostModelStreamCloseRequest{StreamID: streamID}, nil)
			return out.Bytes(), cloneHeader(resp.Headers), nil
		}
	}
}

func streamIDFromResponse(raw json.RawMessage) string {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	for _, key := range []string{"StreamID", "stream_id", "StreamId"} {
		if v, ok := probe[key].(string); ok {
			return v
		}
	}
	return ""
}

func statusFromError(err error) int {
	var he *hostError
	if errors.As(err, &he) {
		return he.status
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func snippet(body []byte) string {
	const maxSnippet = 400
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) > maxSnippet {
		return trimmed[:maxSnippet] + "..."
	}
	return trimmed
}

func cloneHeader(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	out := make(http.Header, len(headers))
	for k, v := range headers {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func logInfo(message string) {
	if !traceOn {
		return
	}
	_, _ = callHost(pluginabi.MethodHostLog, map[string]any{
		"level":   "info",
		"message": "combos: " + message,
	})
}

func logWarn(message string) {
	_, _ = callHost(pluginabi.MethodHostLog, map[string]any{
		"level":   "warn",
		"message": "combos: " + message,
	})
}
