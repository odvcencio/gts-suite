package treesitter

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"

	"gts-suite/internal/model"
)

func TestParseGoSymbolsAndImports(t *testing.T) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		t.Fatal("expected go language entry")
	}

	parser, err := NewParser(*entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	const source = `package demo

import (
	"fmt"
	"net/http"
)

type Service struct{}

func TestService() error {
	fmt.Println("ok")
	return nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
`

	summary, err := parser.Parse("main.go", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "go" {
		t.Fatalf("expected language go, got %q", summary.Language)
	}
	if len(summary.Imports) != 2 {
		t.Fatalf("expected two imports, got %d", len(summary.Imports))
	}

	if !hasSymbol(summary, "type_definition", "Service") {
		t.Fatal("expected type_definition Service")
	}
	if !hasSymbol(summary, "function_definition", "TestService") {
		t.Fatal("expected function_definition TestService")
	}
	method := findSymbol(summary, "method_definition", "ServeHTTP")
	if method == nil {
		t.Fatal("expected method_definition ServeHTTP")
	}
	if method.Receiver == "" {
		t.Fatal("expected receiver metadata for method definition")
	}
	if !hasReference(summary, "reference.call", "Println") {
		t.Fatal("expected reference.call Println")
	}
}

func TestParsePythonSymbols(t *testing.T) {
	entry := grammars.DetectLanguage("main.py")
	if entry == nil {
		t.Fatal("expected python language entry")
	}

	parser, err := NewParser(*entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	const source = `class Worker:
    def run(self):
        return 1

def helper():
    return Worker().run()
`
	summary, err := parser.Parse("main.py", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "python" {
		t.Fatalf("expected language python, got %q", summary.Language)
	}
	if !hasSymbol(summary, "type_definition", "Worker") {
		t.Fatal("expected type_definition Worker")
	}
	if !hasSymbol(summary, "function_definition", "helper") {
		t.Fatal("expected function_definition helper")
	}
	if !hasReference(summary, "reference.call", "run") {
		t.Fatal("expected reference.call run")
	}
}

func hasSymbol(summary model.FileSummary, kind, name string) bool {
	return findSymbol(summary, kind, name) != nil
}

func findSymbol(summary model.FileSummary, kind, name string) *model.Symbol {
	for i := range summary.Symbols {
		symbol := summary.Symbols[i]
		if symbol.Kind == kind && symbol.Name == name {
			return &summary.Symbols[i]
		}
	}
	return nil
}

func hasReference(summary model.FileSummary, kind, name string) bool {
	for _, reference := range summary.References {
		if reference.Kind == kind && reference.Name == name {
			return true
		}
	}
	return false
}
