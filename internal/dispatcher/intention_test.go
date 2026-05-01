package dispatcher

import (
	"testing"

	"leiAgent/internal/proxy"
)

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

func TestInterpretActionGateResult_EmptyChatResponseFallsBack(t *testing.T) {
	needAction, handled := interpretActionGateResult(&proxy.ToolAndContent{
		Content:    " \n\t",
		NeedAction: false,
	})

	if needAction {
		t.Fatal("needAction = true, want false")
	}
	if handled {
		t.Fatal("handled = true, want false so the full intent/chat path can respond")
	}
}

func TestInterpretActionGateResult_ChatContentIsHandled(t *testing.T) {
	needAction, handled := interpretActionGateResult(&proxy.ToolAndContent{
		Content:    "你好，有什么想聊的？",
		NeedAction: false,
	})

	if needAction || !handled {
		t.Fatalf("needAction, handled = %v, %v; want false, true", needAction, handled)
	}
}

func TestInterpretActionGateResult_ActionContinues(t *testing.T) {
	needAction, handled := interpretActionGateResult(&proxy.ToolAndContent{
		NeedAction: true,
	})

	if !needAction || !handled {
		t.Fatalf("needAction, handled = %v, %v; want true, true", needAction, handled)
	}
}

func TestIntentUserContent_RemovesPersistencePrefix(t *testing.T) {
	tests := map[string]string{
		"User:?":         "?",
		"User： ? ":       "?",
		"用户请求: 看看日志":     "看看日志",
		"用户请求：看看日志":      "看看日志",
		"unchanged text": "unchanged text",
	}

	for input, want := range tests {
		if got := intentUserContent(input); got != want {
			t.Errorf("intentUserContent(%q) = %q, want %q", input, got, want)
		}
	}
}
