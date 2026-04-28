package compressor

import (
	"testing"

	"leiAgent/internal/memory"
)

func TestCompressRulesOnlyDoesNotMutateInput(t *testing.T) {
	raw := []*memory.Message{
		{Role: memory.MessageRoleSystem, Content: "sys"},
		{Role: memory.MessageRoleUser, Content: "# Title\n结论: ok\nerror: boom"},
		{Role: memory.MessageRoleAssistant, Content: "next"},
	}
	orig0 := raw[1].Content

	art := CompressRulesOnly(raw, Options{ChatID: "c"})
	if art.Version != ArtifactV1 {
		t.Fatalf("expected version %d got %d", ArtifactV1, art.Version)
	}
	if raw[1].Content != orig0 {
		t.Fatalf("input mutated")
	}
	if art.Outputs.TLDR == "" {
		t.Fatalf("expected non-empty tldr")
	}
	if len(art.SelectedSegments) == 0 {
		t.Fatalf("expected selected segments")
	}
}

