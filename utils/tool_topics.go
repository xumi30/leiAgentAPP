package utils

import (
	"encoding/json"
	"strings"
)

type needToolTopicsPayload struct {
	NeedToolToics  []string `json:"needToolToics"`
	NeedToolTopics []string `json:"needToolTopics"`
}

// GetNeedToolToics 从模型回复中解析需要补充加载的工具话题。
// 为兼容现有 prompt，优先读取 needToolToics，同时兼容拼写正确的 needToolTopics。
func GetNeedToolToics(raw string) ([]string, bool) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil, false
	}

	if topics, ok := parseNeedToolTopicsFromJSON(candidate); ok {
		return topics, true
	}

	return extractNeedToolTopicsFromText(candidate)
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
