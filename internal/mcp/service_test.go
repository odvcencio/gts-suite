package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"gts-suite/internal/chunk"
	"gts-suite/internal/contextpack"
	"gts-suite/internal/deps"
	"gts-suite/internal/xref"
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
	for _, name := range []string{"gts_query", "gts_refs", "gts_context", "gts_scope", "gts_deps", "gts_callgraph", "gts_dead", "gts_chunk", "gts_lint"} {
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

func TestServiceCallgraphAndDead(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func Used() {}
func Dead() {}

func main() {
	Used()
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	service := NewService(tmpDir, "")
	callgraphRaw, err := service.Call("gts_callgraph", map[string]any{
		"name":    "main",
		"depth":   2,
		"reverse": false,
	})
	if err != nil {
		t.Fatalf("gts_callgraph call failed: %v", err)
	}
	callgraph, ok := callgraphRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected callgraph map result, got %T", callgraphRaw)
	}
	edges, ok := callgraph["edges"].([]xref.Edge)
	if !ok {
		t.Fatalf("expected callgraph edges slice, got %T", callgraph["edges"])
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 callgraph edge, got %d", len(edges))
	}

	deadRaw, err := service.Call("gts_dead", map[string]any{
		"kind": "function",
	})
	if err != nil {
		t.Fatalf("gts_dead call failed: %v", err)
	}
	dead, ok := deadRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected dead map result, got %T", deadRaw)
	}
	count, ok := dead["count"].(int)
	if !ok {
		t.Fatalf("expected dead count int, got %T", dead["count"])
	}
	if count != 1 {
		t.Fatalf("expected dead count=1, got %d", count)
	}
}

func TestServiceChunkAndLint(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	patternPath := filepath.Join(tmpDir, "empty.scm")
	source := `package sample

func Empty() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile source failed: %v", err)
	}
	if err := os.WriteFile(patternPath, []byte(`(function_declaration (block) @violation)`), 0o644); err != nil {
		t.Fatalf("WriteFile pattern failed: %v", err)
	}

	service := NewService(tmpDir, "")

	chunkRaw, err := service.Call("gts_chunk", map[string]any{
		"tokens": 200,
	})
	if err != nil {
		t.Fatalf("gts_chunk call failed: %v", err)
	}
	chunkReport, ok := chunkRaw.(chunk.Report)
	if !ok {
		t.Fatalf("expected chunk.Report, got %T", chunkRaw)
	}
	if chunkReport.ChunkCount == 0 {
		t.Fatalf("expected non-zero chunks")
	}

	lintRaw, err := service.Call("gts_lint", map[string]any{
		"pattern": patternPath,
	})
	if err != nil {
		t.Fatalf("gts_lint call failed: %v", err)
	}
	lintResult, ok := lintRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected lint map result, got %T", lintRaw)
	}
	count, ok := lintResult["count"].(int)
	if !ok {
		t.Fatalf("expected lint count int, got %T", lintResult["count"])
	}
	if count == 0 {
		t.Fatalf("expected lint violations > 0")
	}
}
