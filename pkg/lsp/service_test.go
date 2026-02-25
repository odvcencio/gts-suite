package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lspRequest builds a Content-Length framed LSP request.
func lspRequest(id int, method string, params any) string {
	p, _ := json.Marshal(params)
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}`, id, method, p)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

// lspNotify builds a Content-Length framed LSP notification.
func lspNotify(method string, params any) string {
	p, _ := json.Marshal(params)
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"%s","params":%s}`, method, p)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func TestServiceInitialize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	input := lspRequest(1, "initialize", map[string]string{
		"rootUri": "file://" + dir,
	})
	input += lspRequest(2, "shutdown", nil)

	var out bytes.Buffer
	svc := NewService()
	srv := NewServer(strings.NewReader(input), &out, os.Stderr)
	svc.Register(srv)
	srv.Serve()

	resp := out.String()
	if !strings.Contains(resp, `"documentSymbolProvider":true`) {
		t.Errorf("expected documentSymbolProvider capability, got: %s", resp)
	}
	if !strings.Contains(resp, `"gtsls"`) {
		t.Errorf("expected server name gtsls, got: %s", resp)
	}
}

func TestServiceDocumentSymbols(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\n\nfunc hello() {}\n\nfunc world() {}\n"), 0644)

	input := lspRequest(1, "initialize", map[string]string{
		"rootUri": "file://" + dir,
	})
	input += lspNotify("initialized", struct{}{})
	input += lspRequest(2, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{
			"uri": "file://" + goFile,
		},
	})
	input += lspRequest(3, "shutdown", nil)

	var out bytes.Buffer
	svc := NewService()
	srv := NewServer(strings.NewReader(input), &out, os.Stderr)
	svc.Register(srv)
	srv.Serve()

	resp := out.String()
	if !strings.Contains(resp, `"hello"`) {
		t.Errorf("expected symbol 'hello' in response, got: %s", resp)
	}
	if !strings.Contains(resp, `"world"`) {
		t.Errorf("expected symbol 'world' in response, got: %s", resp)
	}
}
