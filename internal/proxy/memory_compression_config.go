package proxy

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// MemoryCompressionYAML mirrors config YAML under `memory_compression`.
// It is intentionally decoupled from runtime logic to keep config parsing simple and stable.
type MemoryCompressionYAML struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	// PersistDir is a directory relative to current working directory (same convention as localmemory/).
	// Default: "compress"
	PersistDir string `yaml:"persist_dir,omitempty"`

	Trigger MemoryCompressionTriggerYAML `yaml:"trigger,omitempty"`
	Context MemoryCompressionContextYAML `yaml:"context,omitempty"`

	// Pipeline/Outputs are reserved for future stages; first iteration focuses on stable defaults.
	Pipeline MemoryCompressionPipelineYAML `yaml:"pipeline,omitempty"`
	Outputs  MemoryCompressionOutputsYAML  `yaml:"outputs,omitempty"`
}

type MemoryCompressionTriggerYAML struct {
	EveryAssistantTurns int `yaml:"every_assistant_turns,omitempty"`
	YAMLMessageThreshold int `yaml:"yaml_message_threshold,omitempty"`
}

type MemoryCompressionContextYAML struct {
	RecentTailMessages int    `yaml:"recent_tail_messages,omitempty"`
	SystemCardRole     string `yaml:"system_card_role,omitempty"`   // default "system"
	SystemCardPrefix   string `yaml:"system_card_prefix,omitempty"` // default "【压缩记忆】\n"
}

type MemoryCompressionPipelineYAML struct {
	StructuredFirst bool `yaml:"structured_first,omitempty"`
	// Future: whitelist/crop/dedupe/stats/segment_select/llm...
}

type MemoryCompressionOutputsYAML struct {
	TLDRSentences int `yaml:"tldr_sentences,omitempty"`
	BulletMax     int `yaml:"bullet_max,omitempty"`
}

// ResolvedMemoryCompressionConfig is the runtime-ready config with defaults applied.
type ResolvedMemoryCompressionConfig struct {
	Enabled   bool
	PersistDir string

	Trigger struct {
		EveryAssistantTurns   int
		YAMLMessageThreshold  int
	}
	Context struct {
		RecentTailMessages int
		SystemCardRole     string
		SystemCardPrefix   string
	}
	Outputs struct {
		TLDRSentences int
		BulletMax     int
	}

	LoadedAt time.Time
	Source   string // "file" | "default" | "env_override"
}

func defaultResolvedMemoryCompressionConfig() ResolvedMemoryCompressionConfig {
	var c ResolvedMemoryCompressionConfig
	c.Enabled = false
	c.PersistDir = "compress"
	c.Trigger.EveryAssistantTurns = 10
	c.Trigger.YAMLMessageThreshold = 20
	c.Context.RecentTailMessages = 8
	c.Context.SystemCardRole = "system"
	c.Context.SystemCardPrefix = "【压缩记忆】\n"
	c.Outputs.TLDRSentences = 3
	c.Outputs.BulletMax = 10
	c.LoadedAt = time.Now()
	c.Source = "default"
	return c
}

// LoadMemoryCompressionConfig loads memory_compression from the same config YAML as LLM settings.
// It is safe to call frequently (same IO pattern as loadModelConfigs).
func LoadMemoryCompressionConfig() (ResolvedMemoryCompressionConfig, error) {
	cfg := defaultResolvedMemoryCompressionConfig()
	root, cfgPath, err := readConfigRoot()
	if err != nil {
		return cfg, fmt.Errorf("读取配置失败（%s）：%w", cfgPath, err)
	}
	if cfgPath == "" {
		return cfg, nil
	}
	if isBundledYAMLPathMarker(cfgPath) {
		cfg.Source = "embedded"
	} else {
		cfg.Source = "file"
	}
	cfg.LoadedAt = time.Now()

	mc := root.MemoryCompression
	cfg.Enabled = mc.Enabled
	if s := strings.TrimSpace(mc.PersistDir); s != "" {
		cfg.PersistDir = s
	}
	if mc.Trigger.EveryAssistantTurns > 0 {
		cfg.Trigger.EveryAssistantTurns = mc.Trigger.EveryAssistantTurns
	}
	if mc.Trigger.YAMLMessageThreshold > 0 {
		cfg.Trigger.YAMLMessageThreshold = mc.Trigger.YAMLMessageThreshold
	}
	if mc.Context.RecentTailMessages > 0 {
		cfg.Context.RecentTailMessages = mc.Context.RecentTailMessages
	}
	if s := strings.TrimSpace(mc.Context.SystemCardRole); s != "" {
		cfg.Context.SystemCardRole = s
	}
	if s := mc.Context.SystemCardPrefix; strings.TrimSpace(s) != "" {
		cfg.Context.SystemCardPrefix = s
	}
	if mc.Outputs.TLDRSentences > 0 {
		cfg.Outputs.TLDRSentences = mc.Outputs.TLDRSentences
	}
	if mc.Outputs.BulletMax > 0 {
		cfg.Outputs.BulletMax = mc.Outputs.BulletMax
	}

	// Optional env override for quick toggles without editing YAML.
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("LEIAGENT_MEMORY_COMPRESSION_ENABLED"))); v != "" {
		switch v {
		case "1", "true", "yes", "on":
			cfg.Enabled = true
			cfg.Source = "env_override"
		case "0", "false", "no", "off":
			cfg.Enabled = false
			cfg.Source = "env_override"
		}
	}
	return cfg, nil
}

