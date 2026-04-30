package proxy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type fileRoot struct {
	LLM         llmYAML   `yaml:"llm"`
	LLMBackends []llmYAML `yaml:"llm_backends"`
}

type llmYAML struct {
	Name            string `yaml:"name"`
	APIKey          string `yaml:"api_key"`
	BaseURL         string `yaml:"base_url"`
	Model           string `yaml:"model"`
	Provider        string `yaml:"provider"`
	StreamMode      string `yaml:"stream_mode"`
	MaxOutputTokens int    `yaml:"max_output_tokens,omitempty"`
	Enabled         *bool  `yaml:"enabled,omitempty"`
}

func rowEnabled(row llmYAML) bool {
	if row.Enabled == nil {
		return true
	}
	return *row.Enabled
}

func resolveConfigPath() (string, bool) {
	if p := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), true
		}
	}

	candidates := []string{
		filepath.Clean(filepath.Join("config", "config.yaml")),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config", "config.yaml"))
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return filepath.Clean(c), true
		}
	}
	return "", false
}

func apiKeyFromEnv() string {
	keys := []string{
		"LEIAGENT_LLM_API_KEY",
		"OPENAI_API_KEY",
		"DASHSCOPE_API_KEY",
		"ZHIPU_API_KEY",
		"ZHIPUAI_API_KEY",
		"SILICONFLOW_API_KEY",
		"DEEPSEEK_API_KEY",
		"MOONSHOT_API_KEY",
		"GROQ_API_KEY",
		"OPENROUTER_API_KEY",
		"TOGETHER_API_KEY",
		"FIREWORKS_API_KEY",
		"PERPLEXITY_API_KEY",
		"MISTRAL_API_KEY",
		"XAI_API_KEY",
		"COHERE_API_KEY",
		"NVIDIA_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"ANTHROPIC_API_KEY",
		"BAIDU_QIANFAN_API_KEY",
		"TENCENT_HUNYUAN_API_KEY",
		"MINIMAX_API_KEY",
		"BAICHUAN_API_KEY",
		"ARK_API_KEY",
		"VOLCENGINE_API_KEY",
	}
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(envKey, fileVal, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if v := strings.TrimSpace(fileVal); v != "" {
		return v
	}
	return fallback
}

func parseStreamMode(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false", "nonstream", "no", "off":
		return StreamModeNonStream, true
	case "1", "true", "stream", "yes", "on":
		return StreamModeStream, true
	case "":
		return 0, false
	default:
		return StreamModeBoth, true
	}
}

func hasLLMConfigInYAML(root fileRoot) bool {
	if len(root.LLMBackends) > 0 {
		return true
	}
	l := root.LLM
	return strings.TrimSpace(l.APIKey) != "" ||
		strings.TrimSpace(l.BaseURL) != "" ||
		strings.TrimSpace(l.Model) != ""
}

func shouldApplyLLMYAMLFromConfig(root fileRoot) bool {
	return hasLLMConfigInYAML(root)
}

func mergeLLMRow(row llmYAML, globalEnv bool) (Backend, error) {
	token := strings.TrimSpace(row.APIKey)
	if token == "" {
		token = apiKeyFromEnv()
	}

	var baseURL, modelName string
	if globalEnv {
		baseURL = firstNonEmpty("LEIAGENT_LLM_BASE_URL", row.BaseURL, "")
		modelName = firstNonEmpty("LEIAGENT_LLM_MODEL", row.Model, "")
	} else {
		baseURL = strings.TrimSpace(row.BaseURL)
		modelName = strings.TrimSpace(row.Model)
	}

	provider := ""
	if globalEnv {
		provider = strings.ToLower(strings.TrimSpace(os.Getenv("LEIAGENT_LLM_PROVIDER")))
	}
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(row.Provider))
	}

	streamMode := ""
	if globalEnv {
		streamMode = strings.TrimSpace(os.Getenv("LEIAGENT_LLM_STREAM_MODE"))
	}
	if streamMode == "" {
		streamMode = strings.TrimSpace(row.StreamMode)
	}
	var sm int
	if v, ok := parseStreamMode(streamMode); ok {
		sm = v
	} else if provider == "gemini" {
		sm = StreamModeNonStream
	} else {
		sm = StreamModeBoth
	}

	switch {
	case token == "":
		return Backend{}, errors.New("missing api_key (set llm.api_key / llm_backends[].api_key or env)")
	case baseURL == "":
		return Backend{}, errors.New("missing base_url")
	case modelName == "":
		return Backend{}, errors.New("missing model")
	}

	return Backend{
		Name:            strings.TrimSpace(row.Name),
		Provider:        provider,
		BaseURL:         baseURL,
		Model:           modelName,
		APIKey:          token,
		StreamMode:      sm,
		MaxOutputTokens: row.MaxOutputTokens,
	}, nil
}

func configFromRoot(root fileRoot) (Config, error) {
	if len(root.LLMBackends) > 0 {
		fallbackKey := strings.TrimSpace(root.LLM.APIKey)
		out := make([]Backend, 0, len(root.LLMBackends))
		for i, row := range root.LLMBackends {
			if !rowEnabled(row) {
				continue
			}
			if strings.TrimSpace(row.APIKey) == "" && fallbackKey != "" {
				row.APIKey = fallbackKey
			}
			b, err := mergeLLMRow(row, false)
			if err != nil {
				return Config{}, fmt.Errorf("llm_backends[%d]: %w", i, err)
			}
			out = append(out, b)
		}
		if len(out) == 0 {
			return Config{}, errors.New("llm_backends: at least one backend must be enabled")
		}
		return Config{Backends: out}, nil
	}

	b, err := mergeLLMRow(root.LLM, true)
	if err != nil {
		return Config{}, err
	}
	return Config{Backends: []Backend{b}}, nil
}

// LoadConfig loads LLM backends from config/config.yaml when that file exists
// and shouldApplyLLMYAMLFromConfig(root) is true. There is no fallback file.
func LoadConfig() (Config, error) {
	cfgPath, ok := resolveConfigPath()
	if !ok {
		return Config{}, errors.New("config not found: need config/config.yaml with LLM settings")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}
	if !shouldApplyLLMYAMLFromConfig(root) {
		return Config{}, fmt.Errorf("no LLM block in %s: add llm or llm_backends", cfgPath)
	}
	cfg, err := configFromRoot(root)
	if err != nil {
		return Config{}, err
	}
	cfg.SourcePath = cfgPath
	return cfg, nil
}
