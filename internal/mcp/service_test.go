package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"gts-suite/internal/contextpack"
	"gts-suite/internal/deps"
)

func TestServiceToolsIncludesCoreRoadmapTools(t *testing.T) {
	service := NewService(".", "")
	tools := service.Tools()
	if len(tools) < 5 {
		t.Fatalf("expected at least 5 tools, got %d", len(tools))
	}

	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"gts_query", "gts_refs", "gts_context", "gts_scope", "gts_deps"} {
		if !seen[name] {
			t.Fatalf("expected tool %q to be present", name)
		}
	}
}

func TestServiceCallRefsAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func A() {}

func B() {
	A()
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	service := NewService(tmpDir, "")
	refsResultRaw, err := service.Call("gts_refs", map[string]any{
		"name": "A",
	})
	if err != nil {
		t.Fatalf("gts_refs call failed: %v", err)
	}
	refsResult, ok := refsResultRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected refs result map, got %T", refsResultRaw)
	}
	if refsResult["count"].(int) != 1 {
		t.Fatalf("expected refs count=1, got %#v", refsResult["count"])
	}

	queryResultRaw, err := service.Call("gts_query", map[string]any{
		"pattern": "(function_declaration (identifier) @name)",
	})
	if err != nil {
		t.Fatalf("gts_query call failed: %v", err)
	}
	queryResult, ok := queryResultRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected query result map, got %T", queryResultRaw)
	}
	if queryResult["count"].(int) != 2 {
		t.Fatalf("expected query count=2, got %#v", queryResult["count"])
	}
}

func TestServiceCallContextAndDeps(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

import "fmt"

func helper() {
	fmt.Println("ok")
}

func work() {
	helper()
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module sample\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod failed: %v", err)
	}

	service := NewService(tmpDir, "")
	contextResultRaw, err := service.Call("gts_context", map[string]any{
		"file":           sourcePath,
		"line":           10,
		"semantic":       true,
		"semantic_depth": 2,
	})
	if err != nil {
		t.Fatalf("gts_context call failed: %v", err)
	}
	contextResult, ok := contextResultRaw.(contextpack.Report)
	if ok {
		if contextResult.File != "main.go" {
			t.Fatalf("expected context file main.go, got %q", contextResult.File)
		}
		if !contextResult.Semantic || contextResult.SemanticDepth != 2 {
			t.Fatalf("expected semantic context depth=2, got semantic=%t depth=%d", contextResult.Semantic, contextResult.SemanticDepth)
		}
		if len(contextResult.Related) == 0 {
			t.Fatalf("expected semantic related symbols, got none")
		}
	} else {
		t.Fatalf("expected contextpack.Report, got %T", contextResultRaw)
	}

	depsResultRaw, err := service.Call("gts_deps", map[string]any{
		"by": "package",
	})
	if err != nil {
		t.Fatalf("gts_deps call failed: %v", err)
	}
	depsResult, ok := depsResultRaw.(deps.Report)
	if !ok {
		t.Fatalf("expected deps.Report, got %T", depsResultRaw)
	}
	if depsResult.Mode != "package" {
		t.Fatalf("expected deps mode package, got %q", depsResult.Mode)
	}
}
