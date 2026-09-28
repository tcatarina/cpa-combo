package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type comboTarget struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
	Label    string `json:"label,omitempty"`
	AuthID   string `json:"auth_id,omitempty"`
	Account  string `json:"account,omitempty"`
}

type combo struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Targets     []comboTarget `json:"targets"`
	CreatedAt   *time.Time    `json:"created_at,omitempty"`
	UpdatedAt   *time.Time    `json:"updated_at,omitempty"`
}

type comboFile struct {
	Version int     `json:"version"`
	Combos  []combo `json:"combos"`
}

type runtimeState struct {
	mu      sync.RWMutex
	enabled bool
	path    string
	combos  []combo
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type pluginYAMLConfig struct {
	Enabled   *bool  `yaml:"enabled"`
	StorePath string `yaml:"store_path"`
}

func newRuntimeState() *runtimeState {
	return &runtimeState{enabled: true, path: defaultStorePath()}
}

func defaultStorePath() string {
	if dir := selfLibraryDir(); dir != "" {
		return filepath.Join(dir, storeFileName)
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, storeFileName)
	}
	return storeFileName
}

func selfLibraryDir() string {
	raw, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return ""
	}
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		idx := strings.Index(line, " /")
		if idx < 0 {
			continue
		}
		path := strings.TrimSpace(line[idx+1:])
		if path == "" || path == self || !strings.HasSuffix(path, ".so") {
			continue
		}
		return filepath.Dir(path)
	}
	return ""
}

func (s *runtimeState) configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
	}
	if len(req.ConfigYAML) > 0 {
		var cfg pluginYAMLConfig
		if err := yaml.Unmarshal(req.ConfigYAML, &cfg); err != nil {
			return err
		}
		if cfg.Enabled != nil {
			s.mu.Lock()
			s.enabled = *cfg.Enabled
			s.mu.Unlock()
		}
		if p := strings.TrimSpace(cfg.StorePath); p != "" {
			s.mu.Lock()
			s.path = p
			s.mu.Unlock()
		}
	}
	return s.reload()
}

func (s *runtimeState) reload() error {
	s.mu.RLock()
	path := s.path
	s.mu.RUnlock()

	combos, err := readCombos(path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.path = path
	s.combos = combos
	s.mu.Unlock()
	return nil
}

func readCombos(path string) ([]combo, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var file comboFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	cleaned := make([]combo, 0, len(file.Combos))
	for _, c := range file.Combos {
		if norm := normalizeCombo(c); norm != nil {
			cleaned = append(cleaned, *norm)
		}
	}
	sort.SliceStable(cleaned, func(i, j int) bool { return cleaned[i].Name < cleaned[j].Name })
	return cleaned, nil
}

func writeCombos(path string, combos []combo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sorted := append([]combo(nil), combos...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	raw, err := json.MarshalIndent(comboFile{Version: 1, Combos: sorted}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normalizeCombo(c combo) *combo {
	out := combo{
		Name:        normalizeModelName(c.Name),
		Description: strings.TrimSpace(c.Description),
		Targets:     []comboTarget{},
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
	if out.Name == "" {
		return nil
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		model := normalizeModelName(t.Model)
		if model == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(t.Provider)) + "\x00" + model + "\x00" + strings.TrimSpace(t.AuthID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Targets = append(out.Targets, comboTarget{
			Provider: strings.ToLower(strings.TrimSpace(t.Provider)),
			Model:    model,
			Label:    strings.TrimSpace(t.Label),
			AuthID:   strings.TrimSpace(t.AuthID),
			Account:  strings.TrimSpace(t.Account),
		})
	}
	if len(out.Targets) == 0 {
		return nil
	}
	return &out
}

func normalizeModelName(v string) string {
	return strings.TrimSpace(v)
}

func splitComboModel(model string) (string, bool) {
	trimmed := normalizeModelName(model)
	if trimmed == "" {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), comboNamespace+"/") {
		return "", false
	}
	rest := trimmed[len(comboNamespace)+1:]
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return "", false
	}
	return normalizeModelName(rest), true
}

func (s *runtimeState) snapshot() (bool, []combo) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]combo, len(s.combos))
	copy(out, s.combos)
	return s.enabled, out
}

func (s *runtimeState) find(name string) (combo, bool) {
	wanted := normalizeModelName(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.combos {
		if strings.EqualFold(c.Name, wanted) {
			return c, true
		}
	}
	return combo{}, false
}

func (s *runtimeState) replaceAll(combos []combo) error {
	cleaned := make([]combo, 0, len(combos))
	seen := map[string]bool{}
	for _, c := range combos {
		norm := normalizeCombo(c)
		if norm == nil {
			continue
		}
		lower := strings.ToLower(norm.Name)
		if seen[lower] {
			return errors.New("duplicate combo name: " + norm.Name)
		}
		seen[lower] = true
		cleaned = append(cleaned, *norm)
	}
	path := s.currentPath()
	if err := writeCombos(path, cleaned); err != nil {
		return err
	}
	s.mu.Lock()
	s.combos = cleaned
	s.mu.Unlock()
	return nil
}

func (s *runtimeState) currentPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}
