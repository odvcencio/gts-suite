package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDriverResponse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
	), 0644)

	req := DriverRequest{
		Mode: NeedName | NeedFiles | NeedImports,
	}
	patterns := []string{"./..."}

	resp, err := HandleDriverRequest(dir, req, patterns)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}

	if len(resp.Packages) == 0 {
		t.Fatal("expected at least one package")
	}
	pkg := resp.Packages[0]
	if pkg.Name != "main" {
		t.Errorf("expected package name 'main', got %q", pkg.Name)
	}
	if len(pkg.GoFiles) == 0 {
		t.Error("expected at least one GoFile")
	}

	// Should have "fmt" in imports
	if _, ok := pkg.Imports["fmt"]; !ok {
		t.Error("expected 'fmt' in imports")
	}
}
