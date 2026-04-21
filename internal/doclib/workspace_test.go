package doclib

import (
	"path/filepath"
	"testing"
)

func TestSafeLibraryAbsRejectsEscapes(t *testing.T) {
	root := filepath.Clean(t.TempDir())

	for _, rel := range []string{"..", "../secret.txt", "/tmp/secret.txt"} {
		if _, err := SafeLibraryAbs(root, rel); err == nil {
			t.Fatalf("expected %q to be rejected", rel)
		}
	}
}

func TestSafeLibraryAbsAcceptsNestedRelativePath(t *testing.T) {
	root := filepath.Clean(t.TempDir())

	got, err := SafeLibraryAbs(root, "stories/chapter-01.md")
	if err != nil {
		t.Fatalf("SafeLibraryAbs returned error: %v", err)
	}

	want := filepath.Join(root, "stories", "chapter-01.md")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
