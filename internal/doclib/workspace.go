package doclib

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WorkspaceDirName 文库根目录名（位于进程当前工作目录下）。
const WorkspaceDirName = "workspace"

// LibraryRootAbs 返回并确保文库根目录存在。
func LibraryRootAbs() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := filepath.Join(wd, WorkspaceDirName)
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("create library root: %w", err)
	}
	return filepath.Clean(root), nil
}

func normalizeLibraryRel(rel string) string {
	rel = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rel, "\\", string(filepath.Separator)), "/", string(filepath.Separator)))
	rel = filepath.Clean(rel)
	if rel == "." {
		return ""
	}
	return rel
}

// SafeLibraryAbs 将相对路径（正斜杠或系统分隔符）解析为根目录下的绝对路径，禁止 .. 与绝对路径逃逸。
func SafeLibraryAbs(root, rel string) (string, error) {
	root = filepath.Clean(root)
	rel = normalizeLibraryRel(rel)
	if rel == "" {
		return root, nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to library root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path")
	}
	abs := filepath.Join(root, rel)
	abs = filepath.Clean(abs)
	r, err := filepath.Rel(root, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes library root")
	}
	return abs, nil
}

// ListWorkspaceDir 列出文库内某一相对目录下的条目（目录优先，按名排序）。rel 可使用正斜杠。
// listedRel 为规范化后的当前相对路径（正斜杠），供面包屑使用。
func ListWorkspaceDir(rel string) (rootAbs string, listedRel string, parentRel string, entries []map[string]interface{}, err error) {
	rootAbs, err = LibraryRootAbs()
	if err != nil {
		return "", "", "", nil, err
	}
	rel = normalizeLibraryRel(rel)
	listedRel = filepath.ToSlash(rel)
	abs, err := SafeLibraryAbs(rootAbs, rel)
	if err != nil {
		return "", "", "", nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", "", "", nil, err
	}
	if !st.IsDir() {
		return "", "", "", nil, fmt.Errorf("not a directory: %s", rel)
	}
	parentRel = ""
	if rel != "" && rel != "." {
		d := filepath.Dir(rel)
		if d != "." && d != rel {
			parentRel = filepath.ToSlash(d)
		}
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return "", "", "", nil, err
	}
	type row struct {
		name  string
		isDir bool
		info  os.FileInfo
		rel   string
	}
	var rows []row
	for _, de := range des {
		name := de.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}
		var subRel string
		if rel != "" {
			subRel = filepath.ToSlash(filepath.Clean(filepath.Join(rel, name)))
		} else {
			subRel = filepath.ToSlash(filepath.Clean(name))
		}
		fi, er := de.Info()
		if er != nil {
			continue
		}
		rows = append(rows, row{name: name, isDir: de.IsDir(), info: fi, rel: subRel})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].isDir != rows[j].isDir {
			return rows[i].isDir
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})
	entries = make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		childAbs := filepath.Join(abs, r.name)
		entries = append(entries, map[string]interface{}{
			"name":    r.name,
			"relPath": r.rel,
			"absPath": filepath.Clean(childAbs),
			"isDir":   r.isDir,
			"modTime": r.info.ModTime().Format(time.RFC3339),
			"size":    r.info.Size(),
		})
	}
	return rootAbs, listedRel, parentRel, entries, nil
}

// WorkspaceMkdir 在文库内创建目录（含中间路径）。
func WorkspaceMkdir(rel string) (string, error) {
	root, err := LibraryRootAbs()
	if err != nil {
		return "", err
	}
	abs, err := SafeLibraryAbs(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	return abs, nil
}

// WorkspaceWriteFile 覆盖写入文本文件并登记到 doc_library。
func WorkspaceWriteFile(rel, content string) (string, error) {
	root, err := LibraryRootAbs()
	if err != nil {
		return "", err
	}
	abs, err := SafeLibraryAbs(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return "", err
	}
	Register(abs)
	return abs, nil
}

// WorkspaceDelete 删除文件，或删除空目录；recursive 为 true 时删除非空目录树。
func WorkspaceDelete(rel string, recursive bool) error {
	root, err := LibraryRootAbs()
	if err != nil {
		return err
	}
	abs, err := SafeLibraryAbs(root, rel)
	if err != nil {
		return err
	}
	if abs == root || abs == filepath.Clean(root) {
		return fmt.Errorf("cannot delete library root")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if !recursive {
			des, _ := os.ReadDir(abs)
			if len(des) > 0 {
				return fmt.Errorf("directory not empty (use recursive or empty it first)")
			}
		}
		if recursive {
			return os.RemoveAll(abs)
		}
		return os.Remove(abs)
	}
	return os.Remove(abs)
}

// WorkspaceRename 在文库根内重命名或移动（仅允许目标仍在根内）。
func WorkspaceRename(oldRel, newRel string) error {
	root, err := LibraryRootAbs()
	if err != nil {
		return err
	}
	oldAbs, err := SafeLibraryAbs(root, oldRel)
	if err != nil {
		return err
	}
	newAbs, err := SafeLibraryAbs(root, newRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newAbs), 0755); err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

// IsPathUnderLibrary 判断绝对路径是否在文库根下（用于 UI 校验）。
func IsPathUnderLibrary(abs string) bool {
	root, err := LibraryRootAbs()
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	r, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
