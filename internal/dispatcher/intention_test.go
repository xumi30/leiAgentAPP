package dispatcher

import "testing"

func TestParseIntention_ParsesRawJSONDirectly(t *testing.T) {
	raw := `{"maingoal":"规划明天南京旅行行程","intent":"PLAN","confidence":0.85,"reason":"ok","requires_clarification":true,"content":"请补充出发时间","tool_source":"local","tooltopic":"搜索","sub_intents":[]}`

	got, err := parseIntention(raw)
	if err != nil {
		t.Fatalf("parseIntention() error = %v", err)
	}
	if got.Intent != "PLAN" {
		t.Fatalf("Intent = %q, want PLAN", got.Intent)
	}
	if !got.RequiresClarification {
		t.Fatalf("RequiresClarification = false, want true")
	}
}

func TestParseIntention_FallsBackToPreparedJSON(t *testing.T) {
	raw := "data: \ufeff{\"maingoal\":\"规划明天南京旅行行程\",\"intent\":\"PLAN\",\"confidence\":0.85,\"reason\":\"ok\",\"requires_clarification\":true,\"content\":\"请补充出发时间\",\"tool_source\":\"local\",\"tooltopic\":\"搜索\",\"sub_intents\":[]}\n\ndata: [DONE]"

	got, err := parseIntention(raw)
	if err != nil {
		t.Fatalf("parseIntention() error = %v", err)
	}
	if got.Intent != "PLAN" {
		t.Fatalf("Intent = %q, want PLAN", got.Intent)
	}
}
