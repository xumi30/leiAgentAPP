package compressor

import "time"

type ArtifactVersion int

const ArtifactV1 ArtifactVersion = 1

// CompressedArtifact is the persisted schema for compress/{chatID}.yaml.
// Keep it stable and backward-compatible; bump Version when breaking changes are unavoidable.
type CompressedArtifact struct {
	Version     ArtifactVersion `yaml:"version"`
	ChatID      string          `yaml:"chat_id"`
	GeneratedAt time.Time       `yaml:"generated_at"`

	Source SourceMeta `yaml:"source"`
	// PolicySnapshot stores a small subset of config for debugging/repro.
	PolicySnapshot map[string]any `yaml:"policy_snapshot,omitempty"`
	ContextRecipe  ContextRecipe  `yaml:"context_recipe"`

	Outputs Outputs `yaml:"outputs"`

	// SelectedSegments keeps optional evidence of rule-based selection.
	SelectedSegments []SelectedSegment `yaml:"selected_segments,omitempty"`
}

type SourceMeta struct {
	RawMessageCount int `yaml:"raw_message_count"`
}

type ContextRecipe struct {
	SystemCard         bool `yaml:"system_card"`
	RecentTailMessages int  `yaml:"recent_tail_messages"`
}

type Outputs struct {
	TLDR    string   `yaml:"tldr,omitempty"`
	Bullets []string `yaml:"bullets,omitempty"`
	Card    Card     `yaml:"card,omitempty"`
	Stats   any      `yaml:"stats,omitempty"`
}

type Card struct {
	Problem  string   `yaml:"problem,omitempty"`
	Cause    string   `yaml:"cause,omitempty"`
	Evidence []string `yaml:"evidence,omitempty"`
	Actions  []string `yaml:"actions,omitempty"`
}

type SelectedSegment struct {
	Kind    string `yaml:"kind"` // e.g. headings/conclusion/error_stack/tail_lines
	Snippet string `yaml:"snippet"`
}
