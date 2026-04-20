package searchFunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/doclib"
	"leiAgent/utils"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	downloadBooksScriptMode = "engine"

	downloadBooksUserAgent   = "leiAgentAPP/1.0 (downloadbooks)"
	downloadBooksMaxFileSize = 50 * 1024 * 1024
	downloadBooksSniffBytes  = 8192
	downloadBooksHTTPTimeout = 120 * time.Second
	downloadBooksTopK        = 10
	downloadBooksMaxQueries  = 8
	downloadBooksWebPerPage  = 10
	downloadBooksWebMaxPages = 3
)

var (
	downloadBooksUnsafeNameRe         = regexp.MustCompile(`[<>:"/\\|?*\x00]+`)
	downloadBooksSpaceRe              = regexp.MustCompile(`\s+`)
	downloadBooksNonWordRe            = regexp.MustCompile(`[\W_]+`)
	downloadBooksUnderscoreCollapseRe = regexp.MustCompile(`_+`)
	downloadBooksRawURLPatterns       = []*regexp.Regexp{
		regexp.MustCompile(`"rawUrl":"([^"]+)"`),
		regexp.MustCompile(`href="(https://raw\.usercontent\.com/[^"]+)"`),
		regexp.MustCompile(`href="(/[^"]+/raw/[^"]+)"`),
	}
)

var downloadBooksDocumentExtensions = map[string]struct{}{
	".pdf":  {},
	".epub": {},
	".mobi": {},
	".azw":  {},
	".azw3": {},
	".djvu": {},
	".txt":  {},
	".md":   {},
	".rtf":  {},
	".doc":  {},
	".docx": {},
	".html": {},
	".htm":  {},
}

var downloadBooksBinarySafeExtensions = map[string]struct{}{
	".pdf":  {},
	".epub": {},
	".mobi": {},
	".azw":  {},
	".azw3": {},
	".djvu": {},
	".doc":  {},
	".docx": {},
	".ppt":  {},
	".pptx": {},
	".xls":  {},
	".xlsx": {},
	".zip":  {},
	".rar":  {},
	".7z":   {},
	".tar":  {},
	".gz":   {},
	".bz2":  {},
}

type DownloadBooksTool struct{}

type downloadBooksInput struct {
	Books       []string `json:"books"`
	OutputDir   string   `json:"output_dir,omitempty"`
	DownloadDir string   `json:"download_dir,omitempty"`
	// FileTypes optional hints (e.g. "pdf", "epub"); appended with spaces after the book title for the script keyword.
	FileTypes  []string `json:"file_types,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
}

type downloadBooksSearchItem struct {
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Path        string  `json:"path"`
	HTMLURL     string  `json:"html_url"`
	Score       float64 `json:"score"`
	LocalScore  float64 `json:"-"`
	SourceMode  string  `json:"-"`
	SourceQuery string  `json:"-"`
}

type downloadBooksBookResult struct {
	Mode           string                   `json:"mode"`
	AttemptedModes []string                 `json:"attempted_modes,omitempty"`
	Keyword        string                   `json:"keyword"`
	SearchQueries  []string                 `json:"_search_queries,omitempty"`
	OutputDir      string                   `json:"output_dir"`
	SearchProvider string                   `json:"search_provider,omitempty"`
	SearchQuery    string                   `json:"search_query,omitempty"`
	QueryTotals    []map[string]interface{} `json:"query_totals,omitempty"`
	TotalCount     int                      `json:"total_count"`
	CandidateCount int                      `json:"candidate_count"`
	Downloaded     int                      `json:"downloaded"`
	Downloads      []map[string]interface{} `json:"downloads"`
	Skipped        []map[string]interface{} `json:"skipped"`
	ResultPath     string                   `json:"result_path"`
	Error          string                   `json:"error,omitempty"`
}

type downloadBooksSearchResponse struct {
	OrganicResults []struct {
		Link     string  `json:"link"`
		Position float64 `json:"position"`
	} `json:"organic_results"`
}

type downloadBooksBraveResponse struct {
	Web struct {
		Results []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"web"`
}

func NewDownloadBooksTool() *DownloadBooksTool {
	return &DownloadBooksTool{}
}

func (t *DownloadBooksTool) Name() string {
	return "downloadbooks"
}

func (t *DownloadBooksTool) Description() string {
	return "Batch search  book files and download.Use this when the user provides one or more book names. simply Search and download."
}

