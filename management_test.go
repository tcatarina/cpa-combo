package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// useTempStore points the runtime at a scratch file so a test that writes
// combos never touches the store sitting in the working directory.
func useTempStore(t *testing.T) {
	t.Helper()
	prev := runtime.currentPath()
	runtime.mu.Lock()
	runtime.path = filepath.Join(t.TempDir(), "combos.json")
	runtime.mu.Unlock()
	t.Cleanup(func() {
		runtime.mu.Lock()
		runtime.path = prev
		runtime.mu.Unlock()
	})
}

// callManagement runs the handler and unwraps the reply envelope.
func callManagement(t *testing.T, req pluginapi.ManagementRequest, body []byte) (int, string) {
	t.Helper()
	out, err := serveCombosAPI(req, body)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			StatusCode int `json:"StatusCode"`
			Body       []byte
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unreadable envelope %s: %v", out, err)
	}
	if !env.OK {
		t.Fatalf("handler reported failure: %s", out)
	}
	return env.Result.StatusCode, string(env.Result.Body)
}

// A write that carries no body used to be answered with 200 and the unchanged
// state, so a client that dropped its payload looked like it had saved.
func TestBodylessPutIsRejectedInsteadOfReportedAsSaved(t *testing.T) {
	status, body := callManagement(t, pluginapi.ManagementRequest{Method: http.MethodPut}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("bodyless PUT answered %d, want 400", status)
	}
	if !strings.Contains(body, "PUT requires a body") {
		t.Fatalf("the error should name the problem, got %s", body)
	}
}

func TestPutWithBodyStillReplaces(t *testing.T) {
	useTempStore(t)
	status, body := callManagement(t, pluginapi.ManagementRequest{Method: http.MethodPut},
		[]byte(`{"version":1,"combos":[{"name":"solo","targets":[{"model":"glm"}]}]}`))
	if status != http.StatusOK {
		t.Fatalf("PUT with a body answered %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"name":"solo"`) {
		t.Fatalf("the replacement was not reflected, got %s", body)
	}
}

func TestGetWithoutABodyStillReads(t *testing.T) {
	status, body := callManagement(t, pluginapi.ManagementRequest{Method: http.MethodGet}, nil)
	if status != http.StatusOK {
		t.Fatalf("GET answered %d, want 200: %s", status, body)
	}
}

// The picker shares one helper for reading and writing. It dropped options.body
// on the floor, so every "add a model to an existing combo" wrote nothing while
// reporting success.
func TestPickerFetchForwardsTheRequestBody(t *testing.T) {
	start := strings.Index(comboPickerJS, "function jfetch(")
	if start < 0 {
		t.Fatal("jfetch is missing from the picker script")
	}
	end := strings.Index(comboPickerJS[start:], "\n  }")
	if end < 0 {
		t.Fatal("could not find the end of jfetch")
	}
	fetchCall := comboPickerJS[start : start+end]
	if !strings.Contains(fetchCall, "options.body") {
		t.Error("jfetch never reads options.body, so every picker write sends no payload")
	}
	if !strings.Contains(fetchCall, "body:") {
		t.Error("jfetch does not pass a body to fetch")
	}
}

// A saved combo has no way to drop a model without the page offering one.
func TestSavedComboRowsOfferRemove(t *testing.T) {
	if !strings.Contains(comboPageJS, `data-rm="' + i`) {
		t.Error("a saved combo's target rows have no remove control")
	}
	if !strings.Contains(comboPageJS, "Remove from combo") {
		t.Error("the remove control has no title")
	}
}

// A combo with no targets is discarded by normalizeCombo, so removing the last
// one would delete the combo instead of a model.
func TestLastTargetIsProtected(t *testing.T) {
	if normalizeCombo(combo{Name: "solo", Targets: nil}) != nil {
		t.Error("a combo with no targets must be dropped, or the last-target guard is not needed")
	}
	if !strings.Contains(comboPageJS, "combo.targets.length === 1") {
		t.Error("the remove control is not disabled for a single-target combo")
	}
}
