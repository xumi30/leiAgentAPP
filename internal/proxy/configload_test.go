package proxy

import (
	"strings"
	"testing"
)

func TestLLMConfigFromRootUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("LEIAGENT_LLM_API_KEY", "env-key")
	t.Setenv("LEIAGENT_LLM_MODEL", "env-model")

	config, err := llmConfigFromRoot(fileRoot{LLM: llmYAML{
		APIKey:  "file-key",
		BaseURL: "https://example.test/v1/chat/completions",
		Model:   "file-model",
	}}, "config.yaml")
	if err != nil {
		t.Fatalf("llmConfigFromRoot returned error: %v", err)
	}
	if config.token != "env-key" || config.modelName != "env-model" {
		t.Fatalf("environment overrides not applied: %+v", config)
	}
}

func TestLLMConfigAllowsEmptyAPIKeyForLocalServices(t *testing.T) {
	t.Setenv("LEIAGENT_LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	config, err := llmConfigFromRoot(fileRoot{LLM: llmYAML{
		BaseURL: "http://127.0.0.1:11434/v1/chat/completions",
		Model:   "local-model",
	}}, "config.yaml")
	if err != nil {
		t.Fatalf("expected keyless local config to pass: %v", err)
	}
	if config.token != "" {
		t.Fatalf("expected empty token, got %q", config.token)
	}
}

func TestLLMConfigAllowsEnvironmentOnly(t *testing.T) {
	t.Setenv("LEIAGENT_LLM_BASE_URL", "https://example.test/v1/chat/completions")
	t.Setenv("LEIAGENT_LLM_MODEL", "env-model")

	config, err := llmConfigFromRoot(fileRoot{}, "config.yaml")
	if err != nil {
		t.Fatalf("expected environment-only config to pass: %v", err)
	}
	if config.url != "https://example.test/v1/chat/completions" || config.modelName != "env-model" {
		t.Fatalf("environment-only config not applied: %+v", config)
	}
}

func TestLLMConfigRequiresURLAndModel(t *testing.T) {
	_, err := llmConfigFromRoot(fileRoot{LLM: llmYAML{Model: "model"}}, "config.yaml")
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("expected base_url error, got %v", err)
	}

	_, err = llmConfigFromRoot(fileRoot{LLM: llmYAML{BaseURL: "https://example.test/v1/chat/completions"}}, "config.yaml")
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model error, got %v", err)
	}
}

func TestValidateLLMConfigYAMLSkipsWhenNoLLMBlock(t *testing.T) {
	data := []byte(`
mcp_servers:
  - label: "demo"
    url: "http://127.0.0.1:3000"
`)

	if err := ValidateLLMConfigYAML(data); err != nil {
		t.Fatalf("expected config without llm to pass, got: %v", err)
	}
}
