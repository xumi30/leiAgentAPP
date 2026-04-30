package mcpbridge

import (
	"os"
	"strings"
	"testing"
)

func TestMissingRequiredEnvKeysUsesProcessEnv(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "test-key")

	missing := MissingRequiredEnvKeys(ServerConfig{
		Label:   "tavily-ai-tavily-mcp",
		Command: "npx",
		Args:    []string{"-y", "tavily-mcp@latest"},
		Env:     map[string]string{},
	})
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}

func TestEnvForServerConfigInjectsConfigResolvedRequiredEnv(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")
	cfgPath := writeTempMCPConfig(t, "openclaw:\n  env:\n    TAVILY_API_KEY: config-key\n")
	t.Setenv("LEIAGENT_CONFIG_PATH", cfgPath)

	env := envForServerConfig(ServerConfig{
		Label:   "tavily-ai-tavily-mcp",
		Command: "npx",
		Args:    []string{"-y", "tavily-mcp@latest"},
		Env:     map[string]string{},
	})

	if !envContains(env, "TAVILY_API_KEY=config-key") {
		t.Fatalf("env does not contain resolved TAVILY_API_KEY: %v", env)
	}
	if countEnvKey(env, "TAVILY_API_KEY") != 1 {
		t.Fatalf("TAVILY_API_KEY appears %d times, want 1", countEnvKey(env, "TAVILY_API_KEY"))
	}
}

func writeTempMCPConfig(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func envContains(env []string, want string) bool {
	for _, item := range env {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func countEnvKey(env []string, key string) int {
	count := 0
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			count++
		}
	}
	return count
}
