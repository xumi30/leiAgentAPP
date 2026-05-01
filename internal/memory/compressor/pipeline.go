package compressor

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"leiAgent/internal/memory"
)

type Options struct {
	ChatID string

	RecentTailMessages int
	SystemCardPrefix   string

	TLDRSentences int
	BulletMax     int
}

// CompressRulesOnly produces a multi-level compressed artifact without calling any LLM.
// It intentionally favors determinism and debuggability over prose quality.
func CompressRulesOnly(msgs []*memory.Message, opt Options) CompressedArtifact {
	opt = normalizeOptions(opt)
	filtered := filterMessages(msgs)
	selected, segs := selectSegments(filtered)
	tldr := buildTLDR(selected, opt.TLDRSentences)
	bullets := buildBullets(selected, opt.BulletMax)
	card := buildCard(selected)

	return CompressedArtifact{
		Version:     ArtifactV1,
		ChatID:      strings.TrimSpace(opt.ChatID),
		GeneratedAt: time.Now().UTC(),
		Source: SourceMeta{
			RawMessageCount: len(msgs),
		},
		PolicySnapshot: map[string]any{
			"mode":               "rules_only",
			"tldr_sentences":     opt.TLDRSentences,
			"bullet_max":         opt.BulletMax,
			"recent_tail_msgs":   opt.RecentTailMessages,
			"system_card_prefix": opt.SystemCardPrefix,
		},
		ContextRecipe: ContextRecipe{
			SystemCard:         true,
			RecentTailMessages: opt.RecentTailMessages,
		},
		Outputs: Outputs{
			TLDR:    tldr,
			Bullets: bullets,
			Card:    card,
		},
		SelectedSegments: segs,
	}
}

func normalizeOptions(opt Options) Options {
	if opt.RecentTailMessages <= 0 {
		opt.RecentTailMessages = 8
	}
	if strings.TrimSpace(opt.SystemCardPrefix) == "" {
		opt.SystemCardPrefix = "【压缩记忆】\n"
	}
	if opt.TLDRSentences <= 0 {
		opt.TLDRSentences = 3
	}
	if opt.BulletMax <= 0 {
		opt.BulletMax = 10
	}
	return opt
}

