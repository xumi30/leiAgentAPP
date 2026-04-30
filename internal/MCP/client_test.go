package mcpbridge

import "testing"

func TestToolAllowedRequiresExplicitAllowlist(t *testing.T) {
	if toolAllowed(nil, "dangerous_write") {
		t.Fatal("nil allowlist should deny calls")
	}
	if toolAllowed([]string{}, "dangerous_write") {
		t.Fatal("empty allowlist should deny calls")
	}
	if !toolAllowed([]string{"safe_lookup"}, "safe_lookup") {
		t.Fatal("explicit allowlist should permit matching tool")
	}
}
