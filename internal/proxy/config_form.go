package proxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v2"
)

// LLMConfig 是应用唯一的模型连接配置。所有服务统一使用 OpenAI-compatible Chat Completions 协议。
type LLMConfig struct {
	APIKey          string `json:"apiKey"`
	BaseURL         string `json:"baseUrl"`
	Model           string `json:"model"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
}

type LLMConfigState struct {
	Config       LLMConfig `json:"config"`
	Path         string    `json:"path"`
	UsingExample bool      `json:"usingExample"`
}

func jsonStringAny(values map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func llmConfigFromYAML(value llmYAML) LLMConfig {
	return LLMConfig{
		APIKey:          value.APIKey,
		BaseURL:         strings.TrimSpace(value.BaseURL),
		Model:           strings.TrimSpace(value.Model),
		MaxOutputTokens: value.MaxOutputTokens,
	}
}

func (config LLMConfig) toYAML() llmYAML {
	return llmYAML{
		APIKey:          strings.TrimSpace(config.APIKey),
		BaseURL:         strings.TrimSpace(config.BaseURL),
		Model:           strings.TrimSpace(config.Model),
		MaxOutputTokens: config.MaxOutputTokens,
	}
}

func GetLLMConfig() (LLMConfigState, error) {
	content, path, usingExample, err := ReadLLMConfigForUI()
	if err != nil {
		return LLMConfigState{}, err
	}

	var root fileRoot
	data := []byte(strings.ReplaceAll(content, "\r\n", "\n"))
	if err := yaml.Unmarshal(data, &root); err != nil {
		return LLMConfigState{}, fmt.Errorf("YAML 解析失败：%w", err)
	}

	return LLMConfigState{
		Config:       llmConfigFromYAML(root.LLM),
		Path:         path,
		UsingExample: usingExample,
	}, nil
}

// SaveLLMConfig 保存唯一的 llm 配置，并移除旧的 llm_backends 结构。
func SaveLLMConfig(config LLMConfig) (savedPath string, err error) {
	content, _, _, err := ReadLLMConfigForUI()
	if err != nil {
		return "", err
	}

	doc, err := parseYAMLDocumentNode(content)
	if err != nil {
		return "", fmt.Errorf("YAML 解析失败：%w", err)
	}

	llmNode, err := nodeFromValue(config.toYAML())
	if err != nil {
		return "", fmt.Errorf("生成 llm 配置失败：%w", err)
	}
	upsertRootKey(doc, "llm", llmNode)
	removeRootKey(doc, "llm_backends")

	data, err := marshalYAMLDocumentNode(doc)
	if err != nil {
		return "", fmt.Errorf("YAML 序列化失败：%w", err)
	}
	if err := ValidateLLMConfigYAML(data); err != nil {
		return "", err
	}
	return writeLLMConfigBytes(data)
}
