package index

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuildPath_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	source := `package sample

func TestMain() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatalf("WriteFile README.md failed: %v", err)
	}

	builder := NewBuilder()
	idx, err := builder.BuildPath(tmpDir)
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}

	if idx.FileCount() != 1 {
		t.Fatalf("expected 1 indexed file, got %d", idx.FileCount())
	}
	if idx.SymbolCount() != 1 {
		t.Fatalf("expected 1 symbol, got %d", idx.SymbolCount())
	}
	if idx.Files[0].Path != "main.go" {
		t.Fatalf("expected relative path main.go, got %q", idx.Files[0].Path)
	}
}

func TestBuildPathIncremental_ReusesUnchangedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s failed: %v", name, err)
		}
	}

	write("a.go", "package sample\n\nfunc A() {}\n")
	write("b.go", "package sample\n\nfunc B() {}\n")

	builder := NewBuilder()
	first, firstStats, err := builder.BuildPathIncremental(tmpDir, nil)
	if err != nil {
		t.Fatalf("BuildPathIncremental first returned error: %v", err)
	}
	if firstStats.ParsedFiles != 2 || firstStats.ReusedFiles != 0 {
		t.Fatalf("unexpected first stats: %+v", firstStats)
	}

	second, secondStats, err := builder.BuildPathIncremental(tmpDir, first)
	if err != nil {
		t.Fatalf("BuildPathIncremental second returned error: %v", err)
	}
	if secondStats.ParsedFiles != 0 || secondStats.ReusedFiles != 2 {
		t.Fatalf("unexpected second stats: %+v", secondStats)
	}
	if second.FileCount() != 2 {
		t.Fatalf("expected 2 files, got %d", second.FileCount())
	}
}

func TestBuildPathIncremental_ParsesOnlyChangedFile(t *testing.T) {
	tmpDir := t.TempDir()
	aPath := filepath.Join(tmpDir, "a.go")
	bPath := filepath.Join(tmpDir, "b.go")

	if err := os.WriteFile(aPath, []byte("package sample\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile a.go failed: %v", err)
	}
	if err := os.WriteFile(bPath, []byte("package sample\n\nfunc B() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile b.go failed: %v", err)
	}

	builder := NewBuilder()
	first, _, err := builder.BuildPathIncremental(tmpDir, nil)
	if err != nil {
		t.Fatalf("BuildPathIncremental first returned error: %v", err)
	}

	// Ensure mtime ticks forward so reuse check observes the update.
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(aPath, []byte("package sample\n\nfunc A() {}\nfunc A2() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile updated a.go failed: %v", err)
	}

	second, secondStats, err := builder.BuildPathIncremental(tmpDir, first)
	if err != nil {
		t.Fatalf("BuildPathIncremental second returned error: %v", err)
	}
	if secondStats.ParsedFiles != 1 || secondStats.ReusedFiles != 1 {
		t.Fatalf("unexpected second stats: %+v", secondStats)
	}
	if second.SymbolCount() <= first.SymbolCount() {
		t.Fatalf("expected symbol count to increase after change, first=%d second=%d", first.SymbolCount(), second.SymbolCount())
	}
}

func TestBuildPathIncremental_RemovedFileDropsFromIndex(t *testing.T) {
	tmpDir := t.TempDir()
	aPath := filepath.Join(tmpDir, "a.go")
	bPath := filepath.Join(tmpDir, "b.go")

	if err := os.WriteFile(aPath, []byte("package sample\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile a.go failed: %v", err)
	}
	if err := os.WriteFile(bPath, []byte("package sample\n\nfunc B() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile b.go failed: %v", err)
	}

	builder := NewBuilder()
	first, _, err := builder.BuildPathIncremental(tmpDir, nil)
	if err != nil {
		t.Fatalf("BuildPathIncremental first returned error: %v", err)
	}
	if first.FileCount() != 2 {
		t.Fatalf("expected 2 files in first index, got %d", first.FileCount())
	}

	if err := os.Remove(bPath); err != nil {
		t.Fatalf("Remove b.go failed: %v", err)
	}

	second, secondStats, err := builder.BuildPathIncremental(tmpDir, first)
	if err != nil {
		t.Fatalf("BuildPathIncremental second returned error: %v", err)
	}
	if second.FileCount() != 1 {
		t.Fatalf("expected 1 file after removal, got %d", second.FileCount())
	}
	if secondStats.ReusedFiles != 1 || secondStats.ParsedFiles != 0 {
		t.Fatalf("unexpected stats after removal: %+v", secondStats)
	}
}

func TestBuildPath_DirectoryOrderStable(t *testing.T) {
	tmpDir := t.TempDir()

	files := []string{"zeta.go", "alpha.go", "mid.go"}
	for i, name := range files {
		source := fmt.Sprintf(`package sample

func F%d() {}
`, i)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(source), 0o644); err != nil {
			t.Fatalf("WriteFile %s failed: %v", name, err)
		}
	}

	builder := NewBuilder()
	idx, err := builder.BuildPath(tmpDir)
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}

	if len(idx.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(idx.Files))
	}
	got := []string{idx.Files[0].Path, idx.Files[1].Path, idx.Files[2].Path}
	want := []string{"alpha.go", "mid.go", "zeta.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected file order got=%v want=%v", got, want)
	}
}

func TestIndexWorkerCount(t *testing.T) {
	t.Setenv("GTS_INDEX_WORKERS", "2")
	if got := indexWorkerCount(10); got != 2 {
		t.Fatalf("expected 2 workers, got %d", got)
	}

	t.Setenv("GTS_INDEX_WORKERS", "999")
	if got := indexWorkerCount(3); got != 3 {
		t.Fatalf("expected worker count capped at 3, got %d", got)
	}

	t.Setenv("GTS_INDEX_WORKERS", "invalid")
	if got := indexWorkerCount(0); got != 0 {
		t.Fatalf("expected 0 workers for empty task set, got %d", got)
	}
	if got := indexWorkerCount(1); got < 1 {
		t.Fatalf("expected at least 1 worker, got %d", got)
	}
}

func BenchmarkBuildPath_Directory(b *testing.B) {
	tmpDir := b.TempDir()

	for i := 0; i < 300; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("f%03d.go", i))
		source := fmt.Sprintf(`package sample

type Type%03d struct{}

func Func%03d() int { return %d }
`, i, i, i)
		if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
			b.Fatalf("WriteFile failed: %v", err)
		}
	}

	builder := NewBuilder()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, err := builder.BuildPath(tmpDir)
		if err != nil {
			b.Fatalf("BuildPath returned error: %v", err)
		}
		if idx.FileCount() != 300 {
			b.Fatalf("expected 300 files, got %d", idx.FileCount())
		}
	}
}

func BenchmarkBuildPathIncremental_Warm(b *testing.B) {
	tmpDir := b.TempDir()

	for i := 0; i < 300; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("f%03d.go", i))
		source := fmt.Sprintf(`package sample

type Type%03d struct{}

func Func%03d() int { return %d }
`, i, i, i)
		if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
			b.Fatalf("WriteFile failed: %v", err)
		}
	}

	builder := NewBuilder()
	base, _, err := builder.BuildPathIncremental(tmpDir, nil)
	if err != nil {
		b.Fatalf("initial BuildPathIncremental returned error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		next, _, err := builder.BuildPathIncremental(tmpDir, base)
		if err != nil {
			b.Fatalf("BuildPathIncremental returned error: %v", err)
		}
		if next.FileCount() != 300 {
			b.Fatalf("expected 300 files, got %d", next.FileCount())
		}
		base = next
	}
}
