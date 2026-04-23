package app

import (
	"testing"
	"time"
)

func TestBackendFailureTracker_ReorderCandidates(t *testing.T) {
	tr := newBackendFailureTracker()
	fixedNow := time.Date(2026, 4, 23, 6, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return fixedNow }

	a := &backendRuntime{cfg: BackendConfig{Model: "m1", BaseURL: "u1", APIKey: "k1", Name: "a"}}
	b := &backendRuntime{cfg: BackendConfig{Model: "m2", BaseURL: "u2", APIKey: "k2", Name: "b"}}
	c := &backendRuntime{cfg: BackendConfig{Model: "m3", BaseURL: "u3", APIKey: "k3", Name: "c"}}

	in := []*backendRuntime{a, b, c}
	// Penalize b
	tr.recordFailure(backendKey(b.cfg))

	out := tr.reorderCandidates(in)
	if len(out) != 3 {
		t.Fatalf("unexpected len=%d", len(out))
	}
	if out[0] == b || out[2] != b {
		t.Fatalf("expected b moved to end, got order=%v,%v,%v", out[0].cfg.Name, out[1].cfg.Name, out[2].cfg.Name)
	}
	// Stable for non-penalized: a then c should remain.
	if out[0] != a || out[1] != c {
		t.Fatalf("expected stable order for non-penalized, got %v,%v", out[0].cfg.Name, out[1].cfg.Name)
	}

	// After cooldown passes, b should no longer be penalized and ordering should be unchanged (stable sort won't move it).
	tr.now = func() time.Time { return fixedNow.Add(2 * time.Minute) }
	out2 := tr.reorderCandidates(in)
	if out2[0] != a || out2[1] != b || out2[2] != c {
		t.Fatalf("expected no penalty after cooldown, got %v,%v,%v", out2[0].cfg.Name, out2[1].cfg.Name, out2[2].cfg.Name)
	}
}

