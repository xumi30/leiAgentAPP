package openclawskill

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/appruntime"
	"leiAgent/logging"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"
)

const defaultInstallTimeout = 3 * time.Minute

func envWithAugmentedPath(base []string) []string {
	// GUI apps on macOS often start with a minimal PATH, so npx/node installed via
	// Homebrew won't be found. Augment PATH to common locations.
	env := append([]string(nil), base...)
	pathIdx := -1
	var cur string
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			pathIdx = i
			cur = strings.TrimPrefix(kv, "PATH=")
			break
		}
	}
	extra := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
	wantPrefix := strings.Join(extra, ":")
	if pathIdx == -1 {
		env = append(env, "PATH="+wantPrefix)
		return env
	}
	cur = strings.TrimSpace(cur)
	if cur == "" {
		env[pathIdx] = "PATH=" + wantPrefix
		return env
	}
	// If it already has the common prefixes, keep as-is.
	for _, p := range extra {
		if strings.Contains(cur, p) {
			return env
		}
	}
	env[pathIdx] = "PATH=" + wantPrefix + ":" + cur
	return env
}

type Requires struct {
	Bins []string `json:"bins" yaml:"bins"`
	Env  []string `json:"env" yaml:"env"`
}

type SkillInfo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Path          string   `json:"path"`
	Requires      Requires `json:"requires"`
	PythonDeps    []string `json:"pythonDeps"`
	PrimaryEnv    string   `json:"primaryEnv"`
	Supported     bool     `json:"supported"`
	Ready         bool     `json:"ready"`
	MissingBins   []string `json:"missingBins"`
	MissingPython []string `json:"missingPython"`
	MissingEnv    []string `json:"missingEnv"`
	Status        string   `json:"status"`
	StatusDetail  string   `json:"statusDetail"`
}

type InstallResult struct {
	OK             bool        `json:"ok"`
	Slug           string      `json:"slug"`
	Command        []string    `json:"command"`
	Output         string      `json:"output"`
	Skills         []SkillInfo `json:"skills"`
	WorkspaceRoot  string      `json:"workspaceRoot"`
	SkillsRoot     string      `json:"skillsRoot"`
	DurationMillis int64       `json:"durationMillis"`
	ExitCode       int         `json:"exitCode"`
	Force          bool        `json:"force"`
	RequiresForce  bool        `json:"requiresForce"`
	ErrorCode      string      `json:"errorCode,omitempty"`
	Message        string      `json:"message,omitempty"`
}

type OpenClawDeleteResult struct {
	OK             bool        `json:"ok"`
	Path           string      `json:"path"`
	Message        string      `json:"message"`
	WorkspaceRoot  string      `json:"workspaceRoot"`
	SkillsRoot     string      `json:"skillsRoot"`
	DurationMillis int64       `json:"durationMillis"`
	Skills         []SkillInfo `json:"skills"`
}

type OpenClawDepsResult struct {
	OK             bool        `json:"ok"`
	Skill          string      `json:"skill"`
	Path           string      `json:"path"`
	Command        []string    `json:"command"`
	Output         string      `json:"output"`
	Message        string      `json:"message"`
	DurationMillis int64       `json:"durationMillis"`
	Skills         []SkillInfo `json:"skills"`
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		OpenClaw struct {
			Requires   Requires `yaml:"requires"`
			PrimaryEnv string   `yaml:"primaryEnv"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`
}

type configRoot struct {
	OpenClaw struct {
		Env map[string]string `yaml:"env"`
	} `yaml:"openclaw"`
}

type clawdSkillPackage struct {
	ID          string      `json:"id"`
	OwnerID     string      `json:"owner_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	Icon        string      `json:"icon"`
	Author      string      `json:"owner_name"`
	Metadata    interface{} `json:"metadata"`
	Readme      string      `json:"readme"`
	Files       interface{} `json:"files"`
}

var skillSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@-]*$`)
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var clawdInstallURLPattern = regexp.MustCompile(`backend\.clawd\.org\.cn/api/skills/([^"'\s|]+)/install\.sh`)

const clawdAPIBaseURL = "https://backend.clawd.org.cn/api"

type installRequest struct {
	Slug  string
	Force bool
	Mode  string
}

func WorkspaceRoot() string {
	if root := strings.TrimSpace(os.Getenv("LEIAGENT_SKILLS_WORKDIR")); root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			return abs
		}
		return filepath.Clean(root)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if abs, err := filepath.Abs(wd); err == nil {
		return abs
	}
	return wd
}

