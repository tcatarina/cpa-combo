package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func lifecycleConfig(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte("enabled: true\nstore_path: " + path + "\n")})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConfigureRequiresPersistentAbsoluteStorePath(t *testing.T) {
	for _, path := range []string{"", "combos.json", "plugins/combos.json"} {
		t.Run(path, func(t *testing.T) {
			state := newRuntimeState()
			err := state.configure(lifecycleConfig(t, path))
			if err == nil || !strings.Contains(err.Error(), "store_path") {
				t.Fatalf("path %q: want actionable store_path error, got %v", path, err)
			}
		})
	}
}

func TestCombosSurviveNewRuntimeWithPersistentStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins", storeFileName)
	config := lifecycleConfig(t, path)
	first := newRuntimeState()
	if err := first.configure(config); err != nil {
		t.Fatal(err)
	}
	want := []combo{{Name: "auto/persistence-test", ContextLength: 204800, Targets: []comboTarget{{Model: "model-one"}, {Model: "model-two"}}}}
	if err := first.replaceAll(want); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second := newRuntimeState()
	if err := second.configure(config); err != nil {
		t.Fatal(err)
	}
	_, got := second.snapshot()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new runtime lost combos: got %+v, want %+v", got, want)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("loading rewrote the stored configuration: %v", err)
	}
}

func TestFailedReconfigurePreservesPreviousStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), storeFileName)
	state := newRuntimeState()
	if err := state.configure(lifecycleConfig(t, path)); err != nil {
		t.Fatal(err)
	}
	if err := state.replaceAll([]combo{{Name: "existing", Targets: []comboTarget{{Model: "model-one"}}}}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), storeFileName)
	if err := os.WriteFile(bad, []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.configure(lifecycleConfig(t, bad)); err == nil {
		t.Fatal("corrupt store accepted")
	}
	if state.currentPath() != path {
		t.Fatal("failed reconfigure replaced the working storage path")
	}
	_, got := state.snapshot()
	if len(got) != 1 || got[0].Name != "existing" {
		t.Fatalf("failed reconfigure cleared working combos: %+v", got)
	}
}
