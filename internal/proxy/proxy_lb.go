package proxy

import (
	"fmt"
	"strings"
)

// ProxyLBDisplayBaseURL 写入 YAML / 表格展示用，不暴露实际主机；实际请求由 ResolveProxyLbRowBaseURL 解析。
const ProxyLBDisplayBaseURL = "proxylb"

// ProxyLBDisplayModel 与 base_url 相同占位符，界面不写真实模型名（如 qwen）；实际由 ResolveProxyLbRowModel 解析。
const ProxyLBDisplayModel = "proxylb"

// DefaultProxyLBOrigin 仅用于认证请求与解析后的 Chat Completions 地址，勿写入 UI/配置文件。
const DefaultProxyLBOrigin = "http://39.96.125.153:7077"

// DefaultProxyLBChatCompletionsURL 实际 HTTP 请求使用的 proxy-lb 端点（由 ProxyLBDisplayBaseURL 解析得到）。
func DefaultProxyLBChatCompletionsURL() string {
	return DefaultProxyLBOrigin + "/v1/chat/completions"
}

func normalizeChatCompletionBaseURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// IsProxyLbPlaceholderOrCanonicalBase 是否为占位符或与内置默认同址的 legacy 配置（均不在界面展示 IP）。
func IsProxyLbPlaceholderOrCanonicalBase(u string) bool {
	s := strings.TrimSpace(u)
	if s == "" {
		return true
	}
	if strings.EqualFold(s, ProxyLBDisplayBaseURL) || strings.EqualFold(s, "proxy-lb") {
		return true
	}
	got := normalizeChatCompletionBaseURL(s)
	want := normalizeChatCompletionBaseURL(DefaultProxyLBChatCompletionsURL())
	return strings.EqualFold(got, want)
}

// ResolveProxyLbRowBaseURL 将 name=proxy-lb 行的 base_url 解析为实际 Chat Completions URL。
func ResolveProxyLbRowBaseURL(name, baseURL string) string {
	if !strings.EqualFold(strings.TrimSpace(name), "proxy-lb") {
		return strings.TrimSpace(baseURL)
	}
	s := strings.TrimSpace(baseURL)
	if IsProxyLbPlaceholderOrCanonicalBase(s) {
		return DefaultProxyLBChatCompletionsURL()
	}
	return s
}

// NormalizeProxyLbBaseURLForDisplay 表格/UI：canonical 或占位均显示为 proxylb。
func NormalizeProxyLbBaseURLForDisplay(name, baseURL string) string {
	if !strings.EqualFold(strings.TrimSpace(name), "proxy-lb") {
		return baseURL
	}
	if IsProxyLbPlaceholderOrCanonicalBase(baseURL) {
		return ProxyLBDisplayBaseURL
	}
	return baseURL
}

// ResolveProxyLbRowModel 将 name=proxy-lb 行的 model 解析为发往服务端的真实模型名（默认 qwen）。
func ResolveProxyLbRowModel(name, model string) string {
	if !strings.EqualFold(strings.TrimSpace(name), "proxy-lb") {
		return strings.TrimSpace(model)
	}
	s := strings.TrimSpace(model)
	if s == "" || strings.EqualFold(s, ProxyLBDisplayModel) || strings.EqualFold(s, "proxy-lb") || strings.EqualFold(s, "qwen") {
		return "qwen"
	}
	return s
}

// NormalizeProxyLbModelForDisplay 表格/UI：占位或默认模型均显示 proxylb。
func NormalizeProxyLbModelForDisplay(name, model string) string {
	if !strings.EqualFold(strings.TrimSpace(name), "proxy-lb") {
		return model
	}
	s := strings.TrimSpace(model)
	if s == "" || strings.EqualFold(s, ProxyLBDisplayModel) || strings.EqualFold(s, "proxy-lb") || strings.EqualFold(s, "qwen") {
		return ProxyLBDisplayModel
	}
	return model
}

// ProxyLBLoginRegisterToken 读取 token 字段，与 proxy-lb 成功响应一致：
// internal/app/router.go 中 POST /auth/login、/auth/register 使用
// c.JSON(200, gin.H{"username": ..., "token": ...})。
func ProxyLBLoginRegisterToken(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	s, ok := m["token"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func backendMatchesCanonicalProxyLb(b LLMConfigRow) bool {
	if !b.Enabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(b.Name), "proxy-lb") {
		return false
	}
	return IsProxyLbPlaceholderOrCanonicalBase(b.BaseURL)
}

// canonicalProxyLbRowStructure 是否与桌面端写入的 proxy-lb 行同一结构（name + base_url），不依赖 enabled。
func canonicalProxyLbRowStructure(r LLMConfigRow) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Name), "proxy-lb") {
		return false
	}
	return IsProxyLbPlaceholderOrCanonicalBase(r.BaseURL)
}

// StripCanonicalProxyLbRows 去掉与桌面端写入一致的 proxy-lb 行（按 name+base_url），避免重复追加。
func StripCanonicalProxyLbRows(rows []LLMConfigRow) []LLMConfigRow {
	out := make([]LLMConfigRow, 0, len(rows))
	for _, r := range rows {
		if canonicalProxyLbRowStructure(r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// AppendProxyLbFallbackBackend 将 token 写成 canonical proxy-lb 行并追加到列表末尾，作故障转移兜底；
// 先移除旧版同名同址行再追加，保持「用户配置在前、Proxy-LB 在后」的顺序。
func AppendProxyLbFallbackBackend(rows []LLMConfigRow, token string) []LLMConfigRow {
	stripped := StripCanonicalProxyLbRows(rows)
	nextRow := LLMConfigRow{
		Name:            "proxy-lb",
		APIKey:          strings.TrimSpace(token),
		BaseURL:         ProxyLBDisplayBaseURL,
		Model:           ProxyLBDisplayModel,
		Provider:        "",
		StreamMode:      "both",
		MaxOutputTokens: 0,
		Enabled:         true,
	}
	return append(stripped, nextRow)
}

// IsProxyLbAuthSession 若存在已启用且带 token 的 canonical proxy-lb 后端则视为「通过 Proxy-LB 登录」。
func IsProxyLbAuthSession() bool {
	state, err := GetLLMConfigFormState()
	if err != nil {
		return false
	}
	for _, b := range state.Backends {
		if !backendMatchesCanonicalProxyLb(b) {
			continue
		}
		if strings.TrimSpace(b.APIKey) != "" {
			return true
		}
	}
	return false
}

// ClearProxyLbAuthCredentials 清除与 canonical Proxy-LB 登录行匹配的 api_key。
func ClearProxyLbAuthCredentials() error {
	state, err := GetLLMConfigFormState()
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	if len(state.Backends) == 0 {
		return fmt.Errorf("没有可清除的 Proxy-LB 会话")
	}
	next := append([]LLMConfigRow(nil), state.Backends...)
	cleared := false
	for i := range next {
		if !backendMatchesCanonicalProxyLb(next[i]) {
			continue
		}
		if strings.TrimSpace(next[i].APIKey) == "" {
			continue
		}
		next[i].APIKey = ""
		cleared = true
	}
	if !cleared {
		return fmt.Errorf("未找到 Proxy-LB 令牌或已清空")
	}
	_, err = SaveLLMConfigForm(LLMConfigRow{}, next)
	return err
}