func SkillsRoot() string {
	return filepath.Join(WorkspaceRoot(), "skills")
}

func ConfigEnv() map[string]string {
	path := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH"))
	if path == "" {
		path = appruntime.ResolvePath(filepath.Join("config", "config.yaml"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root configRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		logging.Warn("OpenClaw skill config env parse failed: path=%s err=%v", path, err)
		return nil
	}
	env := map[string]string{}
	for key, value := range root.OpenClaw.Env {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(value) == "" {
			continue
		}
		env[key] = value
	}
	return env
}

func ParseInstallInput(input string) (string, error) {
	req, err := parseInstallRequest(input)
	if err != nil {
		return "", err
	}
	return req.Slug, nil
}

func parseInstallRequest(input string) (installRequest, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return installRequest{}, fmt.Errorf("请输入 skill 名称或 ClawHub 安装命令")
	}
	if match := clawdInstallURLPattern.FindStringSubmatch(input); len(match) == 2 {
		decoded, err := url.PathUnescape(match[1])
		if err != nil {
			return installRequest{}, fmt.Errorf("解析 clawd install URL 失败：%w", err)
		}
		slug, err := validateSlug(decoded)
		return installRequest{Slug: slug, Force: hasForceFlag(strings.Fields(input)), Mode: "clawd"}, err
	}
	fields := strings.Fields(input)
	force := hasForceFlag(fields)
	if len(fields) == 1 {
		slug, err := validateSlug(strings.Trim(fields[0], `"'`))
		return installRequest{Slug: slug, Force: force, Mode: installModeForSlug(slug)}, err
	}
	for i, field := range fields {
		if field != "install" || i+1 >= len(fields) {
			continue
		}
		for _, candidate := range fields[i+1:] {
			candidate = strings.Trim(candidate, `"'`)
			if candidate == "" || strings.HasPrefix(candidate, "-") {
				continue
			}
			slug, err := validateSlug(candidate)
			return installRequest{Slug: slug, Force: force, Mode: installModeForSlug(slug)}, err
		}
	}
	return installRequest{}, fmt.Errorf("未能从安装命令中解析 skill 名称")
}

func Install(ctx context.Context, input string) (InstallResult, error) {
	start := time.Now()
	req, err := parseInstallRequest(input)
	if err != nil {
		logging.Warn("OpenClaw skill install parse failed: input=%q err=%v", input, err)
		return InstallResult{
			OK:             false,
			DurationMillis: time.Since(start).Milliseconds(),
			ErrorCode:      "parse_failed",
			Message:        err.Error(),
			WorkspaceRoot:  WorkspaceRoot(),
			SkillsRoot:     SkillsRoot(),
			ExitCode:       -1,
		}, err
	}
	slug := req.Slug
	logging.Info("OpenClaw skill install requested: slug=%s force=%t mode=%s workspace=%s skillsRoot=%s", slug, req.Force, req.Mode, WorkspaceRoot(), SkillsRoot())
	if err := os.MkdirAll(SkillsRoot(), 0o755); err != nil {
		logging.Error("OpenClaw skill install mkdir failed: slug=%s skillsRoot=%s err=%v", slug, SkillsRoot(), err)
		return InstallResult{
			OK:             false,
			Slug:           slug,
			WorkspaceRoot:  WorkspaceRoot(),
			SkillsRoot:     SkillsRoot(),
			DurationMillis: time.Since(start).Milliseconds(),
			ExitCode:       -1,
			Force:          req.Force,
			ErrorCode:      "mkdir_failed",
			Message:        err.Error(),
		}, fmt.Errorf("创建 skills 目录失败：%w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultInstallTimeout)
	defer cancel()

	if req.Mode == "clawd" {
		return installFromClawd(ctx, req, start)
	}

	// If npx is not available (common in packaged desktop apps), fall back to clawd HTTP install.
	if _, lookErr := exec.LookPath("npx"); lookErr != nil {
		logging.Warn("OpenClaw install: npx not found, fallback to clawd mode: slug=%s err=%v", slug, lookErr)
		req.Mode = "clawd"
		return installFromClawd(ctx, req, start)
	}

	args := []string{"-y", "clawhub@latest", "install", slug, "--workdir", WorkspaceRoot()}
	if req.Force {
		args = append(args, "--force")
	}
	command := append([]string{"npx"}, args...)
	logging.Info("OpenClaw skill install command: slug=%s command=%s", slug, strings.Join(command, " "))
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Env = envWithAugmentedPath(append(os.Environ(), "CLAWHUB_WORKDIR="+WorkspaceRoot()))
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	out := strings.TrimSpace(buf.String())
	duration := time.Since(start)
	exitCode := commandExitCode(err)
	requiresForce := outputRequiresForce(out)
	result := InstallResult{
		OK:             err == nil,
		Slug:           slug,
		Command:        command,
		Output:         out,
		Skills:         Scan(),
		WorkspaceRoot:  WorkspaceRoot(),
		SkillsRoot:     SkillsRoot(),
		DurationMillis: duration.Milliseconds(),
		ExitCode:       exitCode,
		Force:          req.Force,
		RequiresForce:  requiresForce,
	}
	logging.Info("OpenClaw skill install finished: slug=%s ok=%t force=%t requiresForce=%t exitCode=%d duration=%s output=%s", slug, result.OK, req.Force, requiresForce, exitCode, duration, compactLogText(out))
	if ctx.Err() == context.DeadlineExceeded {
		result.ErrorCode = "timeout"
		result.Message = "安装超时，请检查网络或稍后重试"
		logging.Warn("OpenClaw skill install timeout: slug=%s duration=%s", slug, duration)
		return result, fmt.Errorf("安装超时，请检查网络或稍后重试")
	}
	if err != nil {
		result.ErrorCode = installErrorCode(out, err)
		if out != "" {
			result.Message = installFailureMessage(out)
			return result, fmt.Errorf("%s", result.Message)
		}
		result.Message = fmt.Sprintf("安装失败：%v", err)
		return result, fmt.Errorf("安装失败：%w", err)
	}
	result.Skills = Scan()
	result.Message = "安装完成"
	return result, nil
}

func installFromClawd(ctx context.Context, req installRequest, start time.Time) (InstallResult, error) {
	slug := req.Slug
	target := filepath.Join(SkillsRoot(), folderNameFromSlug(slug))
	if result, err := installFromClawdPackage(ctx, req, start, target); err == nil {
		return result, nil
	} else {
		logging.Warn("OpenClaw clawd package install failed, fallback to zip: slug=%s err=%v", slug, err)
	}
	if result, err := installFromClawdRegistry(ctx, req, start, target); err == nil {
		return result, nil
	} else {
		logging.Warn("OpenClaw clawd registry install failed, fallback to zip: slug=%s err=%v", slug, err)
	}
	return installFromClawdZip(ctx, req, start, target)
}

func installFromClawdRegistry(ctx context.Context, req installRequest, start time.Time, target string) (InstallResult, error) {
	slug := req.Slug
	searchURL, err := url.Parse(clawdAPIBaseURL + "/skills")
	if err != nil {
		return InstallResult{}, err
	}
	q := searchURL.Query()
	q.Set("q", skillNameFromSlug(slug))
	searchURL.RawQuery = q.Encode()
	command := []string{"GET", searchURL.String()}
	result := InstallResult{
		OK:            false,
		Slug:          slug,
		Command:       command,
		WorkspaceRoot: WorkspaceRoot(),
		SkillsRoot:    SkillsRoot(),
		ExitCode:      -1,
		Force:         req.Force,
	}
	logging.Info("OpenClaw clawd registry install: slug=%s target=%s url=%s", slug, target, searchURL.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return result, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("registry endpoint failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var packages []clawdSkillPackage
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return result, fmt.Errorf("解析 registry 响应失败：%w", err)
	}
	pkg, ok := pickClawdRegistryPackage(packages, slug)
	if !ok {
		return result, fmt.Errorf("registry 未找到 skill：%s", slug)
	}
	if strings.TrimSpace(pkg.ID) == "" {
		pkg.ID = slug
	}
	if err := os.RemoveAll(target); err != nil {
		return result, fmt.Errorf("清理旧 skill 目录失败：%w", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return result, fmt.Errorf("创建 skill 目录失败：%w", err)
	}
	if err := restoreClawdFiles(target, pkg.Files); err != nil {
		return result, err
	}
	if err := writeClawdSkillMD(target, pkg); err != nil {
		return result, err
	}
	result.OK = true
	result.ExitCode = 0
	result.DurationMillis = time.Since(start).Milliseconds()
	result.Message = "安装完成"
	result.Output = fmt.Sprintf("Installed %s -> %s", slug, target)
	result.Skills = Scan()
	logging.Info("OpenClaw clawd registry install finished: slug=%s target=%s durationMs=%d", slug, target, result.DurationMillis)
	return result, nil
}

func pickClawdRegistryPackage(packages []clawdSkillPackage, slug string) (clawdSkillPackage, bool) {
	slug = strings.TrimSpace(slug)
	name := skillNameFromSlug(slug)
	for _, pkg := range packages {
		if strings.EqualFold(strings.TrimSpace(pkg.ID), slug) {
			return pkg, true
		}
	}
	for _, pkg := range packages {
		if strings.EqualFold(strings.TrimSpace(pkg.Name), name) {
			return pkg, true
		}
	}
	for _, pkg := range packages {
		if strings.EqualFold(skillNameFromSlug(pkg.ID), name) {
			return pkg, true
		}
	}
	return clawdSkillPackage{}, false
}

func installFromClawdPackage(ctx context.Context, req installRequest, start time.Time, target string) (InstallResult, error) {
	slug := req.Slug
	encoded := url.PathEscape(slug)
	packageURL := fmt.Sprintf("%s/skills/%s/package", clawdAPIBaseURL, encoded)
	command := []string{"GET", packageURL}
	result := InstallResult{
		OK:            false,
		Slug:          slug,
		Command:       command,
		WorkspaceRoot: WorkspaceRoot(),
		SkillsRoot:    SkillsRoot(),
		ExitCode:      -1,
		Force:         req.Force,
	}
	logging.Info("OpenClaw clawd package install: slug=%s target=%s url=%s", slug, target, packageURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return result, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("package endpoint failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var pkg clawdSkillPackage
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return result, fmt.Errorf("解析 package 响应失败：%w", err)
	}
	if strings.TrimSpace(pkg.ID) == "" {
		pkg.ID = slug
	}
	if err := os.RemoveAll(target); err != nil {
		return result, fmt.Errorf("清理旧 skill 目录失败：%w", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return result, fmt.Errorf("创建 skill 目录失败：%w", err)
	}
	if err := restoreClawdFiles(target, pkg.Files); err != nil {
		return result, err
	}
	if err := writeClawdSkillMD(target, pkg); err != nil {
		return result, err
	}
	result.OK = true
	result.ExitCode = 0
	result.DurationMillis = time.Since(start).Milliseconds()
	result.Message = "安装完成"
	result.Output = fmt.Sprintf("Installed %s -> %s", slug, target)
	result.Skills = Scan()
	logging.Info("OpenClaw clawd package install finished: slug=%s target=%s durationMs=%d", slug, target, result.DurationMillis)
	return result, nil
}

func installFromClawdZip(ctx context.Context, req installRequest, start time.Time, target string) (InstallResult, error) {
	slug := req.Slug
	encoded := url.PathEscape(slug)
	downloadURL := fmt.Sprintf("%s/skills/%s/download", clawdAPIBaseURL, encoded)
	command := []string{"GET", downloadURL}
	result := InstallResult{
		OK:            false,
		Slug:          slug,
		Command:       command,
		WorkspaceRoot: WorkspaceRoot(),
		SkillsRoot:    SkillsRoot(),
		ExitCode:      -1,
		Force:         req.Force,
	}
	logging.Info("OpenClaw clawd install download: slug=%s target=%s url=%s", slug, target, downloadURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		result.ErrorCode = "request_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		result.ErrorCode = "network_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, fmt.Errorf("下载 skill 失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		result.ErrorCode = "download_failed"
		result.Output = strings.TrimSpace(string(body))
		result.Message = fmt.Sprintf("下载 skill 失败：HTTP %d %s", resp.StatusCode, result.Output)
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, fmt.Errorf("%s", result.Message)
	}
	tmp, err := os.CreateTemp("", "leiagent-clawd-skill-*.zip")
	if err != nil {
		result.ErrorCode = "temp_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		result.ErrorCode = "download_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, fmt.Errorf("保存 skill 下载包失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		result.ErrorCode = "temp_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		result.ErrorCode = "mkdir_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, fmt.Errorf("创建 skill 目录失败：%w", err)
	}
	if err := unzipSkill(tmpPath, target); err != nil {
		result.ErrorCode = "extract_failed"
		result.Message = err.Error()
		result.DurationMillis = time.Since(start).Milliseconds()
		return result, err
	}
	result.OK = true
	result.ExitCode = 0
	result.DurationMillis = time.Since(start).Milliseconds()
	result.Message = "安装完成"
	result.Output = fmt.Sprintf("Installed %s -> %s", slug, target)
	result.Skills = Scan()
	logging.Info("OpenClaw clawd install finished: slug=%s target=%s durationMs=%d", slug, target, result.DurationMillis)
	return result, nil
}

func restoreClawdFiles(target string, raw interface{}) error {
	files := map[string]string{}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			_ = json.Unmarshal([]byte(v), &files)
		}
	case map[string]interface{}:
		for key, value := range v {
			if s, ok := value.(string); ok {
				files[key] = s
			}
		}
	case map[string]string:
		files = v
	}
	for relPath, content := range files {
		relPath = filepath.Clean(relPath)
		if relPath == "." || strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}
		targetPath := filepath.Join(target, relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeClawdSkillMD(target string, pkg clawdSkillPackage) error {
	metadata := map[string]interface{}{}
	switch v := pkg.Metadata.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			_ = json.Unmarshal([]byte(v), &metadata)
		}
	case map[string]interface{}:
		metadata = v
	}
	frontmatter := map[string]interface{}{
		"id":          pkg.ID,
		"owner_id":    pkg.OwnerID,
		"name":        pkg.Name,
		"description": pkg.Description,
		"version":     pkg.Version,
		"icon":        pkg.Icon,
		"author":      pkg.Author,
	}
	if len(metadata) > 0 {
		frontmatter["metadata"] = metadata
	}
	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return err
	}
	content := "---\n" + string(yamlData) + "---\n\n" + pkg.Readme
	return os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(content), 0o644)
}

func unzipSkill(zipPath, target string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("读取 skill zip 失败：%w", err)
	}
	defer reader.Close()
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		name := filepath.Clean(file.Name)
		if name == "." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return fmt.Errorf("zip 包含不安全路径：%s", file.Name)
		}
		dest := filepath.Join(targetAbs, name)
		destAbs, err := filepath.Abs(dest)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(targetAbs, destAbs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("zip 解压路径越界：%s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destAbs, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(destAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func Scan() []SkillInfo {
	patterns := []string{
		filepath.Join(SkillsRoot(), "*", "SKILL.md"),
		filepath.Join(SkillsRoot(), "*", "*", "SKILL.md"),
	}
	seen := map[string]struct{}{}
	out := make([]SkillInfo, 0)
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			clean := filepath.Clean(path)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			info := readSkillInfo(clean)
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Delete(skillPath string) (OpenClawDeleteResult, error) {
	start := time.Now()
	target, err := validateSkillPath(skillPath)
	result := OpenClawDeleteResult{
		OK:             false,
		Path:           filepath.Clean(skillPath),
		WorkspaceRoot:  WorkspaceRoot(),
		SkillsRoot:     SkillsRoot(),
		DurationMillis: 0,
	}
	if err != nil {
		result.DurationMillis = time.Since(start).Milliseconds()
		result.Message = err.Error()
		logging.Warn("OpenClaw skill delete rejected: path=%s err=%v", skillPath, err)
		return result, err
	}

	logging.Info("OpenClaw skill delete requested: path=%s", target)
	if err := os.RemoveAll(target); err != nil {
		result.DurationMillis = time.Since(start).Milliseconds()
		result.Message = err.Error()
		logging.Error("OpenClaw skill delete failed: path=%s err=%v", target, err)
		return result, fmt.Errorf("删除 skill 失败：%w", err)
	}
	result.OK = true
	result.Path = target
	result.DurationMillis = time.Since(start).Milliseconds()
	result.Message = "删除完成"
	result.Skills = Scan()
	logging.Info("OpenClaw skill delete finished: path=%s durationMs=%d", target, result.DurationMillis)
	return result, nil
}

func Find(name string) (SkillInfo, bool) {
	target := strings.ToLower(strings.TrimSpace(name))
	for _, skill := range Scan() {
		base := strings.ToLower(strings.TrimSpace(filepath.Base(skill.Path)))
		sn := strings.ToLower(strings.TrimSpace(skill.Name))
		// Exact matches: skill name or folder name.
		if sn == target || base == target {
			return skill, true
		}
		// Fuzzy folder match for namespaced installs, e.g. official__baidu-search.
		if target != "" && base != "" && strings.Contains(base, target) {
			return skill, true
		}
	}
	return SkillInfo{}, false
}

func BaiduSearchScriptPath() (string, error) {
	skill, ok := Find("baidu-search")
	if !ok {
		return "", fmt.Errorf("未安装 baidu-search skill：可在设置页粘贴 `claw skill install official/baidu-search` 安装")
	}
	script := filepath.Join(skill.Path, "scripts", "search.py")
	if st, err := os.Stat(script); err != nil || st.IsDir() {
		return "", fmt.Errorf("baidu-search 已安装，但缺少脚本：%s", script)
	}
	return script, nil
}

func CheckRequirements(info SkillInfo) SkillInfo {
	info.MissingBins = nil
	info.MissingPython = nil
	info.MissingEnv = nil
	for _, bin := range info.Requires.Bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err != nil {
			info.MissingBins = append(info.MissingBins, bin)
		}
	}
	for _, key := range info.Requires.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		_, source := ResolveEnvValue(key)
		if source == "" {
			info.MissingEnv = append(info.MissingEnv, key)
			logging.Warn("OpenClaw skill env missing: skill=%s key=%s", info.Name, key)
		} else {
			logging.Info("OpenClaw skill env resolved: skill=%s key=%s source=%s", info.Name, key, source)
		}
	}
	for _, dep := range info.PythonDeps {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if !pythonDependencyAvailable(info, dep) {
			info.MissingPython = append(info.MissingPython, dep)
		}
	}
	info.Ready = len(info.MissingBins) == 0 && len(info.MissingEnv) == 0 && len(info.MissingPython) == 0
	if info.Ready {
		info.Status = "ok"
		info.StatusDetail = "可用"
	} else {
		info.Status = "warning"
		parts := make([]string, 0, 2)
		if len(info.MissingBins) > 0 {
			parts = append(parts, "缺少命令："+strings.Join(info.MissingBins, ", "))
		}
		if len(info.MissingPython) > 0 {
			parts = append(parts, "缺少 Python 依赖："+strings.Join(info.MissingPython, ", "))
		}
		if len(info.MissingEnv) > 0 {
			parts = append(parts, "缺少环境变量："+strings.Join(info.MissingEnv, ", "))
		}
		info.StatusDetail = strings.Join(parts, "；")
	}
	return info
}

func InstallDeps(skillPath string) (OpenClawDepsResult, error) {
	start := time.Now()
	target, err := validateSkillPath(skillPath)
	result := OpenClawDepsResult{
		OK:    false,
		Path:  filepath.Clean(skillPath),
		Skill: filepath.Base(filepath.Clean(skillPath)),
	}
	if err != nil {
		result.DurationMillis = time.Since(start).Milliseconds()
		result.Message = err.Error()
		return result, err
	}
	info := readSkillInfo(filepath.Join(target, "SKILL.md"))
	if len(info.PythonDeps) == 0 {
		result.OK = true
		result.Path = target
		result.Skill = info.Name
		result.Message = "没有需要安装的 Python 依赖"
		result.DurationMillis = time.Since(start).Milliseconds()
		result.Skills = Scan()
		return result, nil
	}
	python, err := ensureSkillVenv(target)
	if err != nil {
		result.Path = target
		result.Skill = info.Name
		result.DurationMillis = time.Since(start).Milliseconds()
		result.Message = err.Error()
		return result, err
	}
	args := append([]string{"-m", "pip", "install"}, info.PythonDeps...)
	cmd := exec.Command(python, args...)
	cmd.Dir = target
	cmd.Env = EnvForSkill(info)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	out := strings.TrimSpace(buf.String())
	command := append([]string{python}, args...)
	result.Path = target
	result.Skill = info.Name
	result.Command = command
	result.Output = out
	result.DurationMillis = time.Since(start).Milliseconds()
	result.OK = err == nil
	result.Skills = Scan()
	logging.Info("OpenClaw skill deps install finished: skill=%s ok=%t command=%s output=%s", info.Name, result.OK, strings.Join(command, " "), compactLogText(out))
	if err != nil {
		if out != "" {
			result.Message = "依赖安装失败：" + out
			return result, fmt.Errorf("%s", result.Message)
		}
		result.Message = fmt.Sprintf("依赖安装失败：%v", err)
		return result, fmt.Errorf("%s", result.Message)
	}
	result.Skills = Scan()
	result.Message = "依赖安装完成"
	return result, nil
}

func EnvForCommand() []string {
	return EnvForSkill(SkillInfo{})
}

func EnvForSkill(info SkillInfo) []string {
	env := os.Environ()
	for key, value := range ConfigEnv() {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(os.Getenv(key)) != "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	for _, key := range info.Requires.Env {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(os.Getenv(key)) != "" {
			continue
		}
		value, source := ResolveEnvValue(key)
		if value == "" || source == "process" || source == "config" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

func ResolveEnvValue(key string) (string, string) {
	key = strings.TrimSpace(key)
	if !envKeyPattern.MatchString(key) {
		return "", ""
	}
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, "process"
	}
	if value := strings.TrimSpace(ConfigEnv()[key]); value != "" {
		return value, "config"
	}
	if runtime.GOOS == "darwin" {
		if value := readLaunchctlEnv(key); value != "" {
			return value, "launchctl"
		}
	}
	if value := readShellEnv(key); value != "" {
		return value, "login_shell"
	}
	return "", ""
}

func readLaunchctlEnv(key string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "launchctl", "getenv", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readShellEnv(key string) string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		if runtime.GOOS == "windows" {
			return ""
		}
		shell = "/bin/zsh"
	}
	if _, err := os.Stat(shell); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lic", "printenv "+key).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if value := strings.TrimSpace(lines[i]); value != "" {
			return value
		}
	}
	return ""
}

func readSkillInfo(skillFile string) SkillInfo {
	dir := filepath.Dir(skillFile)
	info := SkillInfo{
		Name:        filepath.Base(dir),
		Path:        dir,
		Supported:   false,
		Status:      "unknown",
		Description: "",
	}
	data, err := os.ReadFile(skillFile)
	if err != nil {
		info.Status = "error"
		info.StatusDetail = err.Error()
		return info
	}
	fm := parseFrontmatter(data)
	if strings.TrimSpace(fm.Name) != "" {
		info.Name = strings.TrimSpace(fm.Name)
	}
	info.Description = strings.TrimSpace(fm.Description)
	info.Requires = fm.Metadata.OpenClaw.Requires
	info.PrimaryEnv = strings.TrimSpace(fm.Metadata.OpenClaw.PrimaryEnv)
	info.PythonDeps = inferPythonDeps(info.Name, dir)
	info.Supported = isSupported(info.Name)
	return CheckRequirements(info)
}

func parseFrontmatter(data []byte) frontmatter {
	var fm frontmatter
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return fm
	}
	rest := trimmed[3:]
	if bytes.HasPrefix(rest, []byte("\r\n")) {
		rest = rest[2:]
	} else if bytes.HasPrefix(rest, []byte("\n")) {
		rest = rest[1:]
	}
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return fm
	}
	_ = yaml.Unmarshal(rest[:idx], &fm)
	return fm
}

