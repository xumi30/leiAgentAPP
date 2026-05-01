package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"leiAgent/internal/memory"
	"leiAgent/internal/memory/compressor"
	"leiAgent/internal/memory/compressstore"
)

func TestBuildMemoryMessagesForLLMFallbackWhenDisabled(t *testing.T) {
	raw := []*memory.Message{
		{Role: memory.MessageRoleUser, Content: "hi"},
	}
	t.Setenv("LEIAGENT_MEMORY_COMPRESSION_ENABLED", "false")

	msgs, ok, err := BuildMemoryMessagesForLLM("c1", raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatalf("expected fallback ok=false, got ok=true with %d msgs", len(msgs))
	}
}

func TestBuildMemoryMessagesForLLMUsesArtifactAndTail(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
memory_compression:
  enabled: true
  persist_dir: "compress"
  context:
    recent_tail_messages: 2
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LEIAGENT_CONFIG_PATH", cfgPath)

	// Make compress dir relative to cwd; run test in temp cwd.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	art := compressor.CompressedArtifact{
		Version:     compressor.ArtifactV1,
		ChatID:      "c2",
		GeneratedAt: time.Now().UTC(),
		Source:      compressor.SourceMeta{RawMessageCount: 2},
		ContextRecipe: compressor.ContextRecipe{
			SystemCard:         true,
			RecentTailMessages: 2,
		},
		Outputs: compressor.Outputs{
			TLDR:    "tldr text",
			Bullets: []string{"a", "b"},
			Card:    compressor.Card{Problem: "p"},
		},
	}
	if _, err := compressstore.PersistYAML("compress", "c2", &art); err != nil {
		t.Fatalf("persist: %v", err)
	}

	raw := []*memory.Message{
		{Role: memory.MessageRoleSystem, Content: "sys"},
		{Role: memory.MessageRoleUser, Content: "u1"},
		{Role: memory.MessageRoleAssistant, Content: "a1"},
		{Role: memory.MessageRoleUser, Content: "u2"},
	}
	out, ok, err := BuildMemoryMessagesForLLM("c2", raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if len(out) != 1+2 {
		t.Fatalf("expected 3 messages (1 card + 2 tail), got %d", len(out))
	}
	if out[0].Role != memory.MessageRoleSystem {
		t.Fatalf("expected system card role, got %s", out[0].Role)
	}
	if out[0].Content == "" {
		t.Fatalf("expected non-empty card content")
	}
	// Tail should be last 2 non-system in chronological order: a1, u2
	if out[1].Content != "a1" || out[2].Content != "u2" {
		t.Fatalf("unexpected tail: %#v %#v", out[1], out[2])
	}
}
