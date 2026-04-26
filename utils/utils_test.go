package utils

import "testing"

func TestPrepareLLMJSON_StripsSSEPrefixAndDone(t *testing.T) {
	raw := "data: \ufeff{\"intent\":\"PLAN\",\"content\":\"ok\"}\n\ndata: [DONE]"

	got := PrepareLLMJSON(raw)
	want := "{\"intent\":\"PLAN\",\"content\":\"ok\"}"
	if got != want {
		t.Fatalf("PrepareLLMJSON() = %q, want %q", got, want)
	}
}

func TestPrepareLLMJSON_ExtractsBalancedObjectFromPrefixNoise(t *testing.T) {
	raw := "说明文字在前 {\"intent\":\"PLAN\",\"content\":\"南京\",\"sub_intents\":[]} 尾部说明"

	got := PrepareLLMJSON(raw)
	want := "{\"intent\":\"PLAN\",\"content\":\"南京\",\"sub_intents\":[]}"
	if got != want {
		t.Fatalf("PrepareLLMJSON() = %q, want %q", got, want)
	}
}

func TestUnmarshalLLMJSON_FallsBackToPreparedPayload(t *testing.T) {
	raw := "data: \ufeff{\"intent\":\"PLAN\",\"content\":\"ok\"}\n\ndata: [DONE]"

	var got struct {
		Intent  string `json:"intent"`
		Content string `json:"content"`
	}
	if err := UnmarshalLLMJSON(raw, &got); err != nil {
		t.Fatalf("UnmarshalLLMJSON() error = %v", err)
	}
	if got.Intent != "PLAN" || got.Content != "ok" {
		t.Fatalf("UnmarshalLLMJSON() = %#v, want intent PLAN and content ok", got)
	}
}
