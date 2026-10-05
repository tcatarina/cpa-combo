package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestNormalizeComboKeepsSameModelOnDifferentAccounts(t *testing.T) {
	in := combo{
		Name: "smart",
		Targets: []comboTarget{
			{Provider: "glm", Model: "glm-5", AuthID: "acct-a", Account: "alice"},
			{Provider: "glm", Model: "glm-5", AuthID: "acct-b", Account: "bob"},
			{Provider: "glm", Model: "glm-5", AuthID: "acct-b", Account: "bob"},
			{Provider: "glm", Model: "glm-5"},
			{Provider: "glm", Model: "glm-5"},
		},
	}
	out := normalizeCombo(in)
	if out == nil {
		t.Fatal("combo was dropped")
	}
	if len(out.Targets) != 3 {
		t.Fatalf("want 3 targets (a, b, unpinned), got %d", len(out.Targets))
	}
	if out.Targets[0].AuthID != "acct-a" || out.Targets[1].AuthID != "acct-b" {
		t.Fatalf("accounts not preserved: %+v", out.Targets)
	}
	if out.Targets[1].Account != "bob" {
		t.Fatalf("account label not preserved: %+v", out.Targets[1])
	}
	if out.Targets[2].AuthID != "" {
		t.Fatalf("unpinned target gained an auth id: %+v", out.Targets[2])
	}
}

// normalizeCombo rebuilds the struct field by field, so a context window set
// in the file used to be dropped on the next load and never advertised.
func TestNormalizeComboKeepsContextLength(t *testing.T) {
	out := normalizeCombo(combo{
		Name:          "sized",
		ContextLength: 200000,
		Targets:       []comboTarget{{Provider: "glm", Model: "glm-5"}},
	})
	if out == nil || out.ContextLength != 200000 {
		t.Fatalf("context length lost on load: %+v", out)
	}
}

// A negative window is not a real limit and must never reach /v1/models.
func TestNormalizeComboDropsNegativeContextLength(t *testing.T) {
	out := normalizeCombo(combo{
		Name:          "bad",
		ContextLength: -1,
		Targets:       []comboTarget{{Provider: "glm", Model: "glm-5"}},
	})
	if out == nil || out.ContextLength != 0 {
		t.Fatalf("negative context length survived: %+v", out)
	}
}

func TestModelRegisterAdvertisesContextLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, storeFileName)
	body, err := json.Marshal(comboFile{Version: 1, Combos: []combo{
		{Name: "sized", ContextLength: 200000, Targets: []comboTarget{{Model: "glm-5"}}},
		{Name: "plain", Targets: []comboTarget{{Model: "glm-5"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	state := newRuntimeState()
	state.path = path
	if err := state.reload(); err != nil {
		t.Fatal(err)
	}
	previous := runtime
	t.Cleanup(func() { runtime = previous })
	runtime = state

	raw, err := handleModelRegister(nil)
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Models []pluginapi.ModelInfo `json:"models"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, m := range resp.Result.Models {
		got[m.ID] = m.ContextLength
	}
	if got["sized"] != 200000 {
		t.Fatalf("sized combo advertised %d, want 200000", got["sized"])
	}
	if got["plain"] != 0 {
		t.Fatalf("unset combo advertised %d, want 0", got["plain"])
	}
}

func TestNormalizeComboStillDedupsIdenticalTargets(t *testing.T) {
	in := combo{
		Name: "dup",
		Targets: []comboTarget{
			{Provider: "glm", Model: "glm-5"},
			{Provider: "glm", Model: "glm-5"},
			{Provider: "codex", Model: "gpt-5"},
		},
	}
	out := normalizeCombo(in)
	if out == nil || len(out.Targets) != 2 {
		t.Fatalf("want 2 targets, got %+v", out)
	}
}

func TestSplitComboModel(t *testing.T) {
	if name, ok := splitComboModel("combo/smart"); !ok || name != "smart" {
		t.Fatalf("combo/smart: got %q %v", name, ok)
	}
	if _, ok := splitComboModel("smart"); ok {
		t.Fatal("bare name should not be treated as a combo prefix")
	}
	if _, ok := splitComboModel("combo/"); ok {
		t.Fatal("combo/ with empty name should not resolve")
	}
}

func TestDescribeTargetIncludesAccount(t *testing.T) {
	got := describeTarget(comboTarget{Provider: "glm", Model: "glm-5", Account: "alice"})
	if got != "glm/glm-5 @ alice" {
		t.Fatalf("got %q", got)
	}
	if got := describeTarget(comboTarget{Model: "glm-5"}); got != "glm-5" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRejectsVendorNamesThatAreNotProviders(t *testing.T) {
	c := &providerCache{}
	c.set([]string{"claude", "codex", "openai-compatibility-glm"})

	if got := c.resolve("claude"); got != "claude" {
		t.Errorf("exact provider key must resolve, got %q", got)
	}
	// "anthropic" is the model catalog's owned_by value, not a provider key.
	// Returning it verbatim pins execution to a provider that does not exist.
	if got := c.resolve("anthropic"); got != "" {
		t.Errorf("vendor name must not be pinned, got %q", got)
	}
	if got := c.resolve("zhipu"); got != "" {
		t.Errorf("unknown vendor name must not be pinned, got %q", got)
	}
	if got := c.resolve("glm"); got != "openai-compatibility-glm" {
		t.Errorf("suffix match expected, got %q", got)
	}
	if got := c.resolve(""); got != "" {
		t.Errorf("empty must stay empty, got %q", got)
	}
}

func TestResolveKeepsIntentWhenProviderListIsCold(t *testing.T) {
	c := &providerCache{}
	if got := c.resolve("codex"); got != "codex" {
		t.Errorf("with no known providers, keep the caller's intent, got %q", got)
	}
}

func TestNegotiationProtocolsSwapsEntryAndReply(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		format    string
		wantEntry string
		wantReply string
	}{
		{"openai client, openai target", "openai", "openai", "openai", "openai"},
		{"claude client, openai target", "claude", "openai", "openai", "claude"},
		{"openai-response client", "openai-response", "openai-response", "openai-response", "openai-response"},
		{"openai-response client, openai target", "openai-response", "openai", "openai", "openai-response"},
		{"gemini client, openai target", "gemini", "openai", "openai", "gemini"},
		{"claude client, claude target", "claude", "claude", "claude", "claude"},
		{"unknown format passes through", "brand-new", "brand-new", "brand-new", "brand-new"},
		{"no target format", "claude", "", "claude", "claude"},
		{"no client format", "", "openai", "openai", "openai"},
		{"neither", "", "", "openai", "openai"},
	}
	for _, tc := range cases {
		req := rpcExecutorRequest{}
		req.SourceFormat = tc.source
		req.Format = tc.format
		entry, reply := negotiationProtocols(req)
		if entry != tc.wantEntry || reply != tc.wantReply {
			t.Errorf("%s: got entry=%q reply=%q, want entry=%q reply=%q",
				tc.name, entry, reply, tc.wantEntry, tc.wantReply)
		}
	}
}

func TestStreamComboStopsFailingOverAfterOutputStarted(t *testing.T) {
	c := combo{Name: "smart", Targets: []comboTarget{
		{Provider: "glm", Model: "a"},
		{Provider: "glm", Model: "b"},
	}}
	err := streamCombo(context.Background(), c, []byte(`{}`), "openai", "openai", "cb", "")
	if err == nil {
		t.Fatal("expected an error with no reachable host")
	}
	// The host is unavailable, so nothing was ever emitted and every target was tried.
	if !strings.Contains(err.Error(), "exhausted all 2 targets") {
		t.Fatalf("want an exhausted-all-targets error, got %v", err)
	}
	for _, target := range c.Targets {
		if !strings.Contains(err.Error(), target.Model) {
			t.Fatalf("error should name target %s: %v", target.Model, err)
		}
	}
}

func TestStreamComboRejectsEmptyTargetList(t *testing.T) {
	err := streamCombo(context.Background(), combo{Name: "empty"}, nil, "openai", "openai", "cb", "")
	if err == nil || !strings.Contains(err.Error(), "has no targets") {
		t.Fatalf("want a no-targets error, got %v", err)
	}
}
