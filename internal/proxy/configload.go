package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

type fileRoot struct {
	LLM         llmYAML   `yaml:"llm"`
	LLMBackends []llmYAML `yaml:"llm_backends"`
}

type llmYAML struct {
	Name       string `yaml:"name"`
	APIKey     string `yaml:"api_key"`
	BaseURL    string `yaml:"base_url"`
	Model      string `yaml:"model"`
	Provider   string `yaml:"provider"`
	StreamMode string `yaml:"stream_mode"`
}

func resolveConfigPath() (path string, ok bool) {
	if p := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), true
		}
	}
	candidates := []string{"config/config.yaml"}
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "config", "config.yaml"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return filepath.Clean(c), true
		}
	}
	return "", false
}

func readConfigRoot() (root fileRoot, configPath string, err error) {
	path, ok := resolveConfigPath()
	if !ok {
		return fileRoot{}, "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileRoot{}, path, err
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fileRoot{}, path, err
	}
	return root, path, nil
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
		return streamModeNonStream, true
	case "1", "true", "stream", "yes", "on":
		return streamModeStream, true
	case "":
		return 0, false
	default:
		return streamModeBoth, true
	}
}

// mergeLLMYAML 将一条 llm 配置合并为 ModelAPIInfo。
// globalEnv=true 时，LEIAGENT_LLM_BASE_URL / LEIAGENT_LLM_MODEL 等可从环境变量覆盖 YAML（单后端）。
// globalEnv=false 时仅使用该行 YAML（llm_backends）；api_key 仍可与环境变量共用。
func mergeLLMYAML(row llmYAML, globalEnv bool, cfgPath string) (*ModelAPIInfo, error) {
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
	var isStream int
	if v, ok := parseStreamMode(streamMode); ok {
		isStream = v
	} else if provider == "gemini" {
		isStream = streamModeNonStream
	} else {
		isStream = streamModeBoth
	}

	if token == "" {
		hint := "config/config.yaml（可复制 config/config.example.yaml）"
		if cfgPath != "" {
			hint = cfgPath
		}
		return nil, fmt.Errorf("未配置 API Key：请在 llm.api_key 或 llm_backends[].api_key 填写，或设置环境变量（配置：%s）", hint)
	}
	if strings.TrimSpace(baseURL) == "" {
		if globalEnv {
			return nil, fmt.Errorf("未配置 base_url：请在 llm.base_url 填写，或设置环境变量 LEIAGENT_LLM_BASE_URL")
		}
		return nil, fmt.Errorf("未配置 base_url：请在 llm_backends[].base_url 填写完整 Chat Completions 地址")
	}
	if strings.TrimSpace(modelName) == "" {
		if globalEnv {
			return nil, fmt.Errorf("未配置 model：请在 llm.model 填写，或设置环境变量 LEIAGENT_LLM_MODEL")
		}
		return nil, fmt.Errorf("未配置 model：请在 llm_backends[].model 填写")
	}

	return &ModelAPIInfo{
		backendName: strings.TrimSpace(row.Name),
		provider:    provider,
		token:       token,
		url:         baseURL,
		modelName:   modelName,
		isStream:    isStream,
	}, nil
}

// modelConfigsFromRoot 将已解析的 YAML 根节点转为后端列表（供磁盘加载与内存校验共用）。
func modelConfigsFromRoot(root fileRoot, cfgPath string) ([]*ModelAPIInfo, error) {
	if len(root.LLMBackends) > 0 {
		out := make([]*ModelAPIInfo, 0, len(root.LLMBackends))
		for i, row := range root.LLMBackends {
			m, err := mergeLLMYAML(row, false, cfgPath)
			if err != nil {
				return nil, fmt.Errorf("llm_backends[%d]: %w", i, err)
			}
			out = append(out, m)
		}
		return out, nil
	}

	m, err := mergeLLMYAML(root.LLM, true, cfgPath)
	if err != nil {
		return nil, err
	}
	return []*ModelAPIInfo{m}, nil
}

// loadModelConfigs 加载后端：有 llm_backends 则按顺序 failover；否则单条 llm（支持环境变量覆盖）。
func loadModelConfigs() ([]*ModelAPIInfo, error) {
	root, cfgPath, err := readConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 配置失败（%s）：%w", cfgPath, err)
	}
	return modelConfigsFromRoot(root, cfgPath)
}

// ValidateLLMConfigYAML 校验 YAML 文本能否解析为可用 LLM 配置（不写盘）。
func ValidateLLMConfigYAML(data []byte) error {
	var root fileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("YAML 解析失败：%w", err)
	}
	if _, err := modelConfigsFromRoot(root, "config.yaml"); err != nil {
		return err
	}
	return nil
}
