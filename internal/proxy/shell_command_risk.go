package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
)

const shellRiskSystemPrompt = `你是命令行安全分级助手。用户会提供一段即将在桌面助手中用于黑名单匹配的「命令子串」或「正则表达式」。
请仅根据该片段在真实执行时可能造成的危害，给出三档之一：high（高）、medium（中）、low（低）。
判定参考（不必向用户展开长文）：
- high：典型删库删盘、格式化存储、进程炸弹、强制批量删除、关机等系统级破坏或类似不可逆高危
- medium：可能影响系统配置、网络与软件环境、批量文件变更等需格外谨慎的操作
- low：相对常规的查看/构建类片段，但仍可能因上下文变危险

输出要求：只输出一行 JSON 对象，不要 markdown 代码块，不要其它文字。字段严格为：
{"comment":"用一句话简述理由（中文）","risklevel":"high"}
其中 risklevel 必须是 high、medium、low 之一。comment 为简短说明。`

// ShellCommandRiskResult LLM 危险度评估结果（供设置页展示）。
type ShellCommandRiskResult struct {
	OK        bool   `json:"ok"`
	Severity  string `json:"severity,omitempty"`  // 归一化档位，与表格 severity 列一致
	RiskLevel string `json:"risklevel,omitempty"` // 与 Severity 同值，便于前端展示
	Comment   string `json:"comment,omitempty"`
	Message   string `json:"message,omitempty"`
}

var (
	jsonSeverityPattern      = regexp.MustCompile(`(?i)"severity"\s*:\s*"(high|medium|low)"`)
	looseSeverityWordPattern = regexp.MustCompile(`(?i)\b(high|medium|low)\b`)
)

func parseSeverityFromModelText(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	// 去常见 markdown 围栏
	if strings.Count(s, "```") >= 2 {
		start := strings.Index(s, "```")
		rest := s[start+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			s = strings.TrimSpace(rest[:end])
		}
	}
	s = strings.TrimSpace(s)

	var payload struct {
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal([]byte(s), &payload); err == nil {
		if norm, ok := normalizeSeverityToken(payload.Severity); ok {
			return norm, true
		}
	}
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			chunk := strings.TrimSpace(s[i : j+1])
			if err := json.Unmarshal([]byte(chunk), &payload); err == nil {
				if norm, ok := normalizeSeverityToken(payload.Severity); ok {
					return norm, true
				}
			}
		}
	}
	if m := jsonSeverityPattern.FindStringSubmatch(s); len(m) == 2 {
		if norm, ok := normalizeSeverityToken(m[1]); ok {
			return norm, true
		}
	}
	if m := looseSeverityWordPattern.FindStringSubmatch(s); len(m) == 2 {
		if norm, ok := normalizeSeverityToken(m[1]); ok {
			return norm, true
		}
	}
	return "", false
}

func parseShellRiskFromModelText(raw string) (sev string, comment string, ok bool) {
	type payload struct {
		Comment   string `json:"comment"`
		RiskLevel string `json:"risklevel"`
		Severity  string `json:"severity"`
	}
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", false
	}
	if strings.Count(s, "```") >= 2 {
		start := strings.Index(s, "```")
		rest := s[start+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			s = strings.TrimSpace(rest[:end])
		}
	}
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = strings.TrimSpace(s[i : j+1])
		}
	}
	var p payload
	if err := json.Unmarshal([]byte(s), &p); err == nil {
		comment = strings.TrimSpace(p.Comment)
		if rl := strings.TrimSpace(p.RiskLevel); rl != "" {
			if norm, ok2 := normalizeSeverityToken(rl); ok2 {
				return norm, comment, true
			}
		}
		if sv := strings.TrimSpace(p.Severity); sv != "" {
			if norm, ok2 := normalizeSeverityToken(sv); ok2 {
				return norm, comment, true
			}
		}
	}
	if sev2, ok2 := parseSeverityFromModelText(raw); ok2 {
		return sev2, "", true
	}
	return "", "", false
}

func normalizeSeverityToken(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high", "h", "高":
		return "high", true
	case "low", "l", "低":
		return "low", true
	case "medium", "m", "中", "mid":
		return "medium", true
	default:
		return "", false
	}
}

// AssessShellCommandRisk 调用已配置 LLM 对命令片段做 high/medium/low 分级；失败时 ok=false 由前端提示用户自行判定。
func AssessShellCommandRisk(ctx context.Context, commandLine string) ShellCommandRiskResult {
	cmd := strings.TrimSpace(commandLine)
	if cmd == "" {
		return ShellCommandRiskResult{OK: false, Message: "请先填写命令"}
	}
	snippet := cmd
	if r := []rune(snippet); len(r) > 2000 {
		snippet = string(r[:2000])
	}

	const llmUnavailable = "大模型当前不可用，无法自动评估危险度，请自行判定。"
	const parseFailed = "模型未返回有效分级，无法自动填写危险度，请自行判定。"

	client := &http.Client{Timeout: 45 * time.Second}
	p, err := NewClient(client)
	if err != nil {
		logging.Warn("shell 危险度评估：无法创建 LLM 代理: %v", err)
		return ShellCommandRiskResult{OK: false, Message: llmUnavailable}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 42*time.Second)
		defer cancel()
	}

	text, err := p.completeText(ctx, []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: shellRiskSystemPrompt},
		{Role: openaistyle.RoleUser, Content: "待评估片段如下（仅用于分级，不要执行）：\n" + snippet},
	}, 384, 0.2)
	if err != nil {
		logging.Warn("shell 危险度评估失败: %v", err)
		return ShellCommandRiskResult{OK: false, Message: llmUnavailable}
	}
	severity, comment, ok := parseShellRiskFromModelText(text)
	if !ok {
		logging.Warn("shell 危险度评估输出无法解析: %q", strings.TrimSpace(text))
		return ShellCommandRiskResult{OK: false, Message: parseFailed}
	}
	return ShellCommandRiskResult{OK: true, Severity: severity, RiskLevel: severity, Comment: comment}
}
