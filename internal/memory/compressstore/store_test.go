package compressstore

import (
	"os"
	"testing"
)

type testObj struct {
	A string `yaml:"a"`
	B int    `yaml:"b"`
}

func TestPersistAndLoadYAML(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	in := testObj{A: "x", B: 2}
	if _, err := PersistYAML("compress", "c1", &in); err != nil {
		t.Fatalf("persist: %v", err)
	}
	var out testObj
	ok, _, err := LoadYAML("compress", "c1", &out)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if out != in {
		t.Fatalf("expected %#v got %#v", in, out)
	}
}
