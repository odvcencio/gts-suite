package scope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoImportResolverStdlib(t *testing.T) {
	r := NewGoImportResolver("", "")
	path, err := r.Resolve("fmt")
	if err != nil {
		t.Fatalf("resolve fmt: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path for stdlib fmt")
	}
}

func TestGoImportResolverLocalModule(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "pkg", "util"), 0755)
	os.WriteFile(filepath.Join(dir, "pkg", "util", "util.go"), []byte("package util\n"), 0644)

	r := NewGoImportResolver(dir, "example.com/test")
	path, err := r.Resolve("example.com/test/pkg/util")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	expected := filepath.Join(dir, "pkg", "util")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}
