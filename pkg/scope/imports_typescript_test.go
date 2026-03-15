package scope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTSRelativeImport(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "utils.ts"), []byte("export function foo() {}"), 0644)

	result := ResolveTypeScriptImport("./utils", filepath.Join(dir, "main.ts"), dir)
	if result == "" {
		t.Error("expected to resolve ./utils")
	}
}

func TestResolveTSNodeModules(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "node_modules", "express")
	os.MkdirAll(modDir, 0755)
	os.WriteFile(filepath.Join(modDir, "index.d.ts"), []byte("declare module 'express' {}"), 0644)

	result := ResolveTypeScriptImport("express", filepath.Join(dir, "src", "app.ts"), dir)
	if result == "" {
		t.Error("expected to resolve express from node_modules")
	}
}

func TestResolveTSTypes(t *testing.T) {
	dir := t.TempDir()
	typesDir := filepath.Join(dir, "node_modules", "@types", "lodash")
	os.MkdirAll(typesDir, 0755)
	os.WriteFile(filepath.Join(typesDir, "index.d.ts"), []byte("declare module 'lodash' {}"), 0644)

	result := ResolveTypeScriptImport("lodash", filepath.Join(dir, "src", "app.ts"), dir)
	if result == "" {
		t.Error("expected to resolve lodash from @types")
	}
}

func TestResolveTSNotFound(t *testing.T) {
	result := ResolveTypeScriptImport("nonexistent", "/tmp/main.ts", t.TempDir())
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
