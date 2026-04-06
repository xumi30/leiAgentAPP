package memo

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const fileName = "memo.md"

// Path returns the legacy markdown file path (used for one-time migration into DB).
func Path() string {
	abs, err := filepath.Abs(filepath.Join(".", "data", fileName))
	if err != nil {
		return filepath.Join(".", "data", fileName)
	}
	return abs
}

// Read returns the full memo text from SQLite (or memo.md if DB cannot open).
func Read() (string, error) {
	if _, err := openNoteDB(); err != nil {
		return readMarkdownFileOnly()
	}
	return ReadFromStore()
}

func writeMarkdownFile(content string) error {
	p := Path()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0644)
}

// WriteAll replaces all memo content.
func WriteAll(content string) error {
	if _, err := openNoteDB(); err != nil {
		return writeMarkdownFile(content)
	}
	return WriteAllToStore(content)
}

// AppendBlock appends a section (tool memo_write). Title becomes top-level # heading.
func AppendBlock(heading, body string) error {
	h := strings.TrimSpace(heading)
	if h == "" {
		h = time.Now().Format("2006-01-02 15:04")
	}
	if _, err := openNoteDB(); err != nil {
		cur, err := readMarkdownFileOnly()
		if err != nil {
			return err
		}
		var block string
		if cur == "" {
			block = "# " + h + "\n\n" + body
		} else {
			if !strings.HasSuffix(cur, "\n") {
				cur += "\n"
			}
			block = cur + "\n# " + h + "\n\n" + body
		}
		return writeMarkdownFile(block)
	}
	return AppendNoteToStore(h, body, "")
}

// AppendMarkdownRaw appends one or more # sections from markdown (e.g. LLM output).
func AppendMarkdownRaw(block string) error {
	block = strings.TrimSpace(block)
	if block == "" {
		return nil
	}
	if _, err := openNoteDB(); err != nil {
		cur, err := readMarkdownFileOnly()
		if err != nil {
			return err
		}
		if cur == "" {
			return writeMarkdownFile(block)
		}
		if !strings.HasSuffix(strings.TrimRight(cur, "\n"), "\n") {
			cur = strings.TrimRight(cur, "\n") + "\n"
		}
		return writeMarkdownFile(strings.TrimRight(cur, "\n") + "\n\n" + block)
	}
	segs := splitTopLevelH1Sections(block)
	if len(segs) == 0 {
		return AppendNoteToStore("摘录", block, "")
	}
	for _, s := range segs {
		if err := AppendNoteToStore(s.title, s.body, s.sourceIDs); err != nil {
			return err
		}
	}
	return nil
}

var isoDateRE = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)

// CalendarDates returns unique YYYY-MM-DD strings referenced in the memo, for UI calendar dots.
func CalendarDates(md string) []string {
	md = strings.TrimSpace(md)
	if md == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(d string) {
		if _, ok := seen[d]; ok {
			return
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	addAll := func(block string, maxLines int) {
		s := block
		if maxLines > 0 {
			lines := strings.Split(block, "\n")
			if len(lines) > maxLines {
				s = strings.Join(lines[:maxLines], "\n")
			}
		}
		for _, m := range isoDateRE.FindAllStringSubmatch(s, -1) {
			add(m[1])
		}
	}

	secHead := regexp.MustCompile(`(?m)^# ([^\n]+)(?:\n|$)`)
	idxs := secHead.FindAllStringSubmatchIndex(md, -1)
	for i, loc := range idxs {
		title := md[loc[2]:loc[3]]
		addAll(title, 0)
		start := loc[1]
		var end int
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		} else {
			end = len(md)
		}
		body := md[start:end]
		addAll(body, 14)
	}
	if len(idxs) == 0 {
		addAll(md, 0)
	} else if idxs[0][0] > 0 {
		addAll(md[:idxs[0][0]], 0)
	}

	sort.Strings(out)
	return out
}
