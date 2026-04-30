package mcpbridge

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"leiAgent/internal/appruntime"

	"go.yaml.in/yaml/v2"
)

type envConfigRoot struct {
	OpenClaw struct {
		Env map[string]string `yaml:"env"`
	} `yaml:"openclaw"`
}

func envForServerConfig(cfg ServerConfig) []string {
	env := os.Environ()
	for key, value := range cfg.Env {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		env = upsertEnv(env, key, value)
	}
	for _, key := range InferRequiredEnvKeys(cfg) {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(os.Getenv(key)) != "" {
			continue
		}
		value, source := resolveEnvValue(cfg, key)
		if value == "" || source == "process" || source == "mcp_config" {
			continue
		}
		env = upsertEnv(env, key, value)
	}
	return env
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	next := env[:0]
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, item)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

func resolveEnvValue(cfg ServerConfig, key string) (string, string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ""
	}
	if value := strings.TrimSpace(cfg.Env[key]); value != "" {
		return value, "mcp_config"
	}
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, "process"
	}
	if value := strings.TrimSpace(configEnv()[key]); value != "" {
		return value, "config"
	}
	if runtime.GOOS == "darwin" {
		if value := readLaunchctlEnv(key); value != "" {
			return value, "launchctl"
		}
	}
	if value := readShellEnv(key); value != "" {
		return value, "login_shell"
	}
	return "", ""
}

func configEnv() map[string]string {
	path := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH"))
	if path == "" {
		path = appruntime.ResolvePath("config/config.yaml")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root envConfigRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil
	}
	out := make(map[string]string, len(root.OpenClaw.Env))
	for key, value := range root.OpenClaw.Env {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func readLaunchctlEnv(key string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "launchctl", "getenv", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readShellEnv(key string) string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		if runtime.GOOS == "windows" {
			return ""
		}
		shell = "/bin/zsh"
	}
	if _, err := os.Stat(shell); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lic", "printenv "+key).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if value := strings.TrimSpace(lines[i]); value != "" {
			return value
		}
	}
	return ""
}
