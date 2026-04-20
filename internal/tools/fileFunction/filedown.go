package fileFunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/doclib"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	fileDownloadUserAgent = "Mozilla/5.0 (compatible; leiAgentAPP/1.0; +https://local.app)"
	fileDownloadTimeout   = 10 * time.Minute
)

var fileDownloadUnsafeNameRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

type FileDownloadTool struct{}

type fileDownloadInput struct {
	URL        string `json:"url"`
	OutputPath string `json:"output_path,omitempty"`
	OutputDir  string `json:"output_dir,omitempty"`
	Filename   string `json:"filename,omitempty"`
	Overwrite  bool   `json:"overwrite,omitempty"`
}

func NewFileDownloadTool() tools.Tool {
	return &FileDownloadTool{}
}

func (t *FileDownloadTool) Name() string {
	return "file_download"
}

func (t *FileDownloadTool) Description() string {
	return "Download a file from a direct http/https URL into the workspace. Prefer this when the user already gives a direct downloadable link (pdf, zip, image, raw file, document). The tool first tries native HTTP download, then falls back to curl or wget automatically for broader compatibility."
}

func (t *FileDownloadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Direct downloadable http/https URL.",
			},
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "Optional target file path. Relative paths are resolved under the workspace root. If set, it takes precedence over output_dir and filename.",
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional target directory under the workspace root when output_path is not set.",
			},
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "Optional target filename when output_path is not set.",
			},
			"overwrite": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to overwrite the target file if it already exists. Default false.",
			},
		},
		"required": []string{"url"},
	}
}

func (t *FileDownloadTool) Execute(ctx context.Context, args string) (string, error) {
	var in fileDownloadInput
	if err := json.Unmarshal([]byte(utils.PrepareLLMJSON(args)), &in); err != nil {
		return "", fmt.Errorf("parse tool args: %w", err)
	}

	downloadURL := strings.TrimSpace(in.URL)
	if downloadURL == "" {
		return "", fmt.Errorf("url is required")
	}
	parsedURL, err := url.Parse(downloadURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return "", fmt.Errorf("url must be a valid http/https URL")
	}

	root, err := doclib.LibraryRootAbs()
	if err != nil {
		return "", err
	}

	targetPath, targetDir, explicitPath, err := fileDownloadResolveTarget(root, in)
	if err != nil {
		return "", err
	}

	type downloadAttempt struct {
		Method string
		Error  string
	}
	attempts := make([]downloadAttempt, 0, 3)

	finalPath, method, statusCode, contentType, err := fileDownloadWithHTTP(ctx, downloadURL, parsedURL, targetPath, targetDir, explicitPath, in.Overwrite)
	if err == nil {
		return fileDownloadSuccess(downloadURL, finalPath, method, statusCode, contentType, attempts)
	}
	attempts = append(attempts, downloadAttempt{Method: "http", Error: err.Error()})

	finalPath, method, err = fileDownloadWithCommand(ctx, "curl", downloadURL, parsedURL, targetPath, targetDir, explicitPath, in.Overwrite)
	if err == nil {
		return fileDownloadSuccess(downloadURL, finalPath, method, 0, "", attempts)
	}
	attempts = append(attempts, downloadAttempt{Method: "curl", Error: err.Error()})

	finalPath, method, err = fileDownloadWithCommand(ctx, "wget", downloadURL, parsedURL, targetPath, targetDir, explicitPath, in.Overwrite)
	if err == nil {
		return fileDownloadSuccess(downloadURL, finalPath, method, 0, "", attempts)
	}
	attempts = append(attempts, downloadAttempt{Method: "wget", Error: err.Error()})

	out, _ := json.MarshalIndent(map[string]interface{}{
		"url":      downloadURL,
		"success":  false,
		"attempts": attempts,
	}, "", "  ")
	return string(out), fmt.Errorf("download failed after trying http, curl, and wget")
}

func (t *FileDownloadTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "File download result.",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type": "string",
			},
			"path": map[string]interface{}{
				"type": "string",
			},
			"filename": map[string]interface{}{
				"type": "string",
			},
			"size": map[string]interface{}{
				"type": "integer",
			},
			"method": map[string]interface{}{
				"type": "string",
			},
			"status_code": map[string]interface{}{
				"type": "integer",
			},
			"content_type": map[string]interface{}{
				"type": "string",
			},
		},
	}
}

func (t *FileDownloadTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicFiles, "当用户已经提供可直接下载的 http/https 文件链接时，将文件下载到 workspace。")
}

func fileDownloadResolveTarget(root string, in fileDownloadInput) (targetPath string, targetDir string, explicitPath bool, err error) {
	if raw := strings.TrimSpace(in.OutputPath); raw != "" {
		targetPath, err = fileDownloadResolveWorkspacePath(root, raw)
		if err != nil {
			return "", "", false, err
		}
		return targetPath, filepath.Dir(targetPath), true, nil
	}

	targetDir = root
	if raw := strings.TrimSpace(in.OutputDir); raw != "" {
		targetDir, err = fileDownloadResolveWorkspacePath(root, raw)
		if err != nil {
			return "", "", false, err
		}
	}
	return "", targetDir, false, nil
}

func fileDownloadResolveWorkspacePath(root, raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(raw) {
		abs := filepath.Clean(raw)
		if !doclib.IsPathUnderLibrary(abs) {
			return "", fmt.Errorf("path must stay inside workspace: %s", raw)
		}
		return abs, nil
	}
	return doclib.SafeLibraryAbs(root, raw)
}

