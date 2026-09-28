package main

import (
	"testing"
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

func TestEntryProtocolPrefersSource(t *testing.T) {
	if got := entryProtocol("claude", "openai"); got != "claude" {
		t.Fatalf("got %q", got)
	}
	if got := entryProtocol("", "nonsense"); got != "openai" {
		t.Fatalf("got %q", got)
	}
	if got := entryProtocol("responses", "openai"); got != "responses" {
		t.Fatalf("got %q", got)
	}
}
