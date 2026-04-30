package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"leiAgent/internal/appruntime"
)

const mcpInstallTimeout = 3 * time.Minute

type MCPHubInstallResult struct {
	Row     MCPConfigRow `json:"row"`
	Message string       `json:"message"`
}

func PrepareMCPHubDeployment(detail MCPHubPluginDetail, opt MCPHubDeploymentOption, existingRows []MCPConfigRow) (MCPHubInstallResult, error) {
	row := rowFromHubDeployment(detail, opt, existingRows)
	if !needsLocalMCPInstall(row) {
		return MCPHubInstallResult{Row: row, Message: "该 MCP 可直接通过 command/args 启动"}, nil
	}

	repoURL := firstNonBlankString(githubCloneURLFromDeployment(opt), githubCloneURLFromDetail(detail))
	if repoURL == "" {
		return MCPHubInstallResult{}, fmt.Errorf("该 MCP 配置包含本地源码占位路径，但 Hub 未提供可下载的 GitHub 仓库地址")
	}

	ctx, cancel := context.WithTimeout(context.Background(), mcpInstallTimeout)
	defer cancel()

	installDir := filepath.Join(appruntime.ResolvePath("data/mcp_servers"), safePathSegment(row.Label))
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return MCPHubInstallResult{}, err
	}
	if err := ensureGitCheckout(ctx, repoURL, installDir); err != nil {
		return MCPHubInstallResult{}, err
	}
	packageDir, err := findNodePackageDir(installDir)
	if err != nil {
		return MCPHubInstallResult{}, err
	}
	if packageDir != "" {
		if err := ensureNodePackageBuilt(ctx, packageDir, row.ArgsText); err != nil {
			return MCPHubInstallResult{}, err
		}
	}

	novelsDir := appruntime.ResolvePath(filepath.Join("workspace", "novels"))
	if err := os.MkdirAll(novelsDir, 0o755); err != nil {
		return MCPHubInstallResult{}, err
	}
	row.ArgsText = replaceMCPPlaceholderArgs(row.ArgsText, installDir, novelsDir)
	row.LastCheckState = ""
	row.LastCheckMessage = ""
	row.LastCheckedAt = ""
	row.CachedTools = nil
	row.CachedToolDetails = nil

	return MCPHubInstallResult{
		Row:     row,
		Message: fmt.Sprintf("已下载到 %s 并完成本地准备", installDir),
	}, nil
}

func rowFromHubDeployment(detail MCPHubPluginDetail, opt MCPHubDeploymentOption, existingRows []MCPConfigRow) MCPConfigRow {
	connection := opt.Connection
	command := strings.TrimSpace(connection.Command)
	urlText := strings.TrimSpace(connection.URL)
	rawType := strings.ToLower(strings.TrimSpace(connection.Type))
	transportType := "streamable_http"
	if command != "" {
		transportType = "stdio"
	} else if rawType == "sse" {
		transportType = "sse"
	}
	return MCPConfigRow{
		Label:         uniqueMCPLabel(existingRows, firstNonBlankString(detail.Identifier, detail.Name, "mcp-service")),
		TransportType: transportType,
		URL:           urlText,
		Command:       command,
		ArgsText:      strings.Join(connection.Args, "\n"),
		HeadersText:   mapToLines(connection.Headers),
		EnvText:       mapToLines(connection.Env),
	}
}

func needsLocalMCPInstall(row MCPConfigRow) bool {
	sample := strings.ToLower(strings.Join([]string{row.Command, row.ArgsText}, "\n"))
	return strings.Contains(sample, "path/to/") || strings.Contains(sample, "/path/to/") || strings.Contains(sample, "<path")
}

func ensureGitCheckout(ctx context.Context, repoURL, dir string) error {
	if hasPath(filepath.Join(dir, ".git")) || hasPath(filepath.Join(dir, "package.json")) {
		return nil
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("目标安装目录已存在但不像可用源码目录：%s", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("下载 MCP 源码失败：%s", msg)
	}
	return nil
}

func findNodePackageDir(root string) (string, error) {
	if hasPath(filepath.Join(root, "package.json")) {
		return root, nil
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found != "" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel != "." && strings.Count(rel, string(os.PathSeparator)) >= 3 {
			return filepath.SkipDir
		}
		if d.Name() == "node_modules" || d.Name() == ".git" {
			return filepath.SkipDir
		}
		if hasPath(filepath.Join(path, "package.json")) {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found, err
}

func ensureNodePackageBuilt(ctx context.Context, packageDir, argsText string) error {
	if !hasPath(filepath.Join(packageDir, "node_modules")) {
		if err := runInstallCommand(ctx, packageDir); err != nil {
			return err
		}
	}
	if packageHasBuildScript(filepath.Join(packageDir, "package.json")) && shouldRunBuild(packageDir, argsText) {
		cmd := exec.CommandContext(ctx, "npm", "run", "build")
		cmd.Dir = packageDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("构建 MCP 失败：%s", msg)
		}
	}
	return nil
}

func runInstallCommand(ctx context.Context, packageDir string) error {
	args := []string{"install"}
	if hasPath(filepath.Join(packageDir, "package-lock.json")) {
		args = []string{"ci"}
	}
	cmd := exec.CommandContext(ctx, "npm", args...)
	cmd.Dir = packageDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("安装 MCP 依赖失败：%s", msg)
	}
	return nil
}

func packageHasBuildScript(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts["build"]) != ""
}

