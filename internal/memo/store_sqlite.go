package memo

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const (
	sourceMarkerOpen  = "<!--leiAgent-memo-src:"
	sourceMarkerClose = "-->"
)

var (
	noteDB     *sql.DB
	noteDBOnce sync.Once
	noteDBErr  error
	storeMu    sync.Mutex
)

// StoreDBPath returns the SQLite path for memo notes.
func StoreDBPath() string {
	abs, err := filepath.Abs(filepath.Join(".", "data", "memo_notes.db"))
	if err != nil {
		return filepath.Join(".", "data", "memo_notes.db")
	}
	return abs
}

func openNoteDB() (*sql.DB, error) {
	noteDBOnce.Do(func() {
		p := StoreDBPath()
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			noteDBErr = err
			return
		}
		dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", filepath.ToSlash(p))
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			noteDBErr = err
			return
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS memo_note (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sort_order INTEGER NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			source_ids TEXT
		);`); err != nil {
			_ = db.Close()
			noteDBErr = err
			return
		}
		noteDB = db
	})
	if noteDBErr != nil {
		return nil, noteDBErr
	}
	return noteDB, nil
}

type noteSection struct {
	title     string
	body      string
	sourceIDs string
}

var sourceLineRE = regexp.MustCompile(`(?m)^<!--leiAgent-memo-src:([^>]+)-->\s*$`)

func extractSourceFromBody(block string) (body string, sourceIDs string) {
	block = strings.TrimSpace(block)
	m := sourceLineRE.FindStringSubmatch(block)
	if m == nil {
		return block, ""
	}
	body = strings.TrimSpace(sourceLineRE.ReplaceAllString(block, ""))
	return body, strings.TrimSpace(m[1])
}

var h1TitleRE = regexp.MustCompile(`(?m)^# ([^\n]+)\s*$`)

// splitTopLevelH1Sections splits markdown by top-level # headings (same rule as UI).
func splitTopLevelH1Sections(md string) []noteSection {
	md = strings.TrimPrefix(md, "\ufeff")
	md = strings.TrimSpace(md)
	if md == "" {
		return nil
	}
	idx := h1TitleRE.FindAllStringSubmatchIndex(md, -1)
	if len(idx) == 0 {
		return []noteSection{{title: "备忘录", body: md, sourceIDs: ""}}
	}
	var out []noteSection
	if idx[0][0] > 0 {
		pre := strings.TrimSpace(md[:idx[0][0]])
		if pre != "" {
			out = append(out, noteSection{title: "摘录", body: pre, sourceIDs: ""})
		}
	}
	for i, loc := range idx {
		title := strings.TrimSpace(md[loc[2]:loc[3]])
		start := loc[1]
		var end int
		if i+1 < len(idx) {
			end = idx[i+1][0]
		} else {
			end = len(md)
		}
		block := strings.TrimSpace(md[start:end])
		body, src := extractSourceFromBody(block)
		out = append(out, noteSection{title: title, body: body, sourceIDs: src})
	}
	return out
}

func joinSectionsMarkdown(sections []noteSection) string {
	if len(sections) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("# ")
		b.WriteString(s.title)
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(s.body))
		if s.sourceIDs != "" {
			b.WriteString("\n\n")
			b.WriteString(sourceMarkerOpen)
			b.WriteString(s.sourceIDs)
			b.WriteString(sourceMarkerClose)
		}
	}
	return b.String()
}

func readMarkdownFileOnly() (string, error) {
	p := Path()
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimPrefix(string(raw), "\ufeff"), nil
}

func migrateFromMarkdownFile(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memo_note`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	md, err := readMarkdownFileOnly()
	if err != nil || strings.TrimSpace(md) == "" {
		return nil
	}
	secs := splitTopLevelH1Sections(md)
	return replaceAllSectionsDB(db, secs)
}

func replaceAllSectionsDB(db *sql.DB, sections []noteSection) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memo_note`); err != nil {
		_ = tx.Rollback()
		return err
	}
	for i, s := range sections {
		_, err := tx.Exec(
			`INSERT INTO memo_note (sort_order, title, body, source_ids) VALUES (?,?,?,?)`,
			i, s.title, s.body, nullIfEmpty(s.sourceIDs),
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func readAllSectionsDB(db *sql.DB) ([]noteSection, error) {
	rows, err := db.Query(`SELECT title, body, source_ids FROM memo_note ORDER BY sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []noteSection
	for rows.Next() {
		var t, b string
		var src sql.NullString
		if err := rows.Scan(&t, &b, &src); err != nil {
			return nil, err
		}
		sid := ""
		if src.Valid {
			sid = src.String
		}
		list = append(list, noteSection{title: t, body: b, sourceIDs: sid})
	}
	return list, rows.Err()
}

// ReadFromStore returns composed markdown from SQLite (after migrate).
func ReadFromStore() (string, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	db, err := openNoteDB()
	if err != nil {
		return "", err
	}
	if err := migrateFromMarkdownFile(db); err != nil {
		return "", err
	}
	list, err := readAllSectionsDB(db)
	if err != nil {
		return "", err
	}
	return joinSectionsMarkdown(list), nil
}

// WriteAllToStore replaces all notes from full markdown document.
func WriteAllToStore(md string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	db, err := openNoteDB()
	if err != nil {
		return err
	}
	if err := migrateFromMarkdownFile(db); err != nil {
		return err
	}
	secs := splitTopLevelH1Sections(md)
	return replaceAllSectionsDB(db, secs)
}

// AppendNoteToStore appends one top-level note (title + body + optional source ids).
func AppendNoteToStore(title, body, sourceIDs string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	db, err := openNoteDB()
	if err != nil {
		return err
	}
	if err := migrateFromMarkdownFile(db); err != nil {
		return err
	}
	var maxOrder int
	_ = db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) FROM memo_note`).Scan(&maxOrder)
	_, err = db.Exec(
		`INSERT INTO memo_note (sort_order, title, body, source_ids) VALUES (?,?,?,?)`,
		maxOrder+1, strings.TrimSpace(title), strings.TrimSpace(body), nullIfEmpty(sourceIDs),
	)
	return err
}

var globalSourceRE = regexp.MustCompile(`<!--leiAgent-memo-src:([^>]+)-->`)

// ReferencedMessageIDs scans stored content for source markers and returns unique message IDs.
func ReferencedMessageIDs() ([]string, error) {
	md, err := ReadFromStore()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []string
	for _, m := range globalSourceRE.FindAllStringSubmatch(md, -1) {
		for _, id := range strings.Split(m[1], ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out, nil
}
