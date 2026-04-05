package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMConnectionStatus 供前端展示：基于配置加载 + 对 OpenAI 兼容 /models 的轻量探测。
type LLMConnectionStatus struct {
	OK         bool   `json:"ok"`         // 配置有效且 /models 返回 2xx
	Reachable  bool   `json:"reachable"`  // 收到任意 HTTP 响应（非网络错误）
	Phase      string `json:"phase"`      // no_config | config_error | gemini_skip | no_probe | connected | http_error | unreachable
	Message    string `json:"message"`
	ConfigPath string `json:"configPath"`
	Backend    string `json:"backend,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

func chatURLToModelsURL(chatURL string) (string, bool) {
	u := strings.TrimSpace(chatURL)
	if u == "" {
		return "", false
	}
	lower := strings.ToLower(u)
	switch {
	case strings.HasSuffix(lower, "/v1/chat/completions"):
		return u[:len(u)-len("/v1/chat/completions")] + "/v1/models", true
	case strings.HasSuffix(lower, "/chat/completions"):
		return u[:len(u)-len("/chat/completions")] + "/models", true
	default:
		return "", false
	}
}

// CheckLLMConnectionStatus 加载配置并探测首个后端的 OpenAI 兼容模型列表接口。
func CheckLLMConnectionStatus(ctx context.Context) LLMConnectionStatus {
	path, ok := resolveConfigPath()
	if !ok {
		return LLMConnectionStatus{
			OK: false, Phase: "no_config", Message: "未找到配置文件，请将 config.example.yaml 复制为 config/config.yaml 并填写",
		}
	}

	backends, err := loadModelConfigs()
	if err != nil {
		return LLMConnectionStatus{
			OK: false, Phase: "config_error", Message: err.Error(), ConfigPath: path,
		}
	}
	if len(backends) == 0 {
		return LLMConnectionStatus{
			OK: false, Phase: "config_error", Message: "配置中没有任何可用 LLM 后端", ConfigPath: path,
		}
	}

	info := backends[0]
	label := info.logLabel()
	if strings.EqualFold(info.provider, "gemini") {
		return LLMConnectionStatus{
			OK: true, Reachable: true, Phase: "gemini_skip", Message: "配置有效（Gemini 未做 HTTP 探测）",
			ConfigPath: path, Backend: label,
		}
	}

	modelsURL, canProbe := chatURLToModelsURL(info.url)
	if !canProbe {
		return LLMConnectionStatus{
			OK: true, Reachable: true, Phase: "no_probe",
			Message: "配置已加载；base_url 非标准 Chat Completions 路径，无法自动探测连通性",
			ConfigPath: path, Backend: label,
		}
	}

	client := &http.Client{Timeout: 12 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return LLMConnectionStatus{
			OK: false, Phase: "unreachable", Message: fmt.Sprintf("构造探测请求失败：%v", err),
			ConfigPath: path, Backend: label,
		}
	}
	switch strings.ToLower(strings.TrimSpace(info.provider)) {
	case "gemini":
		req.Header.Set("x-goog-api-key", info.token)
	default:
		req.Header.Set("Authorization", "Bearer "+info.token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return LLMConnectionStatus{
			OK: false, Phase: "unreachable", Message: fmt.Sprintf("网络不可达：%v", err),
			ConfigPath: path, Backend: label,
		}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	st := resp.StatusCode
	out := LLMConnectionStatus{
		Reachable: true, ConfigPath: path, Backend: label, HTTPStatus: st,
	}
	switch {
	case st >= 200 && st < 300:
		out.OK = true
		out.Phase = "connected"
		out.Message = "API 连接正常"
	case st == 404:
		// 部分兼容网关未实现 OpenAI 的 GET /models，避免误判为不可用。
		out.OK = true
		out.Phase = "no_probe"
		out.Message = "配置有效；服务端未提供模型列表接口（404），无法自动确认连通性"
	case st == 401 || st == 403:
		out.OK = false
		out.Phase = "http_error"
		out.Message = fmt.Sprintf("服务可达但鉴权失败（HTTP %d），请检查 API Key", st)
	default:
		out.OK = false
		out.Phase = "http_error"
		out.Message = fmt.Sprintf("探测返回 HTTP %d，请检查 base_url 与服务商文档", st)
	}
	return out
}
