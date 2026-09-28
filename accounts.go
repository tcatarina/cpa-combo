package main

import (
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type authListResponse struct {
	Files []pluginapi.HostAuthFileEntry `json:"files"`
}

type comboAccount struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
	Disabled    bool   `json:"disabled"`
	Unavailable bool   `json:"unavailable"`
}

func listComboAccounts() ([]comboAccount, error) {
	var resp authListResponse
	if err := callHostResult(pluginabi.MethodHostAuthList, map[string]any{}, &resp); err != nil {
		return nil, err
	}
	out := make([]comboAccount, 0, len(resp.Files))
	for _, f := range resp.Files {
		id := strings.TrimSpace(f.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = id
		}
		label := strings.TrimSpace(f.Label)
		if label == "" {
			label = name
		}
		out = append(out, comboAccount{
			ID:          id,
			Name:        name,
			Label:       label,
			Provider:    strings.ToLower(strings.TrimSpace(f.Provider)),
			Status:      strings.TrimSpace(f.Status),
			Disabled:    f.Disabled,
			Unavailable: f.Unavailable,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
