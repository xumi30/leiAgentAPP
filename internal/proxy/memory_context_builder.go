package proxy

import (
	"fmt"
	"strings"

	"leiAgent/internal/memory"
	"leiAgent/internal/memory/compressor"
	"leiAgent/internal/memory/compressstore"
)

// BuildMemoryMessagesForLLM returns either:
// - (overrideMessages, true, nil) when compress artifact is found and enabled
// - (nil, false, nil) when should fallback to raw memory
func BuildMemoryMessagesForLLM(chatID string, raw []*memory.Message) ([]*memory.Message, bool, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return nil, false, nil
	}
	cfg, err := LoadMemoryCompressionConfig()
	if err != nil {
		return nil, false, err
	}
	if !cfg.Enabled {
		return nil, false, nil
	}

	var art compressor.CompressedArtifact
	ok, _, err := compressstore.LoadYAML(cfg.PersistDir, cid, &art)
	if err != nil || !ok {
		// If file missing or unreadable, fallback to raw.
		return nil, false, nil
	}
	cardRole := normalizeRole(cfg.Context.SystemCardRole)
	if cardRole == "" {
		cardRole = memory.MessageRoleSystem
	}
	card := renderSystemCard(cfg.Context.SystemCardPrefix, &art)
	if strings.TrimSpace(card) == "" {
		return nil, false, nil
	}

	tail := pickRecentTail(raw, cfg.Context.RecentTailMessages)
	out := make([]*memory.Message, 0, 1+len(tail))
	out = append(out, &memory.Message{Role: cardRole, Content: card})
	out = append(out, tail...)
	return out, true, nil
}

func normalizeRole(s string) memory.MessageRole {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "system":
		return memory.MessageRoleSystem
	case "user":
		return memory.MessageRoleUser
	case "assistant":
		return memory.MessageRoleAssistant
	default:
		return ""
	}
}

func renderSystemCard(prefix string, art *compressor.CompressedArtifact) string {
	if art == nil {
		return ""
	}
	p := prefix
	if strings.TrimSpace(p) == "" {
		p = "【压缩记忆】\n"
	}
	var b strings.Builder
	b.WriteString(p)
	if t := strings.TrimSpace(art.Outputs.TLDR); t != "" {
		fmt.Fprintf(&b, "TL;DR: %s\n", t)
	}
	if len(art.Outputs.Bullets) > 0 {
		b.WriteString("要点:\n")
		for i, it := range art.Outputs.Bullets {
			it = strings.TrimSpace(it)
			if it == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", it)
			if i >= 9 { // hard cap
				break
			}
		}
	}
	c := art.Outputs.Card
	if strings.TrimSpace(c.Problem) != "" || strings.TrimSpace(c.Cause) != "" || len(c.Evidence) > 0 || len(c.Actions) > 0 {
		b.WriteString("结构化卡片:\n")
		if s := strings.TrimSpace(c.Problem); s != "" {
			fmt.Fprintf(&b, "- 问题: %s\n", s)
		}
		if s := strings.TrimSpace(c.Cause); s != "" {
			fmt.Fprintf(&b, "- 原因: %s\n", s)
		}
		if len(c.Evidence) > 0 {
			b.WriteString("- 证据:\n")
			for i, e := range c.Evidence {
				e = strings.TrimSpace(e)
				if e == "" {
					continue
				}
				fmt.Fprintf(&b, "  - %s\n", e)
				if i >= 4 {
					break
				}
			}
		}
		if len(c.Actions) > 0 {
			b.WriteString("- 行动项:\n")
			for i, a := range c.Actions {
				a = strings.TrimSpace(a)
				if a == "" {
					continue
				}
				fmt.Fprintf(&b, "  - %s\n", a)
				if i >= 4 {
					break
				}
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func pickRecentTail(raw []*memory.Message, n int) []*memory.Message {
	if n <= 0 {
		n = 8
	}
	// Keep last N messages excluding system, keep tool/user/assistant.
	tmp := make([]*memory.Message, 0, n)
	for i := len(raw) - 1; i >= 0 && len(tmp) < n; i-- {
		m := raw[i]
		if m == nil {
			continue
		}
		if m.Role == memory.MessageRoleSystem {
			continue
		}
		// copy to avoid mutation surprises
		cp := *m
		if len(m.ToolCalls) > 0 {
			cp.ToolCalls = append([]memory.ToolCall(nil), m.ToolCalls...)
		}
		tmp = append(tmp, &cp)
	}
	// reverse to chronological
	for i, j := 0, len(tmp)-1; i < j; i, j = i+1, j-1 {
		tmp[i], tmp[j] = tmp[j], tmp[i]
	}
	return tmp
}

