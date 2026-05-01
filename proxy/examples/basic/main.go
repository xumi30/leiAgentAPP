package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"leiAgent/proxy"
)

// This example demonstrates how to:
// - Create a proxy client from disk/env config (with default fallback)
// - Enable retry + circuit breaker + optional probe
// - Send a single OpenAI-compatible chat completion request
func main() {
	client, err := proxy.New(
		proxy.WithRetryPolicy(proxy.RetryPolicy{
			MaxAttempts: 2,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			JitterRatio: 0.2,
		}),
		proxy.WithCircuitBreaker(proxy.CircuitBreakerPolicy{
			Enabled:        true,
			FailuresToOpen: 3,
			CoolDown:       15 * time.Second,
		}),
		proxy.WithProber(proxy.ModelsProber{Timeout: 3 * time.Second}),
	)
	if err != nil {
		panic(err)
	}

	// OpenAI-compatible Chat Completions payload.
	// NOTE: model will be taken from your configured backend; many gateways accept it here too.
	payload := map[string]any{
		"model": clientModelHint(), // optional; safe to remove if your gateway ignores it
		"messages": []map[string]any{
			{"role": "system", "content": "You are a concise assistant."},
			{"role": "user", "content": "Say hello in one sentence."},
		},
		"stream": false,
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, backend, err := client.DoRequest(ctx, proxy.Request{
		BodyBytes:   body,
		ContentType: "application/json",
	})
	if err != nil {
		// You can inspect backend-aware errors via errors.As(err, *proxy.BackendError)
		panic(err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	fmt.Printf("backend=%s provider=%s status=%d\n", backend.Name, backend.Provider, resp.StatusCode)
	fmt.Println(string(data))
}

// clientModelHint exists only to keep the example payload closer to OpenAI format.
// If you don't need it, remove the "model" field in payload.
func clientModelHint() string {
	// keep empty by default: the configured backend already pins the model.
	return ""
}
