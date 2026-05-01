package proxy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v2"
)

// LLMConfigRow 供前端表格与 Wails 绑定（JSON camelCase）。
type LLMConfigRow struct {
	Name            string `json:"name"`
	APIKey          string `json:"apiKey"`
	BaseURL         string `json:"baseUrl"`
	Model           string `json:"model"`
	Provider        string `json:"provider"`
	StreamMode      string `json:"streamMode"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
	Enabled         bool   `json:"enabled"`
}

// UnmarshalJSON 兼容 Wails/前端可能使用的 camelCase、snake_case 等键名。
func (r *LLMConfigRow) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if len(m) == 0 {
		r.Enabled = true
		return nil
	}
	r.Name = jsonStringAny(m, "name", "Name")
	r.APIKey = jsonStringAny(m, "apiKey", "api_key", "ApiKey", "APIKey")
	r.BaseURL = jsonStringAny(m, "baseUrl", "base_url", "BaseURL", "BaseUrl")
	r.Model = jsonStringAny(m, "model", "Model")
	r.Provider = jsonStringAny(m, "provider", "Provider")
	r.StreamMode = jsonStringAny(m, "streamMode", "stream_mode", "StreamMode")
	r.MaxOutputTokens = jsonIntAny(m, "maxOutputTokens", "max_output_tokens", "MaxOutputTokens")
	r.Enabled = jsonBoolDefaultTrue(m, "enabled", "Enabled")
	return nil
}

func jsonBoolDefaultTrue(m map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			return b
		}
	}
	return true
}

func jsonStringAny(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func jsonIntAny(m map[string]json.RawMessage, keys ...string) int {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var n int
		if err := json.Unmarshal(raw, &n); err == nil {
			return n
		}
		var f float64
		if err := json.Unmarshal(raw, &f); err == nil {
			return int(f)
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				return v
			}
		}
	}
	return 0
}

// LLMConfigFormState 为 GetLLMConfigFormState 的返回值。
type LLMConfigFormState struct {
	Primary      LLMConfigRow   `json:"primary"`
	Backends     []LLMConfigRow `json:"backends"`
	Path         string         `json:"path"`
	UsingExample bool           `json:"usingExample"`
}

func rowFromYAML(y llmYAML) LLMConfigRow {
	en := true
	if y.Enabled != nil {
		en = *y.Enabled
	}
	return LLMConfigRow{
		Name:            strings.TrimSpace(y.Name),
		APIKey:          y.APIKey,
		BaseURL:         strings.TrimSpace(y.BaseURL),
		Model:           strings.TrimSpace(y.Model),
		Provider:        strings.TrimSpace(y.Provider),
		StreamMode:      strings.TrimSpace(y.StreamMode),
		MaxOutputTokens: y.MaxOutputTokens,
		Enabled:         en,
	}
}

func (r LLMConfigRow) toYAML() llmYAML {
	y := llmYAML{
		Name:            strings.TrimSpace(r.Name),
		APIKey:          strings.TrimSpace(r.APIKey),
		BaseURL:         strings.TrimSpace(r.BaseURL),
		Model:           strings.TrimSpace(r.Model),
		Provider:        strings.TrimSpace(r.Provider),
		StreamMode:      strings.TrimSpace(r.StreamMode),
		MaxOutputTokens: r.MaxOutputTokens,
	}
	if !r.Enabled {
		f := false
		y.Enabled = &f
	}
	return y
}

// GetLLMConfigFormState 将当前或示例 YAML 解析为表格数据。
func GetLLMConfigFormState() (LLMConfigFormState, error) {
	content, path, usingExample, err := ReadLLMConfigForUI()
	if err != nil {
		return LLMConfigFormState{}, err
	}

	data := []byte(strings.ReplaceAll(content, "\r\n", "\n"))
	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return LLMConfigFormState{}, fmt.Errorf("YAML 解析失败：%w", err)
	}

	backends := make([]LLMConfigRow, 0, len(root.LLMBackends))
	for _, b := range root.LLMBackends {
		backends = append(backends, rowFromYAML(b))
	}
	if len(backends) == 0 {
		llm := rowFromYAML(root.LLM)
		if strings.TrimSpace(llm.BaseURL) != "" || strings.TrimSpace(llm.Model) != "" || strings.TrimSpace(llm.APIKey) != "" {
			llm.Enabled = true
			backends = []LLMConfigRow{llm}
		}
	}
	for i := range backends {
		backends[i].BaseURL = NormalizeProxyLbBaseURLForDisplay(backends[i].Name, backends[i].BaseURL)
		backends[i].Model = NormalizeProxyLbModelForDisplay(backends[i].Name, backends[i].Model)
	}

	return LLMConfigFormState{
		Primary:      LLMConfigRow{Enabled: true},
		Backends:     backends,
		Path:         path,
		UsingExample: usingExample,
	}, nil
}

// SaveLLMConfigForm 将表格数据序列化为 YAML，校验并写入（与 SaveLLMConfigText 相同落盘规则）。
func SaveLLMConfigForm(primary LLMConfigRow, backends []LLMConfigRow) (savedPath string, err error) {
	_ = primary
	content, _, _, err := ReadLLMConfigForUI()
	if err != nil {
		return "", err
	}

	if len(backends) == 0 {
		return "", fmt.Errorf("多后端列表至少需一行")
	}

	doc, err := parseYAMLDocumentNode(content)
	if err != nil {
		return "", fmt.Errorf("YAML 解析失败：%w", err)
	}

	rows := make([]llmYAML, 0, len(backends))
	for _, b := range backends {
		rows = append(rows, b.toYAML())
	}
	backendsNode, err := nodeFromValue(rows)
	if err != nil {
		return "", fmt.Errorf("生成 llm_backends 失败：%w", err)
	}
	upsertRootKey(doc, "llm_backends", backendsNode)

	data, err := marshalYAMLDocumentNode(doc)
	if err != nil {
		return "", fmt.Errorf("YAML 序列化失败：%w", err)
	}
	if err := ValidateLLMConfigYAML(data); err != nil {
		return "", err
	}
	return writeLLMConfigBytes(data)
}
