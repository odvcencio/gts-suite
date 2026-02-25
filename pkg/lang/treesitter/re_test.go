package treesitter

import (
	"testing"
)

func TestLLVMIREntityExtraction(t *testing.T) {
	entry := findEntryByExtension(t, ".ll")
	parser, err := NewParser(entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	// DFA parser handles single top-level definitions well.
	// Multi-function files degrade after the first function body.
	const source = `define i32 @main() {
  call void @puts()
  ret i32 0
}
`

	summary, err := parser.Parse("sample.ll", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "llvm" {
		t.Fatalf("expected language llvm, got %q", summary.Language)
	}

	if !hasSymbol(summary, "function_definition", "@main") {
		t.Fatal("expected function_definition @main")
	}
	if !hasReference(summary, "reference.call", "@puts") {
		t.Fatal("expected reference.call @puts")
	}
}

func TestASMEntityExtraction(t *testing.T) {
	entry := findEntryByExtension(t, ".s")
	parser, err := NewParser(entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	// ASM labels are extracted as top-level identifiers.
	// The DFA produces a degraded tree but ident nodes are still matched.
	const source = `main:
    mov eax, 1
    ret
`

	summary, err := parser.Parse("sample.s", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "asm" {
		t.Fatalf("expected language asm, got %q", summary.Language)
	}

	if len(summary.Symbols) == 0 {
		t.Fatal("expected at least one symbol from ASM parse")
	}
}

func TestDisassemblyEntityExtraction(t *testing.T) {
	entry := findEntryByExtension(t, ".dis")
	parser, err := NewParser(entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	const source = `0000000000001000 <main>:
    1000: 55                   push   rbp
    1001: 48 89 e5             mov    rbp,rsp
`

	summary, err := parser.Parse("sample.dis", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "disassembly" {
		t.Fatalf("expected language disassembly, got %q", summary.Language)
	}

	if !hasSymbol(summary, "function_definition", "main") {
		t.Fatal("expected function_definition main")
	}
}
