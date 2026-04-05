package doclib

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxViewerBytes = 5 << 20

// 与 isLikelyTextFile 常见扩展一致；路径须在扩展名处结束，避免把「xxx.md 后面的中文」吃进路径。
const winPathDocExt = `(?i)\.(?:markdown|md|txt|json|csv|html?|log|yml|yaml|xml|css|scss|less|tsx?|jsx?|ts|js|go|py|rs|sql|sh|bat|ps1|env|toml|ini|cfg)\b`

var (
	storeMu sync.Mutex

	winBackslashPath = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]+` + winPathDocExt)
	winSlashPath     = regexp.MustCompile(`[A-Za-z]:/(?:[^/<>:"|?*\r\n]+/)*[^/<>:"|?*\r\n]+` + winPathDocExt)
	unixAbsPath      = regexp.MustCompile(`/(?:[\w.-]+/)+[\w.-]+\.[A-Za-z0-9]{1,12}\b`)
)

type storeFile struct {
	Entries []storeEntry `json:"entries"`
}

type storeEntry struct {
	Path string `json:"path"`
	At   string `json:"at"`
}

func storePath() string {
	return filepath.Join("data", "doc_library.json")
}

// Register records an absolute file path created or updated by tools (deduped).
func Register(absPath string) {
	absPath = filepath.Clean(absPath)
	if absPath == "" || absPath == "." {
		return
	}
	st, err := os.Stat(absPath)
	if err != nil || st.IsDir() {
		return
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	sf, _ := readStoreUnlocked()
	found := false
	for i := range sf.Entries {
		if strings.EqualFold(filepath.Clean(sf.Entries[i].Path), absPath) {
			sf.Entries[i].Path = absPath
			sf.Entries[i].At = time.Now().UTC().Format(time.RFC3339)
			found = true
			break
		}
	}
	if !found {
		sf.Entries = append(sf.Entries, storeEntry{
			Path: absPath,
			At:   time.Now().UTC().Format(time.RFC3339),
		})
	}
	_ = writeStoreUnlocked(sf)
}

func readStoreUnlocked() (storeFile, error) {
	var sf storeFile
	b, err := os.ReadFile(storePath())
	if err != nil {
		if os.IsNotExist(err) {
			return sf, nil
		}
		return sf, err
	}
	if err := json.Unmarshal(b, &sf); err != nil {
		return sf, err
	}
	return sf, nil
}

func writeStoreUnlocked(sf storeFile) error {
	if err := os.MkdirAll(filepath.Dir(storePath()), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storePath(), b, 0644)
}

// HarvestPathsFromText extracts plausible filesystem paths from message text.
func HarvestPathsFromText(s string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if runtime.GOOS == "windows" {
			p = strings.ReplaceAll(p, "/", `\`)
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, m := range winBackslashPath.FindAllString(s, -1) {
		add(m)
	}
	for _, m := range winSlashPath.FindAllString(s, -1) {
		add(m)
	}
	for _, m := range unixAbsPath.FindAllString(s, -1) {
		add(m)
	}
	return out
}

func isLikelyTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown", ".txt", ".json", ".csv", ".log", ".html", ".htm",
		".yml", ".yaml", ".xml", ".css", ".scss", ".less", ".ts", ".tsx", ".js", ".jsx",
		".go", ".py", ".rs", ".sql", ".sh", ".bat", ".ps1", ".env", ".toml", ".ini", ".cfg":
		return true
	default:
		return ext == ""
	}
}

type docItem struct {
	Path    string
	Name    string
	ModTime time.Time
	Size    int64
	Source  string
}

// List merges registered paths, paths harvested from messages, and returns existing files sorted by mtime desc.
func List(workspace string, messageBodies []string) ([]map[string]interface{}, error) {
	workspace = filepath.Clean(workspace)
	pathSet := map[string]docItem{}

	storeMu.Lock()
	sf, _ := readStoreUnlocked()
	storeMu.Unlock()
	for _, e := range sf.Entries {
		p := filepath.Clean(e.Path)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && isLikelyTextFile(p) {
			pathSet[p] = docItem{
				Path:    p,
				Name:    filepath.Base(p),
				ModTime: fi.ModTime(),
				Size:    fi.Size(),
				Source:  "registered",
			}
		}
	}

	for _, body := range messageBodies {
		for _, p := range HarvestPathsFromText(body) {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && isLikelyTextFile(p) {
				if cur, ok := pathSet[p]; ok {
					if fi.ModTime().After(cur.ModTime) {
						cur.ModTime = fi.ModTime()
						cur.Size = fi.Size()
						pathSet[p] = cur
					}
					continue
				}
				pathSet[p] = docItem{
					Path:    p,
					Name:    filepath.Base(p),
					ModTime: fi.ModTime(),
					Size:    fi.Size(),
					Source:  "message",
				}
			}
		}
	}

	var items []docItem
	for _, it := range pathSet {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ModTime.After(items[j].ModTime)
	})

	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]interface{}{
			"path":     it.Path,
			"name":     it.Name,
			"modTime":  it.ModTime.Format(time.RFC3339),
			"size":     it.Size,
			"source":   it.Source,
			"relHint":  relHint(workspace, it.Path),
		})
	}
	return out, nil
}

func relHint(workspace, abs string) string {
	workspace = filepath.Clean(workspace)
	abs = filepath.Clean(abs)
	if rel, err := filepath.Rel(workspace, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return ""
}

// ReadText reads a file for in-app viewing (size-capped, UTF-8 safe).
func ReadText(abs string) (string, error) {
	abs = filepath.Clean(abs)
	fi, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if fi.Size() > maxViewerBytes {
		return "", fmt.Errorf("file too large (max %d bytes)", maxViewerBytes)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		s := strings.ToValidUTF8(string(b), "\uFFFD")
		return s, nil
	}
	return string(b), nil
}

// RevealInFileManager opens Explorer / Finder at the file.
func RevealInFileManager(abs string) error {
	abs = filepath.Clean(abs)
	switch runtime.GOOS {
	case "windows":
		// /select, must be concatenated without space after comma
		return exec.Command("explorer", "/select,", abs).Start()
	case "darwin":
		return exec.Command("open", "-R", abs).Start()
	default:
		dir := filepath.Dir(abs)
		return exec.Command("xdg-open", dir).Start()
	}
}
