package appruntime

import (
	"os"
	"path/filepath"
)

const appSupportDirName = "leiAgent"

// RuntimeRoot returns the best runtime root for relative app paths.
// During local development we prefer the repository root.
// For installed apps we fall back to a user-writable app support directory.
func RuntimeRoot() string {
	if root := detectProjectRoot(); root != "" {
		return root
	}
	if root := appSupportRoot(); root != "" {
		return root
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// ResolvePath resolves a relative path against the runtime root.
func ResolvePath(path string) string {
	if path == "" {
		return RuntimeRoot()
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(RuntimeRoot(), filepath.Clean(path))
}

// BootstrapWorkingDirectory moves the process working directory to the runtime root
// so the rest of the app can keep using existing relative paths.
func BootstrapWorkingDirectory() (string, error) {
	root := RuntimeRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return root, err
	}
	if err := os.Chdir(root); err != nil {
		return root, err
	}
	return root, nil
}

func detectProjectRoot() string {
	if wd, err := os.Getwd(); err == nil {
		// When running `go test` the working directory is often a package subdir.
		// Walk upwards to locate repo root.
		dir := wd
		for i := 0; i < 8; i++ {
			if looksLikeProjectRoot(dir) {
				return dir
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if realExe, err := filepath.EvalSymlinks(exe); err == nil {
		exe = realExe
	}

	dir := filepath.Dir(exe)
	for i := 0; i < 8; i++ {
		if looksLikeProjectRoot(dir) {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return ""
}

func looksLikeProjectRoot(dir string) bool {
	if dir == "" {
		return false
	}
	if !hasPath(filepath.Join(dir, "go.mod")) {
		return false
	}
	if !hasPath(filepath.Join(dir, "wails.json")) {
		return false
	}
	return true
}

func hasPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appSupportRoot() string {
	base, err := os.UserConfigDir()
	if err == nil && base != "" {
		return filepath.Join(base, appSupportDirName)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, "."+appSupportDirName)
	}
	return ""
}