func fileDownloadWithHTTP(ctx context.Context, downloadURL string, parsedURL *url.URL, targetPath string, targetDir string, explicitPath bool, overwrite bool) (string, string, int, string, error) {
	client := &http.Client{
		Timeout: fileDownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			req.Header.Set("User-Agent", fileDownloadUserAgent)
			req.Header.Set("Accept", "*/*")
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", "", 0, "", err
	}
	req.Header.Set("User-Agent", fileDownloadUserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	finalPath, err := fileDownloadBuildFinalPath(targetPath, targetDir, explicitPath, overwrite, parsedURL, resp.Header.Get("Content-Disposition"), resp.Header.Get("Content-Type"))
	if err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(finalPath), ".file_download-*")
	if err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}
	if err := tmpFile.Close(); err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", "", resp.StatusCode, resp.Header.Get("Content-Type"), err
	}

	return finalPath, "http", resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

func fileDownloadWithCommand(ctx context.Context, commandName string, downloadURL string, parsedURL *url.URL, targetPath string, targetDir string, explicitPath bool, overwrite bool) (string, string, error) {
	cmdPath, err := exec.LookPath(commandName)
	if err != nil {
		return "", "", fmt.Errorf("%s not found: %w", commandName, err)
	}

	finalPath, err := fileDownloadBuildFinalPath(targetPath, targetDir, explicitPath, overwrite, parsedURL, "", "")
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return "", "", err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(finalPath), ".file_download-*")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	var cmd *exec.Cmd
	switch commandName {
	case "curl":
		cmd = exec.CommandContext(ctx, cmdPath, "-fL", "--retry", "2", "--connect-timeout", "20", "--max-time", "600", "-A", fileDownloadUserAgent, "-o", tmpPath, downloadURL)
	case "wget":
		cmd = exec.CommandContext(ctx, cmdPath, "--tries=2", "--timeout=20", "--user-agent="+fileDownloadUserAgent, "-O", tmpPath, downloadURL)
	default:
		return "", "", fmt.Errorf("unsupported command fallback: %s", commandName)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("%s failed: %v, output=%s", commandName, err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", "", err
	}

	return finalPath, commandName, nil
}

func fileDownloadBuildFinalPath(targetPath string, targetDir string, explicitPath bool, overwrite bool, parsedURL *url.URL, contentDisposition string, contentType string) (string, error) {
	if explicitPath {
		if err := fileDownloadPrepareExistingPath(targetPath, overwrite, true); err != nil {
			return "", err
		}
		return targetPath, nil
	}

	name := fileDownloadFilenameFromDisposition(contentDisposition)
	if name == "" {
		name = fileDownloadFilenameFromURL(parsedURL)
	}
	if ext := filepath.Ext(name); ext == "" {
		if inferredExt := fileDownloadExtensionFromContentType(contentType); inferredExt != "" {
			name += inferredExt
		}
	}
	name = fileDownloadSanitizeFilename(name)
	if name == "" {
		name = "downloaded_file"
	}

	finalPath := filepath.Join(targetDir, name)
	if overwrite {
		return finalPath, nil
	}
	return fileDownloadUniquePath(finalPath)
}

func fileDownloadPrepareExistingPath(targetPath string, overwrite bool, explicitPath bool) error {
	_, err := os.Stat(targetPath)
	if err == nil && !overwrite && explicitPath {
		return fmt.Errorf("target file already exists: %s", targetPath)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func fileDownloadFilenameFromDisposition(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	if name := strings.TrimSpace(params["filename*"]); name != "" {
		if idx := strings.Index(name, "''"); idx >= 0 {
			name = name[idx+2:]
		}
		if decoded, err := url.QueryUnescape(name); err == nil {
			return decoded
		}
		return name
	}
	return strings.TrimSpace(params["filename"])
}

func fileDownloadFilenameFromURL(parsedURL *url.URL) string {
	if parsedURL == nil {
		return ""
	}
	base := path.Base(parsedURL.Path)
	if base == "." || base == "/" || strings.TrimSpace(base) == "" {
		return "downloaded_file"
	}
	if decoded, err := url.PathUnescape(base); err == nil {
		return decoded
	}
	return base
}

func fileDownloadExtensionFromContentType(contentType string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType == "" {
		return ""
	}
	exts, err := mime.ExtensionsByType(contentType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	return exts[0]
}

func fileDownloadSanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = fileDownloadUnsafeNameRe.ReplaceAllString(name, "_")
	name = strings.Trim(name, ". ")
	if name == "" {
		return ""
	}
	return name
}

func fileDownloadUniquePath(target string) (string, error) {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return target, nil
	} else if err != nil {
		return "", err
	}

	dir := filepath.Dir(target)
	ext := filepath.Ext(target)
	base := strings.TrimSuffix(filepath.Base(target), ext)
	for i := 1; i <= 999; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate unique filename for %s", target)
}

func fileDownloadSuccess(downloadURL string, finalPath string, method string, statusCode int, contentType string, attempts interface{}) (string, error) {
	info, err := os.Stat(finalPath)
	if err != nil {
		return "", err
	}
	doclib.Register(finalPath)

	out, err := json.MarshalIndent(map[string]interface{}{
		"url":          downloadURL,
		"path":         finalPath,
		"filename":     filepath.Base(finalPath),
		"size":         info.Size(),
		"method":       method,
		"status_code":  statusCode,
		"content_type": contentType,
		"success":      true,
		"attempts":     attempts,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