func isSupported(name string) bool {
	return strings.TrimSpace(name) != ""
}

func inferPythonDeps(name, dir string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	reqFile := filepath.Join(dir, "requirements.txt")
	if data, err := os.ReadFile(reqFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			dep := strings.Fields(line)[0]
			if _, ok := seen[dep]; !ok {
				seen[dep] = struct{}{}
				out = append(out, dep)
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(name), "baidu-search") {
		if _, ok := seen["requests"]; !ok {
			out = append(out, "requests")
		}
	}
	return out
}

func pythonDependencyAvailable(info SkillInfo, dep string) bool {
	module := pythonModuleName(dep)
	if module == "" {
		return true
	}
	python := skillPythonPath(info.Path)
	if _, err := os.Stat(python); err != nil {
		python = "python3"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", "import "+module)
	cmd.Dir = info.Path
	cmd.Env = EnvForSkill(info)
	return cmd.Run() == nil
}

func pythonModuleName(dep string) string {
	dep = strings.TrimSpace(dep)
	if dep == "" {
		return ""
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "["} {
		if idx := strings.Index(dep, sep); idx >= 0 {
			dep = dep[:idx]
		}
	}
	return strings.ReplaceAll(strings.TrimSpace(dep), "-", "_")
}

func skillPythonPath(skillPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(skillPath, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(skillPath, ".venv", "bin", "python")
}

func SkillPythonPath(skillPath string) string {
	python := skillPythonPath(skillPath)
	if st, err := os.Stat(python); err == nil && !st.IsDir() {
		return python
	}
	return ""
}

func ensureSkillVenv(skillPath string) (string, error) {
	python := skillPythonPath(skillPath)
	if st, err := os.Stat(python); err == nil && !st.IsDir() {
		return python, nil
	}
	cmd := exec.Command("python3", "-m", "venv", ".venv")
	cmd.Dir = skillPath
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(buf.String())
		if out != "" {
			return "", fmt.Errorf("创建 Python venv 失败：%s", out)
		}
		return "", fmt.Errorf("创建 Python venv 失败：%w", err)
	}
	if st, err := os.Stat(python); err == nil && !st.IsDir() {
		return python, nil
	}
	return "", fmt.Errorf("创建 Python venv 后未找到解释器：%s", python)
}

func validateSlug(slug string) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", fmt.Errorf("skill 名称不能为空")
	}
	if !skillSlugPattern.MatchString(slug) || strings.Contains(slug, "..") {
		return "", fmt.Errorf("skill 名称不安全：%q", slug)
	}
	return slug, nil
}

