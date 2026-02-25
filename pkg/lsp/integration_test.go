package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationMultiFileGoProject(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Go project
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(
		"package main\n\nimport \"example.com/test/pkg\"\n\nfunc main() {\n\tpkg.Hello()\n}\n",
	), 0644)
	os.WriteFile(filepath.Join(dir, "pkg", "hello.go"), []byte(
		"package pkg\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n",
	), 0644)

	// Initialize
	input := lspRequest(1, "initialize", map[string]string{"rootUri": "file://" + dir})
	input += lspNotify("initialized", struct{}{})

	// Document symbols for main.go
	input += lspRequest(2, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": "file://" + filepath.Join(dir, "main.go")},
	})

	// Workspace symbol search
	input += lspRequest(3, "workspace/symbol", map[string]string{"query": "Hello"})

	// Hover on Hello in pkg/hello.go
	input += lspRequest(4, "textDocument/hover", map[string]any{
		"textDocument": map[string]string{"uri": "file://" + filepath.Join(dir, "pkg", "hello.go")},
		"position":     map[string]int{"line": 2, "character": 5},
	})

	input += lspRequest(5, "shutdown", nil)

	var out bytes.Buffer
	svc := NewService()
	srv := NewServer(strings.NewReader(input), &out, os.Stderr)
	svc.Register(srv)
	srv.Serve()

	resp := out.String()

	// Verify document symbols found "main"
	if !strings.Contains(resp, `"main"`) {
		t.Error("expected 'main' in document symbols")
	}

	// Verify workspace symbol found "Hello"
	if !strings.Contains(resp, `"Hello"`) {
		t.Error("expected 'Hello' in workspace symbols")
	}

	// Verify hover returns something useful
	if !strings.Contains(resp, `Hello`) {
		t.Error("expected hover content for Hello")
	}
}

func TestIntegrationDriverMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n"), 0644)

	req := DriverRequest{Mode: NeedName | NeedFiles | NeedImports}
	resp, err := HandleDriverRequest(dir, req, []string{"./..."})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}

	if len(resp.Packages) == 0 {
		t.Fatal("expected packages")
	}

	// Verify package metadata
	found := false
	for _, pkg := range resp.Packages {
		if pkg.Name == "main" {
			found = true
			if len(pkg.GoFiles) == 0 {
				t.Error("expected GoFiles")
			}
			if _, ok := pkg.Imports["fmt"]; !ok {
				t.Error("expected fmt in imports")
			}
		}
	}
	if !found {
		t.Error("expected main package")
	}
}