func (t *DownloadBooksTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"books": map[string]interface{}{
				"type":        "array",
				"description": "List of book names to download in batch.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional output directory. Absolute paths are used directly; relative paths are created under the app workspace root.",
			},
			"download_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional directory to copy downloaded files into. Absolute paths are used directly; relative paths are created under the app workspace root. Files are copied into a per-book subdirectory.",
			},
			"file_types": map[string]interface{}{
				"type":        "array",
				"description": "Optional file type hints (e.g. pdf, epub). When set, each is appended after the book title with spaces to form the search keyword passed to getgitfile.py.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum candidates to attempt per book, default 10, max 20.",
			},
		},
		"required": []string{"books"},
	}
}

func (t *DownloadBooksTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *DownloadBooksTool) Execute(ctx context.Context, args string) (string, error) {
	var in downloadBooksInput
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("parse tool args: %w", err)
	}
	if len(in.Books) == 0 {
		return "", fmt.Errorf("books must be a non-empty list")
	}

	scriptAbs, err := downloadBooksScriptPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(scriptAbs); err != nil {
		return "", fmt.Errorf("getgitfile.py not found at %s: %w", scriptAbs, err)
	}

	workspaceRoot, err := doclib.LibraryRootAbs()
	if err != nil {
		return "", err
	}

	outRoot := workspaceRoot
	if rel := strings.TrimSpace(in.OutputDir); rel != "" {
		if filepath.IsAbs(rel) {
			outRoot = filepath.Clean(rel)
		} else {
			outRoot = filepath.Join(workspaceRoot, filepath.Clean(rel))
		}
	}
	if err := os.MkdirAll(outRoot, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	downloadRoot := ""
	if rel := strings.TrimSpace(in.DownloadDir); rel != "" {
		if filepath.IsAbs(rel) {
			downloadRoot = filepath.Clean(rel)
		} else {
			downloadRoot = filepath.Join(workspaceRoot, filepath.Clean(rel))
		}
		if err := os.MkdirAll(downloadRoot, 0755); err != nil {
			return "", fmt.Errorf("create download dir: %w", err)
		}
	}

	py, err := downloadBooksPython()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Dir(scriptAbs)

	fileTypeHints := make([]string, 0, len(in.FileTypes))
	for _, ft := range in.FileTypes {
		ft = strings.TrimSpace(ft)
		if ft != "" {
			fileTypeHints = append(fileTypeHints, ft)
		}
	}

	type bookRun struct {
		Book         string `json:"book"`
		SearchQuery  string `json:"search_query,omitempty"`
		ExitCode     int    `json:"exit_code"`
		Error        string `json:"error,omitempty"`
		ScriptOutput string `json:"script_output,omitempty"`
		ResultPath   string `json:"result_path,omitempty"`
		Downloaded   int    `json:"downloaded,omitempty"`
		Copied       int    `json:"copied,omitempty"`
		DownloadDir  string `json:"download_dir,omitempty"`
	}

	runs := make([]bookRun, 0, len(in.Books))
	totalDownloaded := 0
	totalCopied := 0

	for _, book := range in.Books {
		book = strings.TrimSpace(book)
		if book == "" {
			runs = append(runs, bookRun{Book: book, ExitCode: -1, Error: "empty book name"})
			continue
		}

		query := book
		if len(fileTypeHints) > 0 {
			query = strings.Join(append([]string{book}, fileTypeHints...), " ")
		}

		cmd := exec.CommandContext(ctx, py, scriptAbs, "--mode", downloadBooksScriptMode, query)
		cmd.Dir = outRoot

		out, runErr := cmd.CombinedOutput()
		br := bookRun{Book: book, ScriptOutput: strings.TrimSpace(string(out))}
		if query != book {
			br.SearchQuery = query
		}

		if runErr != nil {
			if ee, ok := runErr.(*exec.ExitError); ok {
				br.ExitCode = ee.ExitCode()
			} else {
				br.ExitCode = -1
			}
			br.Error = runErr.Error()
		}

		outDirName := downloadBooksSafeDirName(query)
		manifestPath := filepath.Join(scriptDir, outDirName, "result.json")
		var downloadedFiles []string
		if st, statErr := os.Stat(manifestPath); statErr == nil && !st.IsDir() {
			br.ResultPath = manifestPath
			if b, readErr := os.ReadFile(manifestPath); readErr == nil {
				var manifest map[string]interface{}
				if json.Unmarshal(b, &manifest) == nil {
					if v, ok := manifest["downloaded"].(float64); ok {
						br.Downloaded = int(v)
					}
					if arr, ok := manifest["downloads"].([]interface{}); ok {
						for _, it := range arr {
							m, _ := it.(map[string]interface{})
							if m == nil {
								continue
							}
							if lf, ok := m["local_filename"].(string); ok {
								lf = strings.TrimSpace(lf)
								if lf != "" {
									downloadedFiles = append(downloadedFiles, lf)
								}
							}
						}
					}
				}
			}
		}

		if runErr == nil {
			br.ExitCode = 0
			totalDownloaded += br.Downloaded

			if downloadRoot != "" && len(downloadedFiles) > 0 {
				destDir := filepath.Join(downloadRoot, downloadBooksSafeDirName(book))
				if err := os.MkdirAll(destDir, 0755); err == nil {
					br.DownloadDir = destDir
					for _, name := range downloadedFiles {
						src := filepath.Join(scriptDir, outDirName, name)
						if _, err := os.Stat(src); err != nil {
							continue
						}
						dstName := downloadBooksAllocateFilename(destDir, name)
						dst := filepath.Join(destDir, dstName)
						if err := downloadBooksCopyFile(src, dst); err == nil {
							br.Copied++
							totalCopied++
						}
					}
				}
			}
		}

		runs = append(runs, br)
	}

	batchPath := filepath.Join(outRoot, "downloadbooks_batch_result.json")
	batchPayload := map[string]interface{}{
		"message":          fmt.Sprintf("Ran getgitfile.py for %d book(s) (engine search).", len(in.Books)),
		"output_dir":       outRoot,
		"download_dir":     downloadRoot,
		"batch_result":     batchPath,
		"downloaded_files": totalDownloaded,
		"copied_files":     totalCopied,
		"books":            runs,
	}
	batchBytes, err := json.MarshalIndent(batchPayload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal batch result: %w", err)
	}
	if err := os.WriteFile(batchPath, batchBytes, 0644); err != nil {
		return "", fmt.Errorf("write batch manifest: %w", err)
	}

	return string(batchBytes), nil
}

