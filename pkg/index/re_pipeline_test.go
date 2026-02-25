package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPath_LLVMIR(t *testing.T) {
	tmpDir := t.TempDir()
	// DFA parser handles single top-level function definitions cleanly.
	llvmSrc := []byte(`define i32 @entry() {
  call void @worker()
  ret i32 0
}
`)
	if err := os.WriteFile(filepath.Join(tmpDir, "sample.ll"), llvmSrc, 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder()
	idx, err := builder.BuildPath(tmpDir)
	if err != nil {
		t.Fatalf("BuildPath: %v", err)
	}

	if len(idx.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(idx.Files))
	}

	f := idx.Files[0]
	if f.Language != "llvm" {
		t.Fatalf("expected language llvm, got %q", f.Language)
	}

	foundEntry := false
	foundWorkerRef := false
	for _, sym := range f.Symbols {
		if sym.Kind == "function_definition" && sym.Name == "@entry" {
			foundEntry = true
		}
	}
	for _, ref := range f.References {
		if ref.Kind == "reference.call" && ref.Name == "@worker" {
			foundWorkerRef = true
		}
	}
	if !foundEntry {
		t.Fatal("expected function_definition @entry in index")
	}
	if !foundWorkerRef {
		t.Fatal("expected reference.call @worker in index")
	}
}

func TestBuildPath_Disassembly(t *testing.T) {
	tmpDir := t.TempDir()
	disSrc := []byte(`0000000000001000 <main>:
    1000: 55                   push   rbp
    1001: 48 89 e5             mov    rbp,rsp
`)
	if err := os.WriteFile(filepath.Join(tmpDir, "dump.dis"), disSrc, 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder()
	idx, err := builder.BuildPath(tmpDir)
	if err != nil {
		t.Fatalf("BuildPath: %v", err)
	}

	if len(idx.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(idx.Files))
	}

	f := idx.Files[0]
	if f.Language != "disassembly" {
		t.Fatalf("expected language disassembly, got %q", f.Language)
	}

	foundMain := false
	for _, sym := range f.Symbols {
		if sym.Kind == "function_definition" && sym.Name == "main" {
			foundMain = true
		}
	}
	if !foundMain {
		t.Fatal("expected function_definition main in index")
	}
}