func shouldRunBuild(packageDir, argsText string) bool {
	if strings.Contains(strings.ToLower(argsText), "dist/") && !hasPath(filepath.Join(packageDir, "dist")) {
		return true
	}
	if !hasPath(filepath.Join(packageDir, "dist")) && !hasPath(filepath.Join(packageDir, "build")) {
		return true
	}
	return false
}

func replaceMCPPlaceholderArgs(argsText, installDir, novelsDir string) string {
	lines := strings.Split(argsText, "\n")
	for i, line := range lines {
		s := strings.TrimSpace(line)
		switch {
		case strings.Contains(s, "path/to/novels") || strings.Contains(s, "/path/to/novels"):
			lines[i] = replacePathToToken(line, "novels", novelsDir)
		case strings.Contains(s, "path/to/"):
			lines[i] = replaceSourcePlaceholder(line, installDir)
		case strings.Contains(s, "/path/to/"):
			lines[i] = replaceSourcePlaceholder(line, installDir)
		}
	}
	return strings.Join(lines, "\n")
}

func replacePathToToken(s, token, replacement string) string {
	re := regexp.MustCompile(`/?path/to/` + regexp.QuoteMeta(token))
	return re.ReplaceAllString(s, filepath.ToSlash(replacement))
}

func replaceSourcePlaceholder(s, installDir string) string {
	normalized := strings.ReplaceAll(s, "\\", "/")
	idx := strings.Index(normalized, "path/to/")
	if idx < 0 {
		return s
	}
	prefix := s[:idx]
	rest := normalized[idx+len("path/to/"):]
	parts := strings.Split(rest, "/")
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], "/")
	} else {
		rest = ""
	}
	if rest == "" {
		return prefix + filepath.ToSlash(installDir)
	}
	return prefix + filepath.ToSlash(filepath.Join(installDir, filepath.FromSlash(rest)))
}

func githubCloneURLFromDetail(detail MCPHubPluginDetail) string {
	candidates := []string{detail.Homepage}
	candidates = append(candidates, stringsFromAny(detail.Github)...)
	candidates = append(candidates, stringsFromAny(detail.Overview)...)
	for _, c := range candidates {
		if clone := normalizeGithubCloneURL(c); clone != "" {
			return clone
		}
	}
	return ""
}

func githubCloneURLFromDeployment(opt MCPHubDeploymentOption) string {
	for _, key := range []string{"repositoryUrlToClone", "repository_url_to_clone", "repositoryUrl", "repoUrl", "git"} {
		if raw, ok := opt.InstallationDetail[key].(string); ok {
			if clone := normalizeGithubCloneURL(raw); clone != "" {
				return clone
			}
		}
	}
	for _, raw := range stringsFromAny(opt.InstallationDetail) {
		if clone := normalizeGithubCloneURL(raw); clone != "" {
			return clone
		}
	}
	return ""
}

func stringsFromAny(v interface{}) []string {
	out := []string{}
	var walk func(interface{})
	walk = func(x interface{}) {
		switch t := x.(type) {
		case string:
			out = append(out, t)
		case map[string]interface{}:
			for _, vv := range t {
				walk(vv)
			}
		case []interface{}:
			for _, vv := range t {
				walk(vv)
			}
		}
	}
	walk(v)
	return out
}

func normalizeGithubCloneURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || !strings.Contains(strings.ToLower(s), "github.com") {
		return ""
	}
	if strings.HasPrefix(s, "git@github.com:") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || !strings.Contains(strings.ToLower(u.Host), "github.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	repo := parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
	return "https://github.com/" + repo + ".git"
}

func uniqueMCPLabel(rows []MCPConfigRow, desired string) string {
	base := strings.TrimSpace(desired)
	if base == "" {
		base = "mcp-service"
	}
	labels := map[string]struct{}{}
	for _, row := range rows {
		label := strings.TrimSpace(row.Label)
		if label != "" {
			labels[label] = struct{}{}
		}
	}
	if _, ok := labels[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, ok := labels[next]; !ok {
			return next
		}
	}
}

func safePathSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "@", "-", "#", "-", " ", "-")
	s = replacer.Replace(s)
	s = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, ".-")
	if s == "" {
		return "mcp-service"
	}
	return s
}

func hasPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
