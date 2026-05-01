package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LLMConnectionStatus struct {
	OK         bool   `json:"ok"`
	Reachable  bool   `json:"reachable"`
	Phase      string `json:"phase"`
	Message    string `json:"message"`
	ConfigPath string `json:"configPath"`
	Model      string `json:"model,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

func chatURLToModelsURL(chatURL string) (string, bool) {
	url := strings.TrimSpace(chatURL)
	if url == "" {
		return "", false
	}
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, "/v1/chat/completions"):
		return url[:len(url)-len("/v1/chat/completions")] + "/v1/models", true
	case strings.HasSuffix(lower, "/chat/completions"):
		return url[:len(url)-len("/chat/completions")] + "/models", true
	default:
		return "", false
	}
}

func CheckLLMConnectionStatus(ctx context.Context) LLMConnectionStatus {
	config, err := loadModelConfig()
	if err != nil {
		return LLMConnectionStatus{Phase: "config_error", Message: err.Error()}
	}

	status := LLMConnectionStatus{ConfigPath: config.configPath, Model: config.modelName}
	modelsURL, canProbe := chatURLToModelsURL(config.url)
	if !canProbe {
		status.OK = true
		status.Reachable = true
		status.Phase = "configured"
		status.Message = "配置已加载；该地址不支持自动推导 /models 探测"
		return status
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		status.Phase = "unreachable"
		status.Message = fmt.Sprintf("构造探测请求失败：%v", err)
		return status
	}
	if config.token != "" {
		request.Header.Set("Authorization", "Bearer "+config.token)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		status.Phase = "unreachable"
		status.Message = fmt.Sprintf("网络不可达：%v", err)
		return status
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	status.Reachable = true
	status.HTTPStatus = response.StatusCode
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		status.OK = true
		status.Phase = "connected"
		status.Message = "API 连接正常"
	case response.StatusCode == http.StatusNotFound:
		status.OK = true
		status.Phase = "configured"
		status.Message = "服务未提供 /models；配置将在实际对话时验证"
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		status.Phase = "http_error"
		status.Message = fmt.Sprintf("鉴权失败（HTTP %d），请检查 API Key", response.StatusCode)
	default:
		status.Phase = "http_error"
		status.Message = fmt.Sprintf("探测返回 HTTP %d，请检查 base_url", response.StatusCode)
	}
	return status
}
