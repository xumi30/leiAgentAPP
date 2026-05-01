package filefunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/doclib"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// FileWriteTool implements the Tool interface for writing content to files
type FileWriteTool struct{}

var (
	fileWriteToolInstance *FileWriteTool
	fileWriteToolOnce     sync.Once
)

// GetFileWriteTool returns a singleton instance of FileWriteTool
func GetFileWriteTool() tools.Tool {
	fileWriteToolOnce.Do(func() {
		fileWriteToolInstance = &FileWriteTool{}
	})
	return fileWriteToolInstance
}

// Name returns the name of the tool
func (t *FileWriteTool) Name() string {
	return "file_write"
}

// Description returns the description of what the tool does
func (t *FileWriteTool) Description() string {
	return `
	Write content to a file with automatic structure design.
	MUST ensure the content is structured and readable.
	MUST ensure Args is json formatted.
	
	
	You MUST NOT write raw or unstructured text.
	
	Before writing:
	1. Understand the purpose of the content
	2. Design a clear structure
	3. Format using Markdown
	
	You are responsible for:
	- Content organization
	- Readability
	- Proper formatting
	
	Bad example (DO NOT DO):
	- Plain paragraphs without structure
	
	Good example:
	- Clear headings
	- Bullet points
	- Logical sections
	
	If the user gives rough ideas, you MUST refine and structure them.
	
	Args:
	{
	"path": "file path",
	  "content": "structured markdown content"
	}

	Security boundary:
	- The target path must stay inside the app workspace directory.
	`
}

// Parameters returns the parameters that the tool accepts
func (t *FileWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The target file path inside the workspace. Relative paths are resolved under workspace/. Absolute paths must also point inside workspace/.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file. This can be any text content.",
			},
		},
		"required": []string{"path", "content"},
	}
}

// Run executes the tool with the given input
func (t *FileWriteTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments
func (t *FileWriteTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}

	// 首先尝试解析 JSON 格式的参数
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		logging.Info("JSON 解析失败，尝试使用字符匹配提取参数: %v , args: %s", err, args)

		// 如果 JSON 解析失败，尝试使用字符匹配提取参数
		params = make(map[string]interface{})

		// 尝试提取 path 参数
		pathMatch := regexp.MustCompile(`"path"\s*:\s*"([^"]*)"`)
		pathMatches := pathMatch.FindStringSubmatch(args)
		if len(pathMatches) > 1 {
			params["path"] = pathMatches[1]
		}

		// 尝试提取 content 参数
		// 使用更灵活的正则表达式，匹配 content 字段后的内容
		contentMatch := regexp.MustCompile(`"content"\s*:\s*"((?:[^"\\]|\\.)*)"`)
		contentMatches := contentMatch.FindStringSubmatch(args)
		if len(contentMatches) > 1 {
			// 处理转义字符
			content := strings.ReplaceAll(contentMatches[1], `\"`, `"`)
			content = strings.ReplaceAll(content, `\\`, `\`)
			content = strings.ReplaceAll(content, `\n`, "\n")
			content = strings.ReplaceAll(content, `\t`, "\t")
			params["content"] = content
		}

		// 如果仍然无法提取到必要的参数，返回错误
		if _, ok := params["path"]; !ok {
			return "无法提取 path 参数", fmt.Errorf("无法提取 path 参数")
		}
		if _, ok := params["content"]; !ok {
			return "无法提取 content 参数", fmt.Errorf("无法提取 content 参数")
		}
	}

	// Get path parameter
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "path parameter is required", fmt.Errorf("path parameter is required")
	}

	// Get content parameter
	content, ok := params["content"].(string)
	if !ok {
		return "content parameter is required", fmt.Errorf("content parameter is required")
	}

	// Normalize path separators for the current OS
	if runtime.GOOS == "windows" {
		// Replace forward slashes with backslashes on Windows
		path = strings.ReplaceAll(path, "/", "\\")
	} else {
		// Replace backslashes with forward slashes on Unix-like systems
		path = strings.ReplaceAll(path, "\\", "/")
	}

	root, err := doclib.LibraryRootAbs()
	if err != nil {
		return "failed to resolve workspace root", fmt.Errorf("failed to resolve workspace root: %v", err)
	}
	path, err = resolveFileWriteWorkspacePath(root, path)
	if err != nil {
		return "path must stay inside workspace", err
	}

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "failed to create directory", fmt.Errorf("failed to create directory: %v", err)
	}

	// Write the content to the file
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "failed to open file", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return "failed to write to file", fmt.Errorf("failed to write to file: %v", err)
	}

	doclib.Register(path)
	out, _ := json.MarshalIndent(map[string]interface{}{
		"message": fmt.Sprintf("Successfully wrote content to file: %s", path),
		"path":    path,
	}, "", "  ")
	return string(out), nil
}

func resolveFileWriteWorkspacePath(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path parameter is required")
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

func (t *FileWriteTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Result of file write operation.",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable result message.",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute file path (if available in message).",
			},
		},
	}

}

func (t *FileWriteTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicFiles, "将结构化 Markdown 等内容追加写入指定路径的本地文件。")
}
