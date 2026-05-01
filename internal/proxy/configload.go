package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"leiAgent/internal/appruntime"
	"leiAgent/internal/bashpolicy"

	"go.yaml.in/yaml/v2"
)

type ShellSafetyYAML struct {
	Rules []bashpolicy.Rule `yaml:"rules,omitempty"`
	// 旧字段：无 rules 时会并入默认列表（仅运行时兼容）。
	ExtraBlockedSubstrings []string `yaml:"extra_blocked_substrings,omitempty"`
}

type fileRoot struct {
	LLM               llmYAML               `yaml:"llm"`
	MemoryCompression MemoryCompressionYAML `yaml:"memory_compression,omitempty"`
	ShellSafety       ShellSafetyYAML       `yaml:"shell_safety,omitempty"`
}

type llmYAML struct {
	APIKey          string `yaml:"api_key,omitempty"`
	BaseURL         string `yaml:"base_url,omitempty"`
	Model           string `yaml:"model,omitempty"`
	MaxOutputTokens int    `yaml:"max_output_tokens,omitempty"`
}

type modelConfig struct {
	url             string
	token           string
	modelName       string
	maxOutputTokens int
	configPath      string
}

func resolveConfigPath() (path string, ok bool) {
	if p := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), true
		}
	}
	candidates := []string{
		appruntime.ResolvePath(filepath.Join("config", "config.yaml")),
		filepath.Clean(filepath.Join("config", "config.yaml")),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config", "config.yaml"))
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return filepath.Clean(candidate), true
		}
	}
	return "", false
}

func hasLLMConfigInYAML(root fileRoot) bool {
	llm := root.LLM
	return strings.TrimSpace(llm.APIKey) != "" ||
		strings.TrimSpace(llm.BaseURL) != "" ||
		strings.TrimSpace(llm.Model) != ""
}

func readConfigRoot() (root fileRoot, configPath string, err error) {
	path, ok := resolveConfigPath()
	if ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return fileRoot{}, path, err
		}
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fileRoot{}, path, err
		}
		return root, path, nil
	}
	if len(bundledConfigYAML) > 0 {
		if err := yaml.Unmarshal(bundledConfigYAML, &root); err != nil {
			return fileRoot{}, bundledYAMLPathMarker, err
		}
		return root, bundledYAMLPathMarker, nil
	}
	return fileRoot{}, "", nil
}

// InitShellSafetyFromConfig 加载 shell_safety.rules（或旧的 extra_blocked_substrings）到运行时策略。
func InitShellSafetyFromConfig() {
	root, path, err := readConfigRoot()
	if path == "" || err != nil {
		_ = bashpolicy.SetRules(bashpolicy.DefaultRules())
		return
	}
	effective := bashpolicy.MergeFromYAML(root.ShellSafety.Rules, root.ShellSafety.ExtraBlockedSubstrings)
	if err := bashpolicy.SetRules(effective); err != nil {
		_ = bashpolicy.SetRules(bashpolicy.DefaultRules())
	}
}

func envOrFile(envKey, fileValue string) string {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	return strings.TrimSpace(fileValue)
}

func llmConfigFromRoot(root fileRoot, configPath string) (*modelConfig, error) {
	apiKey := envOrFile("LEIAGENT_LLM_API_KEY", root.LLM.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	baseURL := envOrFile("LEIAGENT_LLM_BASE_URL", root.LLM.BaseURL)
	model := envOrFile("LEIAGENT_LLM_MODEL", root.LLM.Model)

	maxOutputTokens := root.LLM.MaxOutputTokens
	if raw := strings.TrimSpace(os.Getenv("LEIAGENT_LLM_MAX_OUTPUT_TOKENS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("LEIAGENT_LLM_MAX_OUTPUT_TOKENS 必须是非负整数")
		}
		maxOutputTokens = value
	}

	if baseURL == "" {
		return nil, fmt.Errorf("未配置 LLM base_url：请填写 OpenAI-compatible Chat Completions 地址")
	}
	if model == "" {
		return nil, fmt.Errorf("未配置 LLM model")
	}

	return &modelConfig{
		url:             baseURL,
		token:           apiKey,
		modelName:       model,
		maxOutputTokens: maxOutputTokens,
		configPath:      configPath,
	}, nil
}

func loadModelConfig() (*modelConfig, error) {
	root, configPath, err := readConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 配置失败（%s）：%w", configPath, err)
	}
	if configPath == "" {
		return nil, fmt.Errorf("未找到 config/config.yaml（可从 config.example.yaml 复制）")
	}
	return llmConfigFromRoot(root, configPath)
}

// ValidateLLMConfigYAML 校验 YAML 文本中的单个 llm 配置（不写盘）。
func ValidateLLMConfigYAML(data []byte) error {
	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("YAML 解析失败：%w", err)
	}
	if !hasLLMConfigInYAML(root) {
		return nil
	}
	_, err := llmConfigFromRoot(root, "config.yaml")
	return err
}
