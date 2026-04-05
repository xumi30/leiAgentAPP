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

// Path returns the absolute path to the memo file under ./data/memo.md.
func Path() string {
	abs, err := filepath.Abs(filepath.Join(".", "data", fileName))
	if err != nil {
		return filepath.Join(".", "data", fileName)
	}
	return abs
}

// Read returns the full memo text. Missing file yields empty string.
func Read() (string, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	s := string(b)
	// Strip UTF-8 BOM so the first line parses as Markdown and Chinese displays correctly.
	s = strings.TrimPrefix(s, "\ufeff")
	return s, nil
}

// WriteAll replaces the entire memo file.
func WriteAll(content string) error {
	p := Path()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0644)
}

// AppendBlock appends a dated section. If heading is empty, uses current local time.
func AppendBlock(heading, body string) error {
	cur, err := Read()
	if err != nil {
		return err
	}
	h := strings.TrimSpace(heading)
	if h == "" {
		h = time.Now().Format("2006-01-02 15:04")
	}
	var block string
	if cur == "" {
		block = "## " + h + "\n\n" + body
	} else {
		if !strings.HasSuffix(cur, "\n") {
			cur += "\n"
		}
		block = "\n## " + h + "\n\n" + body
	}
	return WriteAll(cur + block)
}

var isoDateRE = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)

// CalendarDates returns unique YYYY-MM-DD strings referenced in the memo, for UI calendar dots.
// It scans each ## section title first, then the first lines of the section body, plus any preamble.
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

	secHead := regexp.MustCompile(`(?m)^## ([^\n]+)(?:\n|$)`)
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
