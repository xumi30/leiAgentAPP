package utils

import (
	"encoding/json"
	"strings"
)

type needToolTopicsPayload struct {
	NeedToolToics  []string `json:"needToolToics"`
	NeedToolTopics []string `json:"needToolTopics"`
}

type ToolCompletePayload struct {
	NeedToolToics           []string
	NeedToolTopicsRequested bool
	Content                 string
	SummaryForNextLLM       string
}

func hasAnyJSONKey(obj map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if raw, ok := obj[key]; ok && len(raw) > 0 {
			return true
		}
	}
	return false
}

// GetNeedToolToics 从模型回复中解析需要补充加载的工具话题。
// 为兼容现有 prompt，优先读取 needToolToics，同时兼容拼写正确的 needToolTopics。
func GetNeedToolToics(raw string) ([]string, bool) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil, false
	}

	if payload, ok := ParseToolCompletePayload(candidate); ok {
		return normalizeNeedToolTopics(payload.NeedToolToics)
	}

	if topics, ok := parseNeedToolTopicsFromJSON(candidate); ok {
		return topics, true
	}

	return extractNeedToolTopicsFromText(candidate)
}

func GetNeedToolToicsFromPayload(payload ToolCompletePayload) ([]string, bool) {
	return normalizeNeedToolTopics(payload.NeedToolToics)
}

func ParseToolCompletePayload(raw string) (ToolCompletePayload, bool) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return ToolCompletePayload{}, false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(PrepareLLMJSON(candidate)), &obj); err != nil {
		return ToolCompletePayload{}, false
	}

	// 只有显式带有 tool-complete 控制字段时，才当作工具续问 payload。
	// 普通结构化 JSON（例如意图识别结果）也常包含 content 字段，不能仅凭 content 误判。
	if !hasAnyJSONKey(obj,
		"needToolToics", "needToolTopics",
		"summaryfornextllm", "summaryForNextLLM", "summaryForNextLlm", "summary_for_next_llm",
	) {
		return ToolCompletePayload{}, false
	}

	var payload ToolCompletePayload
	payload.NeedToolToics, payload.NeedToolTopicsRequested = parseFlexibleToolTopics(obj["needToolToics"])
	if len(payload.NeedToolToics) == 0 {
		if topics, requested := parseFlexibleToolTopics(obj["needToolTopics"]); len(topics) > 0 || requested {
			payload.NeedToolToics = topics
			payload.NeedToolTopicsRequested = requested
		}
	}

	payload.Content = firstJSONString(obj, "content", "message")
	payload.SummaryForNextLLM = firstJSONString(obj, "summaryfornextllm", "summaryForNextLLM", "summaryForNextLlm", "summary_for_next_llm")

	if len(payload.NeedToolToics) == 0 && !payload.NeedToolTopicsRequested && payload.Content == "" && payload.SummaryForNextLLM == "" {
		return ToolCompletePayload{}, false
	}
	return payload, true
}

func parseFlexibleToolTopics(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var topics []string
	if err := json.Unmarshal(raw, &topics); err == nil {
		normalized, ok := normalizeNeedToolTopics(topics)
		return normalized, ok
	}

	var requested bool
	if err := json.Unmarshal(raw, &requested); err == nil {
		return nil, requested
	}

	var topic string
	if err := json.Unmarshal(raw, &topic); err == nil {
		normalized, ok := normalizeNeedToolTopics([]string{topic})
		return normalized, ok
	}

	return nil, false
}

func firstJSONString(obj map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok || len(raw) == 0 {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func parseNeedToolTopicsFromJSON(raw string) ([]string, bool) {
	var payload needToolTopicsPayload
	if err := json.Unmarshal([]byte(PrepareLLMJSON(raw)), &payload); err != nil {
		return nil, false
	}

	topics := payload.NeedToolToics
	if len(topics) == 0 {
		topics = payload.NeedToolTopics
	}

	return normalizeNeedToolTopics(topics)
}

func extractNeedToolTopicsFromText(raw string) ([]string, bool) {
	// Text fallback is intentionally strict to avoid false-positives.
	// The previous implementation matched any topic word (e.g. "时间") anywhere in normal assistant replies,
	// which accidentally triggered extra tool-loading loops and duplicated final responses.
	if !strings.Contains(raw, "needToolToics") && !strings.Contains(raw, "needToolTopics") {
		return nil, false
	}

	// Best-effort parse formats like:
	// {needToolToics:[时间, 搜索], message:"..."}
	// { "needToolTopics": ["时间","搜索"] }
	lo := raw
	start := strings.Index(lo, "[")
	end := strings.Index(lo, "]")
	if start < 0 || end < 0 || end <= start {
		return nil, false
	}
	body := lo[start+1 : end]
	parts := strings.FieldsFunc(body, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	topics := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p == "" {
			continue
		}
		topics = append(topics, p)
	}
	return normalizeNeedToolTopics(topics)
}

func normalizeNeedToolTopics(topics []string) ([]string, bool) {
	if len(topics) == 0 {
		return nil, false
	}

	allowed := make(map[string]struct{}, len(ToolTopics))
	for _, topic := range ToolTopics {
		allowed[strings.TrimSpace(topic)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(topics))
	result := make([]string, 0, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if _, ok := allowed[topic]; !ok {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		result = append(result, topic)
	}

	return result, len(result) > 0
}
