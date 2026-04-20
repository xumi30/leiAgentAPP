package mcpbridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ToolCache struct {
	Label     string     `json:"label"`
	OK        bool       `json:"ok"`
	State     string     `json:"state,omitempty"`
	Message   string     `json:"message,omitempty"`
	CheckedAt string     `json:"checked_at"`
	Tools     []ToolInfo `json:"tools,omitempty"`
}

func ReadToolCache(label string) (*ToolCache, error) {
	path := cacheFilePath(label)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cache ToolCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func WriteToolCache(cache ToolCache) error {
	if strings.TrimSpace(cache.Label) == "" {
		return nil
	}
	if strings.TrimSpace(cache.CheckedAt) == "" {
		cache.CheckedAt = time.Now().Format(time.RFC3339)
	}
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFilePath(cache.Label), data, 0o600)
}

func DeleteToolCache(label string) error {
	path := cacheFilePath(label)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cacheDir() string {
	return filepath.Clean("data/mcp_cache")
}

func cacheFilePath(label string) string {
	return filepath.Join(cacheDir(), sanitizeLabel(label)+".json")
}

func sanitizeLabel(label string) string {
	label = strings.TrimSpace(strings.ToLower(label))
	if label == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}