func installModeForSlug(slug string) string {
	if strings.Contains(slug, "/") {
		return "clawd"
	}
	return "clawhub"
}

func skillNameFromSlug(slug string) string {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return "skill"
	}
	parts := strings.Split(slug, "/")
	name := strings.TrimSpace(parts[len(parts)-1])
	if name == "" {
		return "skill"
	}
	return name
}

func folderNameFromSlug(slug string) string {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if strings.Contains(slug, "/") {
		return strings.ReplaceAll(slug, "/", "__")
	}
	return skillNameFromSlug(slug)
}

func validateSkillPath(skillPath string) (string, error) {
	skillPath = strings.TrimSpace(skillPath)
	if skillPath == "" {
		return "", fmt.Errorf("skill path 不能为空")
	}
	targetAbs, err := filepath.Abs(filepath.Clean(skillPath))
	if err != nil {
		return "", fmt.Errorf("解析 skill path 失败：%w", err)
	}
	rootAbs, err := filepath.Abs(SkillsRoot())
	if err != nil {
		return "", fmt.Errorf("解析 skillsRoot 失败：%w", err)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("校验 skill path 失败：%w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("拒绝删除 skills 目录外的路径：%s", targetAbs)
	}
	st, err := os.Stat(targetAbs)
	if err != nil {
		return "", fmt.Errorf("skill 不存在：%s", targetAbs)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("skill path 不是目录：%s", targetAbs)
	}
	skillFile := filepath.Join(targetAbs, "SKILL.md")
	if st, err := os.Stat(skillFile); err != nil || st.IsDir() {
		return "", fmt.Errorf("目标目录缺少 SKILL.md，拒绝删除：%s", targetAbs)
	}
	return targetAbs, nil
}

func hasForceFlag(fields []string) bool {
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "--force" || field == "-f" {
			return true
		}
	}
	return false
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func outputRequiresForce(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "--force") && strings.Contains(lower, "suspicious")
}

func installErrorCode(out string, err error) string {
	if outputRequiresForce(out) {
		return "requires_force"
	}
	lower := strings.ToLower(out)
	switch {
	case strings.Contains(lower, "command not found") || strings.Contains(lower, "executable file not found"):
		return "command_not_found"
	case strings.Contains(lower, "network") || strings.Contains(lower, "econn") || strings.Contains(lower, "timeout"):
		return "network_failed"
	case strings.Contains(lower, "not found"):
		return "skill_not_found"
	default:
		if _, ok := err.(*exec.ExitError); ok {
			return "command_failed"
		}
		return "execution_failed"
	}
}

func installFailureMessage(out string) string {
	if outputRequiresForce(out) {
		return "安装被 ClawHub 安全策略拦截：该 skill 被标记为 suspicious。请先审查 skill 代码；确认仍要安装时，在安装命令末尾显式添加 --force 后重试。原始输出：" + out
	}
	return "安装失败：" + out
}

func compactLogText(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	lines := strings.Fields(s)
	if len(lines) == 0 {
		return ""
	}
	compact := strings.Join(lines, " ")
	if len(compact) > 2000 {
		return compact[:2000] + "...(truncated)"
	}
	return compact
}

func MarshalForCommand(input map[string]interface{}) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
