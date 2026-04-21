package proxy

import (
	"strings"
	"testing"
)

func TestValidateLLMConfigYAMLWithSingleBackendFromEnv(t *testing.T) {
	t.Setenv("LEIAGENT_LLM_API_KEY", "test-key")

	data := []byte(`
llm:
  api_key: ""
  base_url: "https://example.test/v1/chat/completions"
  model: "test-model"
`)

	if err := ValidateLLMConfigYAML(data); err != nil {
		t.Fatalf("ValidateLLMConfigYAML returned error: %v", err)
	}
}

func TestModelConfigsFromRootUsesFallbackKeyAndSkipsDisabledBackends(t *testing.T) {
	disabled := false
	root := fileRoot{
		LLM: llmYAML{APIKey: "fallback-key"},
		LLMBackends: []llmYAML{
			{
				Name:    "disabled",
				APIKey:  "unused-key",
				BaseURL: "https://disabled.example/v1/chat/completions",
				Model:   "disabled-model",
				Enabled: &disabled,
			},
			{
				Name:    "primary",
				BaseURL: "https://primary.example/v1/chat/completions",
				Model:   "primary-model",
			},
		},
	}

	configs, err := modelConfigsFromRoot(root, "test-config.yaml")
	if err != nil {
		t.Fatalf("modelConfigsFromRoot returned error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one enabled backend, got %d", len(configs))
	}
	if configs[0].backendName != "primary" {
		t.Fatalf("expected primary backend, got %q", configs[0].backendName)
	}
	if configs[0].token != "fallback-key" {
		t.Fatalf("expected fallback api key, got %q", configs[0].token)
	}
}

func TestModelConfigsFromRootRequiresEnabledBackend(t *testing.T) {
	disabled := false
	root := fileRoot{
		LLMBackends: []llmYAML{
			{
				Name:    "disabled",
				APIKey:  "unused-key",
				BaseURL: "https://disabled.example/v1/chat/completions",
				Model:   "disabled-model",
				Enabled: &disabled,
			},
		},
	}

	_, err := modelConfigsFromRoot(root, "test-config.yaml")
	if err == nil {
		t.Fatal("expected error for no enabled backends")
	}
	if !strings.Contains(err.Error(), "至少需要启用一条后端") {
		t.Fatalf("unexpected error: %v", err)
	}
}
