package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type providerCache struct {
	mu      sync.RWMutex
	known   []string
	updated time.Time
}

var providers = &providerCache{}

func (c *providerCache) set(list []string) {
	cleaned := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, p := range list {
		v := strings.ToLower(strings.TrimSpace(p))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		cleaned = append(cleaned, v)
	}
	sort.Strings(cleaned)
	c.mu.Lock()
	c.known = cleaned
	c.updated = time.Now()
	c.mu.Unlock()
}

func (c *providerCache) list() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.known))
	copy(out, c.known)
	return out
}

func (c *providerCache) resolve(spec string) string {
	want := strings.ToLower(strings.TrimSpace(spec))
	if want == "" {
		return ""
	}
	known := c.list()
	for _, k := range known {
		if k == want {
			return k
		}
	}
	var suffix []string
	for _, k := range known {
		if strings.HasSuffix(k, "-"+want) || strings.HasSuffix(k, want) {
			suffix = append(suffix, k)
		}
	}
	if len(suffix) == 1 {
		return suffix[0]
	}
	var contains []string
	for _, k := range known {
		if strings.Contains(k, want) {
			contains = append(contains, k)
		}
	}
	if len(contains) == 1 {
		return contains[0]
	}
	return want
}
