package utils

import "testing"

func TestParseToolCompletePayload_RequiresControlFields(t *testing.T) {
	raw := `{"maingoal":"规划明天南京旅行行程","intent":"PLAN","confidence":0.85,"reason":"ok","requires_clarification":true,"content":"请补充出发时间","tool_source":"local","tooltopic":"搜索","sub_intents":[]}`

	if payload, ok := ParseToolCompletePayload(raw); ok {
		t.Fatalf("ParseToolCompletePayload() unexpectedly matched: %#v", payload)
	}
}

func TestParseToolCompletePayload_MatchesRealPayload(t *testing.T) {
	raw := `{"needToolToics":[],"content":"已完成","summaryfornextllm":"任务完成"}`

	payload, ok := ParseToolCompletePayload(raw)
	if !ok {
		t.Fatalf("ParseToolCompletePayload() = false, want true")
	}
	if payload.Content != "已完成" {
		t.Fatalf("payload.Content = %q, want 已完成", payload.Content)
	}
	if payload.SummaryForNextLLM != "任务完成" {
		t.Fatalf("payload.SummaryForNextLLM = %q, want 任务完成", payload.SummaryForNextLLM)
	}
}