func downloadBooksScriptPath() (string, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		candidate := filepath.Join(filepath.Dir(file), "scripts", "getgitfile.py")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Abs(candidate)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve script path: %w", err)
	}
	for _, rel := range []string{
		filepath.Join("internal", "tools", "searchFuctions", "scripts", "getgitfile.py"),
		filepath.Join("scripts", "getgitfile.py"),
	} {
		candidate := filepath.Join(wd, rel)
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("getgitfile.py not found next to tool source or under cwd (%s)", wd)
}

func downloadBooksPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("python not found in PATH (tried python3, python)")
}

func downloadBooksSafeDirName(keyword string) string {
	s := strings.TrimSpace(strings.ReplaceAll(keyword, " ", "_"))
	for _, c := range `<>:\"/|?*` + "\x00" {
		s = strings.ReplaceAll(s, string(c), "_")
	}
	s = downloadBooksUnderscoreCollapseRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "output"
	}
	return s
}

func downloadBooksAllocateFilename(dir, desired string) string {
	desired = strings.TrimSpace(desired)
	if desired == "" {
		desired = "file"
	}
	candidate := desired
	if _, err := os.Stat(filepath.Join(dir, candidate)); err != nil {
		return candidate
	}
	stem := strings.TrimSuffix(desired, filepath.Ext(desired))
	ext := filepath.Ext(desired)
	if stem == "" {
		stem = "file"
	}
	for i := 2; i < 10000; i++ {
		candidate = fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(dir, candidate)); err != nil {
			return candidate
		}
	}
	return fmt.Sprintf("%s_%d%s", stem, time.Now().UnixNano(), ext)
}

func downloadBooksCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func (t *DownloadBooksTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Batch download result for one or more requested books.",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Summary of the batch download task.",
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Resolved batch output directory.",
			},
			"download_dir": map[string]interface{}{
				"type":        "string",
				"description": "Resolved directory where downloaded files were copied (empty when not requested).",
			},
			"batch_result": map[string]interface{}{
				"type":        "string",
				"description": "Path to the batch manifest JSON file.",
			},
			"downloaded_files": map[string]interface{}{
				"type":        "integer",
				"description": "Total downloaded files across all books.",
			},
			"copied_files": map[string]interface{}{
				"type":        "integer",
				"description": "Total number of files copied into download_dir across all books.",
			},
			"books": map[string]interface{}{
				"type":        "array",
				"description": "Per-book detailed results including downloads and skips.",
				"items": map[string]interface{}{
					"type": "object",
				},
			},
		},
	}
}

func (t *DownloadBooksTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicSearch, "按书名列表批量检索 电子书文件候选，并下载原始文档到应用 workspace。")
}
