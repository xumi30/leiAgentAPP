package app

import "testing"

func TestConvertGeminiMessages_MultimodalTextParts(t *testing.T) {
	msgs := []ChatMessage{{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "hello"},
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.test/x.png"}},
			map[string]interface{}{"type": "text", "text": "world"},
		},
	}}

	out := convertGeminiMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("expected 1 content, got %d", len(out))
	}
	if len(out[0].Parts) != 1 || out[0].Parts[0].Text == "" {
		t.Fatalf("expected one text part, got %#v", out[0].Parts)
	}
	if out[0].Parts[0].Text != "hello\n[unsupported image_url] https://example.test/x.png\nworld" {
		t.Fatalf("unexpected text: %q", out[0].Parts[0].Text)
	}
}

