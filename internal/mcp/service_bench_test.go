package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

// setupBenchService creates a temp directory with a small Go source file and
// returns a Service rooted at that directory. The cleanup is handled by
// testing.B.TempDir.
func setupBenchService(b *testing.B) *Service {
	b.Helper()
	tmpDir := b.TempDir()
	source := `package sample

func Alpha() {}
func Beta()  { Alpha() }
func Gamma() { Beta() }
func Delta() { Gamma(); Alpha() }
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(source), 0o644); err != nil {
		b.Fatalf("WriteFile: %v", err)
	}
	return NewService(tmpDir, "")
}

func BenchmarkCallGrep(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{
		"selector": "function_definition[name=/^A/]",
	}
	// Warm the index by running one call before timing.
	if _, err := svc.Call("gts_grep", args); err != nil {
		b.Fatalf("warmup gts_grep: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_grep", args)
	}
}

func BenchmarkCallMap(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{}
	if _, err := svc.Call("gts_map", args); err != nil {
		b.Fatalf("warmup gts_map: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_map", args)
	}
}

func BenchmarkCallRefs(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{
		"name": "Alpha",
	}
	if _, err := svc.Call("gts_refs", args); err != nil {
		b.Fatalf("warmup gts_refs: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_refs", args)
	}
}

func BenchmarkCallStats(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{}
	if _, err := svc.Call("gts_stats", args); err != nil {
		b.Fatalf("warmup gts_stats: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_stats", args)
	}
}

func BenchmarkCallFiles(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{
		"sort": "symbols",
		"top":  10,
	}
	if _, err := svc.Call("gts_files", args); err != nil {
		b.Fatalf("warmup gts_files: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_files", args)
	}
}

func BenchmarkCallDead(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{}
	if _, err := svc.Call("gts_dead", args); err != nil {
		b.Fatalf("warmup gts_dead: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_dead", args)
	}
}

func BenchmarkCallCallgraph(b *testing.B) {
	svc := setupBenchService(b)
	args := map[string]any{
		"name":  "Delta",
		"depth": 3,
	}
	if _, err := svc.Call("gts_callgraph", args); err != nil {
		b.Fatalf("warmup gts_callgraph: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = svc.Call("gts_callgraph", args)
	}
}
