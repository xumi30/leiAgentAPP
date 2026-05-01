package filefunctions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWriteRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	outsidePath := filepath.Join(tmp, "outside.md")
	args, _ := json.Marshal(map[string]string{
		"path":    outsidePath,
		"content": "# outside\n",
	})

	_, err = (&FileWriteTool{}).Execute(context.Background(), string(args))
	if err == nil {
		t.Fatal("expected file_write to reject absolute path outside workspace")
	}
	if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
		t.Fatalf("outside file should not exist, statErr=%v", statErr)
	}
}

func TestFileWriteAllowsRelativePathInsideWorkspace(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	args, _ := json.Marshal(map[string]string{
		"path":    "notes/today.md",
		"content": "# Today\n",
	})

	out, err := (&FileWriteTool{}).Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("file_write returned error: %v", err)
	}
	if !strings.Contains(out, filepath.Join("workspace", "notes", "today.md")) {
		t.Fatalf("expected output path under workspace, got %s", out)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "workspace", "notes", "today.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Today\n" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}
