package compressstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// Dir returns an absolute directory path for compressed artifacts (creates it if missing).
// persistDir is a path relative to current working directory, e.g. "compress".
func Dir(persistDir string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	rel := strings.TrimSpace(persistDir)
	if rel == "" {
		rel = "compress"
	}
	dir := filepath.Join(cwd, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func filePath(persistDir, chatID string) (string, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return "", fmt.Errorf("chatID 为空")
	}
	dir, err := Dir(persistDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cid+".yaml"), nil
}

// PersistYAML writes v as YAML to compress/{chatID}.yaml atomically.
func PersistYAML(persistDir, chatID string, v any) (string, error) {
	path, err := filePath(persistDir, chatID)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// LoadYAML reads compress/{chatID}.yaml into out.
// Returns (false, nil) if file does not exist.
func LoadYAML(persistDir, chatID string, out any) (bool, string, error) {
	path, err := filePath(persistDir, chatID)
	if err != nil {
		return false, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, path, nil
		}
		return false, path, err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return true, path, err
	}
	return true, path, nil
}