func filterMessages(msgs []*memory.Message) []*memory.Message {
	out := make([]*memory.Message, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		// keep system out: UI/raw memory still keeps it, but compression card shouldn't echo system prompts
		if m.Role == memory.MessageRoleSystem {
			continue
		}
		// drop empty slots
		if strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) == 0 && strings.TrimSpace(m.ToolCallID) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func selectSegments(msgs []*memory.Message) (string, []SelectedSegment) {
	// Rule-based selection:
	// - headings: markdown headings
	// - conclusion: lines starting with "结论"/"TL;DR"/"总结"
	// - error_stack: common error keywords and stack traces
	// - tail_lines: last ~80 lines worth of text from tail messages
	var segs []SelectedSegment

	var b strings.Builder

	add := func(kind, s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		segs = append(segs, SelectedSegment{Kind: kind, Snippet: s})
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s)
	}

	headingRe := regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+.+$`)
	conclusionRe := regexp.MustCompile(`(?mi)^\s*(结论|总结|TL;DR|tldr)\s*[:：].*$`)
	errorRe := regexp.MustCompile(`(?mi)(error|exception|panic|失败|报错|堆栈|stack trace|traceback|fatal)`)

	// Scan all messages for headings/conclusions/errors.
	for _, m := range msgs {
		c := m.Content
		if strings.TrimSpace(c) == "" {
			continue
		}
		if ms := headingRe.FindAllString(c, -1); len(ms) > 0 {
			add("headings", strings.Join(uniqueLines(normalizeLines(ms)), "\n"))
		}
		if ms := conclusionRe.FindAllString(c, -1); len(ms) > 0 {
			add("conclusion", strings.Join(uniqueLines(normalizeLines(ms)), "\n"))
		}
		if errorRe.MatchString(c) {
			add("error_stack", pickErrorContext(c))
		}
	}

	// Tail lines from last messages.
	tail := lastNonEmptyContents(msgs, 6)
	if tail != "" {
		add("tail_lines", tail)
	}

	// Dedupe segments by snippet text, keep stable order.
	segs = dedupeSegments(segs)
	return b.String(), segs
}

func lastNonEmptyContents(msgs []*memory.Message, maxMsgs int) string {
	lines := make([]string, 0, 120)
	for i := len(msgs) - 1; i >= 0 && maxMsgs > 0; i-- {
		m := msgs[i]
		if m == nil {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		maxMsgs--
		parts := strings.Split(c, "\n")
		// take last up to 20 lines per message
		start := 0
		if len(parts) > 20 {
			start = len(parts) - 20
		}
		for _, ln := range parts[start:] {
			ln = strings.TrimRight(ln, " \t")
			ln = normalizeLine(ln)
			if ln == "" {
				continue
			}
			if isNoiseLine(ln) {
				continue
			}
			lines = append(lines, ln)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	// reverse to chronological
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	// limit to 80 lines
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	return strings.Join(mergeRepeatedLines(lines, 80), "\n")
}

func pickErrorContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	type hit struct {
		i int
		l string
	}
	var hits []hit
	errorRe := regexp.MustCompile(`(?mi)(error|exception|panic|失败|报错|堆栈|stack trace|traceback|fatal)`)
	for i, ln := range lines {
		if errorRe.MatchString(ln) {
			hits = append(hits, hit{i: i, l: ln})
		}
	}
	if len(hits) == 0 {
		if len(lines) > 12 {
			return strings.Join(lines[:12], "\n")
		}
		return s
	}
	// take first hit window.
	i := hits[0].i
	start := i - 4
	if start < 0 {
		start = 0
	}
	end := i + 8
	if end > len(lines) {
		end = len(lines)
	}
	window := lines[start:end]
	window = normalizeLines(window)
	window = filterNoiseLines(window)
	return strings.Join(mergeRepeatedLines(window, 50), "\n")
}

func uniqueLines(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func dedupeSegments(segs []SelectedSegment) []SelectedSegment {
	seen := map[string]struct{}{}
	out := make([]SelectedSegment, 0, len(segs))
	for _, s := range segs {
		key := strings.TrimSpace(s.Kind) + "\n" + strings.TrimSpace(s.Snippet)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func buildTLDR(selected string, n int) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return ""
	}
	// Heuristic: pick first N non-empty sentences/lines.
	lines := splitToMeaningfulLines(selected)
	if len(lines) == 0 {
		return ""
	}
	if n <= 0 {
		n = 3
	}
	lines = mergeRepeatedLines(lines, n)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "；")
}

func buildBullets(selected string, max int) []string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return nil
	}
	lines := splitToMeaningfulLines(selected)
	if len(lines) == 0 {
		return nil
	}
	if max <= 0 {
		max = 10
	}
	lines = mergeRepeatedLines(lines, max)
	if len(lines) > max {
		lines = lines[:max]
	}
	// normalize bullet-like lines
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(strings.TrimLeft(ln, "-*•\t "))
		ln = normalizeLine(ln)
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if isNoiseLine(ln) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func buildCard(selected string) Card {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return Card{}
	}
	// Very light structure extraction.
	lines := splitToMeaningfulLines(selected)
	if len(lines) == 0 {
		return Card{}
	}
	lines = mergeRepeatedLines(lines, 30)
	card := Card{
		Problem: lines[0],
	}
	// Evidence: top 3 distinct lines (excluding first).
	ev := make([]string, 0, 3)
	seen := map[string]struct{}{card.Problem: {}}
	for _, ln := range lines[1:] {
		if len(ev) >= 3 {
			break
		}
		if _, ok := seen[ln]; ok {
			continue
		}
		seen[ln] = struct{}{}
		ev = append(ev, ln)
	}
	card.Evidence = ev
	// Actions: lines that look like imperatives / TODO.
	actions := pickActions(lines, 5)
	card.Actions = actions
	return card
}

func splitToMeaningfulLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = normalizeLine(ln)
		if ln == "" || isNoiseLine(ln) {
			continue
		}
		// skip pure separators
		if strings.HasPrefix(ln, "──") {
			continue
		}
		out = append(out, ln)
	}
	// stable sort by length ascending for some diversity? keep original order, but dedupe.
	seen := map[string]struct{}{}
	deduped := make([]string, 0, len(out))
	for _, ln := range out {
		if _, ok := seen[ln]; ok {
			continue
		}
		seen[ln] = struct{}{}
		deduped = append(deduped, ln)
	}
	return deduped
}

func pickActions(lines []string, max int) []string {
	if max <= 0 {
		max = 5
	}
	actionHints := []string{"TODO", "行动", "修复", "解决", "请", "建议", "下一步", "改", "更新", "检查", "确认"}
	type cand struct {
		line  string
		score int
	}
	var cands []cand
	for _, ln := range lines {
		score := 0
		up := strings.ToUpper(ln)
		for _, h := range actionHints {
			if strings.Contains(up, strings.ToUpper(h)) {
				score++
			}
		}
		if score > 0 {
			cands = append(cands, cand{line: ln, score: score})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	out := make([]string, 0, max)
	seen := map[string]struct{}{}
	for _, c := range cands {
		if len(out) >= max {
			break
		}
		if _, ok := seen[c.line]; ok {
			continue
		}
		seen[c.line] = struct{}{}
		out = append(out, c.line)
	}
	return out
}

var fromPrefixRe = regexp.MustCompile(`(?i)^\s*from\s+[^:]{1,60}:\s*`)

func normalizeLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Collapse nested "From A: From B: ..." chains to reduce noise.
	for i := 0; i < 3; i++ {
		next := fromPrefixRe.ReplaceAllString(s, "")
		next = strings.TrimSpace(next)
		if next == "" || next == s {
			break
		}
		s = next
	}
	// Normalize some common duplicated punctuation/spaces.
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func normalizeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = normalizeLine(ln)
		if ln == "" {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func filterNoiseLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = normalizeLine(ln)
		if ln == "" || isNoiseLine(ln) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func isNoiseLine(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return true
	}
	noiseContains := []string{
		"输入为空", "空消息", "无效输入", "忽略", "闭麦", "占坑", "测延迟",
		"请明确任务", "无指令", "没下文", "有事说事", "别浪费带宽",
		"工具已经执行完成", "已按你的要求加载",
	}
	for _, h := range noiseContains {
		if strings.Contains(low, strings.ToLower(h)) {
			return true
		}
	}
	// Pure numbering templates without content.
	if low == "1." || low == "2." || low == "3." {
		return true
	}
	return false
}

// mergeRepeatedLines keeps order but merges exact repeated lines as "line (xN)".
func mergeRepeatedLines(lines []string, limit int) []string {
	if len(lines) == 0 {
		return nil
	}
	type item struct {
		line  string
		count int
	}
	var items []item
	index := map[string]int{}
	for _, ln := range lines {
		ln = normalizeLine(ln)
		if ln == "" || isNoiseLine(ln) {
			continue
		}
		if pos, ok := index[ln]; ok {
			items[pos].count++
			continue
		}
		index[ln] = len(items)
		items = append(items, item{line: ln, count: 1})
		if limit > 0 && len(items) >= limit {
			// stop adding new unique items; still allow counts for existing
			continue
		}
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.count > 1 {
			out = append(out, it.line+" (x"+itoa(it.count)+")")
		} else {
			out = append(out, it.line)
		}
	}
	return out
}

func itoa(n int) string {
	// small, allocation-free enough for our counts
	if n == 0 {
		return "0"
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
