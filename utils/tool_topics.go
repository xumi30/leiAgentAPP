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
	topics := make([]string, 0, len(ToolTopics))
	for _, topic := range ToolTopics {
		if strings.Contains(raw, topic) {
			topics = append(topics, topic)
		}
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
