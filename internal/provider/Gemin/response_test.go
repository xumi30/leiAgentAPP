package gemini

import "testing"

func TestConvertToOpenAIResponse_ConcatenatesMultipleTextParts(t *testing.T) {
	resp := ConvertToOpenAIResponse(&ChatCompletionResponse{
		Candidates: []Candidate{
			{
				Content: Content{
					Role: "model",
					Parts: []Part{
						{Text: "第一段"},
						{Text: "第二段"},
						{
							FunctionCall: &FunctionCall{
								Name: "search",
								Args: map[string]interface{}{"q": "demo"},
							},
						},
						{Text: "第三段"},
					},
				},
				FinishReason: "STOP",
			},
		},
	})

	if len(resp.Choices) != 1 || resp.Choices[0].Message == nil {
		t.Fatalf("expected one choice with message, got %#v", resp.Choices)
	}

	got, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %#v", resp.Choices[0].Message.Content)
	}
	if got != "第一段第二段第三段" {
		t.Fatalf("unexpected content: %q", got)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", resp.Choices[0].Message.ToolCalls)
	}
}
