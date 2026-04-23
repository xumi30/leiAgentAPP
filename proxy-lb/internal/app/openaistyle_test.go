package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolChoice_Unmarshal_String(t *testing.T) {
	var req ChatCompletionRequest
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}],"tool_choice":"auto"}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req.ToolChoice == nil || req.ToolChoice.Type != ToolChoiceAuto {
		t.Fatalf("expected tool_choice auto, got %#v", req.ToolChoice)
	}
}

func TestToolChoice_Unmarshal_Object(t *testing.T) {
	var req ChatCompletionRequest
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"none"}}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req.ToolChoice == nil || req.ToolChoice.Type != ToolChoiceNone {
		t.Fatalf("expected tool_choice none, got %#v", req.ToolChoice)
	}
}

func TestToolCall_FunctionArguments_StringifiedJSON(t *testing.T) {
	var req ChatCompletionRequest
	body := []byte(`{
		"model":"x",
		"messages":[
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"call_1","type":"function","index":0,"function":{"name":"f","arguments":"{\"a\":1}"}}
			]}
		]
	}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected tool_calls: %#v", req.Messages)
	}
	fn := req.Messages[0].ToolCalls[0].Function
	if fn == nil || fn.Arguments == nil || fn.Arguments["a"] != float64(1) {
		t.Fatalf("expected parsed arguments, got %#v", fn)
	}
}

func TestTools_FunctionArguments_OmitEmpty(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "x",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Tools: []Tool{{
			Type:     "function",
			Function: &Function{Name: "f", Description: "d"},
		}},
	}
	out, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(out)
	if strings.Contains(s, `"arguments":null`) || strings.Contains(s, `"parameters":null`) {
		t.Fatalf("should not emit null arguments/parameters, got %s", s)
	}
}

