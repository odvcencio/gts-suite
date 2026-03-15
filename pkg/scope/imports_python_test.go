package scope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePythonImportLocal(t *testing.T) {
	dir := t.TempDir()
	// Create a package: mylib/__init__.py
	os.MkdirAll(filepath.Join(dir, "mylib"), 0755)
	os.WriteFile(filepath.Join(dir, "mylib", "__init__.py"), []byte(""), 0644)

	result := ResolvePythonImport("mylib", dir)
	if result == "" {
		t.Error("expected to resolve mylib")
	}
}

func TestResolvePythonImportFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "utils.py"), []byte("def foo(): pass"), 0644)

	result := ResolvePythonImport("utils", dir)
	if result == "" {
		t.Error("expected to resolve utils.py")
	}
}

func TestResolvePythonImportNotFound(t *testing.T) {
	result := ResolvePythonImport("nonexistent", t.TempDir())
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
