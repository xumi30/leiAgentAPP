package libraryfs

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/doclib"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"strings"
)

// Tool 让 Agent 在应用「文库」根目录（工作目录下 workspace/）内做目录与文件操作，适合连载、项目分层等。
type Tool struct{}

func New() tools.Tool { return &Tool{} }

func (t *Tool) Name() string { return "library_fs" }

func (t *Tool) Description() string {
	return `Manage files and folders inside the app's document library root (a folder named "workspace" under the app working directory). All paths are relative to that root — use forward slashes (e.g. "小说/三侠五义/第一回.md"). Safe: cannot escape outside the library.

Use for: creating novel/project folders, chapter files, notes trees. Combine with operation write_file for content. Prefer mkdir before write_file when the parent folder does not exist.

Operations:
- list_dir: list entries in "path" (default "").
- mkdir: create directory "path" (and parents).
- write_file: overwrite file at "path" with "content" (UTF-8 text).
- delete: remove file or empty directory; set recursive_delete true to remove a non-empty directory tree.
- rename: move/rename from "path" to "path_to".

Example: user wants 小说/三侠五义 with chapters — mkdir "小说", mkdir "小说/三侠五义", write_file "小说/三侠五义/第一回.md" with content, etc.`
}

func (t *Tool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list_dir", "mkdir", "write_file", "delete", "rename"},
				"description": "Action to perform.",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path inside the library (e.g. 小说/三侠五义). Use \"\" for list_dir root.",
			},
			"path_to": map[string]interface{}{
				"type":        "string",
				"description": "For rename: new relative path inside the library.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "For write_file: full file content (overwrite).",
			},
			"recursive_delete": map[string]interface{}{
				"type":        "boolean",
				"description": "For delete: if true, remove directories with all contents. Default false.",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	op, _ := params["operation"].(string)
	op = strings.TrimSpace(strings.ToLower(op))
	path, _ := params["path"].(string)
	pathTo, _ := params["path_to"].(string)
	content, _ := params["content"].(string)
	recursive := false
	if v, ok := params["recursive_delete"].(bool); ok {
		recursive = v
	}

	switch op {
	case "list_dir":
		root, listed, parent, entries, err := doclib.ListWorkspaceDir(path)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"operation": "list_dir",
			"message":   "",
			"data": map[string]interface{}{
				"library_root": root,
				"listed_rel":   listed,
				"parent_rel":   parent,
				"entries":      entries,
			},
		}, "", "  ")
		return string(b), nil
	case "mkdir":
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("path required for mkdir")
		}
		abs, err := doclib.WorkspaceMkdir(path)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"operation": "mkdir",
			"message":   fmt.Sprintf("Created directory: %s", abs),
			"data": map[string]interface{}{
				"path":     path,
				"abs_path": abs,
			},
		}, "", "  ")
		return string(b), nil
	case "write_file":
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("path required for write_file")
		}
		abs, err := doclib.WorkspaceWriteFile(path, content)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"operation": "write_file",
			"message":   fmt.Sprintf("Wrote file: %s", abs),
			"data": map[string]interface{}{
				"path":     path,
				"abs_path": abs,
			},
		}, "", "  ")
		return string(b), nil
	case "delete":
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("path required for delete")
		}
		if err := doclib.WorkspaceDelete(path, recursive); err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"operation": "delete",
			"message":   fmt.Sprintf("Deleted: %s", path),
			"data": map[string]interface{}{
				"path":             path,
				"recursive_delete": recursive,
			},
		}, "", "  ")
		return string(b), nil
	case "rename":
		if strings.TrimSpace(path) == "" || strings.TrimSpace(pathTo) == "" {
			return "", fmt.Errorf("path and path_to required for rename")
		}
		if err := doclib.WorkspaceRename(path, pathTo); err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"operation": "rename",
			"message":   fmt.Sprintf("Renamed %s -> %s", path, pathTo),
			"data": map[string]interface{}{
				"path":    path,
				"path_to": pathTo,
			},
		}, "", "  ")
		return string(b), nil
	default:
		return "", fmt.Errorf("unknown operation: %s", op)
	}
}

func (t *Tool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Result of library file system operation (may include structured JSON for list_dir).",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable result message (non-list operations).",
			},
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Operation executed.",
			},
			"data": map[string]interface{}{
				"type":        "object",
				"description": "Structured data payload (e.g. list_dir results).",
			},
		},
	}
}

func (t *Tool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicFiles, "在应用 workspace 文库根目录内安全地列出、创建、读写、删除或重命名文件与文件夹。")
}
