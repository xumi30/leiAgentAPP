package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestMergeLLMRow_GlobalEnvOverridesSingleBackend(t *testing.T) {
	t.Setenv("LEIAGENT_LLM_API_KEY", "k_env")
	t.Setenv("LEIAGENT_LLM_BASE_URL", "https://env.example/v1/chat/completions")
	t.Setenv("LEIAGENT_LLM_MODEL", "m_env")
	t.Setenv("LEIAGENT_LLM_PROVIDER", "gemini")
	t.Setenv("LEIAGENT_LLM_STREAM_MODE", "nonstream")

	b, err := mergeLLMRow(llmYAML{
		APIKey:     "",
		BaseURL:    "https://file.example/v1/chat/completions",
		Model:      "m_file",
		Provider:   "",
		StreamMode: "",
	}, true)
	if err != nil {
		t.Fatalf("mergeLLMRow returned error: %v", err)
	}
	if b.APIKey != "k_env" {
		t.Fatalf("APIKey = %q, want %q", b.APIKey, "k_env")
	}
	if b.BaseURL != "https://env.example/v1/chat/completions" {
		t.Fatalf("BaseURL = %q", b.BaseURL)
	}
	if b.Model != "m_env" {
		t.Fatalf("Model = %q", b.Model)
	}
	if b.Provider != "gemini" {
		t.Fatalf("Provider = %q", b.Provider)
	}
	if b.StreamMode != StreamModeNonStream {
		t.Fatalf("StreamMode = %d", b.StreamMode)
	}
}

func TestResolveConfigPath_EnvTakesPrecedence(t *testing.T) {
	// Ensure env path is accepted only if it points to a file.
	tmp := t.TempDir()
	p := tmp + "/config.yaml"
	if err := os.WriteFile(p, []byte("# temp config\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("LEIAGENT_CONFIG_PATH", p)

	got, ok := resolveConfigPath()
	if !ok {
		t.Fatalf("resolveConfigPath ok=false")
	}
	if got != p {
		t.Fatalf("resolveConfigPath=%q, want %q", got, p)
	}
}

func TestCircuitBreaker_SkipsOpenBackend(t *testing.T) {
	var badHits int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&badHits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer good.Close()

	c := &Client{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		backends: []Backend{
			{Name: "bad", BaseURL: bad.URL, APIKey: "k", Provider: ""},
			{Name: "good", BaseURL: good.URL, APIKey: "k", Provider: ""},
		},
		retry: defaultRetryPolicy().normalized(),
		breaker: CircuitBreakerPolicy{
			Enabled:        true,
			FailuresToOpen: 1,
			CoolDown:       5 * time.Second,
		}.normalized(),
		states: make([]backendState, 2),
	}
	c.retry.MaxAttempts = 1 // keep deterministic

	ctx := context.Background()
	req := Request{BodyBytes: []byte(`{}`)}

	// 1st call: bad backend fails -> opens; should then succeed with good.
	resp, b, err := c.DoRequest(ctx, req)
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	_ = resp.Body.Close()
	if b.Name != "good" {
		t.Fatalf("backend=%q want %q", b.Name, "good")
	}
	if atomic.LoadInt32(&badHits) != 1 {
		t.Fatalf("badHits=%d want 1", atomic.LoadInt32(&badHits))
	}

	// 2nd call immediately: bad backend should be skipped due to open circuit.
	resp2, b2, err := c.DoRequest(ctx, req)
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	_ = resp2.Body.Close()
	if b2.Name != "good" {
		t.Fatalf("backend=%q want %q", b2.Name, "good")
	}
	if atomic.LoadInt32(&badHits) != 1 {
		t.Fatalf("badHits=%d want still 1", atomic.LoadInt32(&badHits))
	}
}
