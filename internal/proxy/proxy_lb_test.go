package proxy

import (
	"testing"
)

func TestAppendProxyLbFallbackBackendPreservesOrderAndAppends(t *testing.T) {
	primary := LLMConfigRow{Name: "my-api", APIKey: "k1", BaseURL: "https://a.com/v1/chat/completions", Model: "x", Enabled: true}
	rows := []LLMConfigRow{primary}
	out := AppendProxyLbFallbackBackend(rows, "tok")
	if len(out) != 2 {
		t.Fatalf("len got %d want 2", len(out))
	}
	if out[0].Name != "my-api" {
		t.Fatalf("first backend should stay primary, got %+v", out[0])
	}
	last := out[len(out)-1]
	if last.Name != "proxy-lb" || last.APIKey != "tok" {
		t.Fatalf("last row want proxy-lb with token, got %+v", last)
	}
	if last.BaseURL != ProxyLBDisplayBaseURL {
		t.Fatalf("base url: got %q want %q", last.BaseURL, ProxyLBDisplayBaseURL)
	}
	if last.Model != ProxyLBDisplayModel {
		t.Fatalf("model: got %q want %q", last.Model, ProxyLBDisplayModel)
	}
}

func TestAppendProxyLbFallbackBackendReplacesExistingCanonicalRow(t *testing.T) {
	primary := LLMConfigRow{Name: "my-api", Enabled: true}
	oldProxy := LLMConfigRow{
		Name:       "proxy-lb",
		APIKey:     "old",
		BaseURL:    ProxyLBDisplayBaseURL,
		Model:      ProxyLBDisplayModel,
		StreamMode: "both",
		Enabled:    true,
	}
	rows := []LLMConfigRow{primary, oldProxy}
	out := AppendProxyLbFallbackBackend(rows, "newtok")
	if len(out) != 2 {
		t.Fatalf("len got %d want 2", len(out))
	}
	if out[1].APIKey != "newtok" {
		t.Fatalf("want refreshed token, got %+v", out[1])
	}
}

func TestProxyLBLoginRegisterToken(t *testing.T) {
	if got := ProxyLBLoginRegisterToken(map[string]interface{}{"username": "u", "token": "abc"}); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := ProxyLBLoginRegisterToken(map[string]interface{}{"message": "ok"}); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
