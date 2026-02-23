package golang

import (
	"testing"

	"gts-suite/internal/model"
)

func TestParseExtractsGoSymbols(t *testing.T) {
	const source = `package demo

import (
	"fmt"
	"net/http"
)

type Service struct {
	Name string
}

func TestService() error {
	fmt.Println("ok")
	return nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}
`

	parser := NewParser()
	summary, err := parser.Parse("demo.go", []byte(source))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if summary.Language != "go" {
		t.Fatalf("expected language go, got %q", summary.Language)
	}
	if len(summary.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(summary.Imports))
	}

	if !hasSymbol(summary, "type_definition", "Service") {
		t.Fatal("expected type_definition Service")
	}
	if !hasSymbol(summary, "function_definition", "TestService") {
		t.Fatal("expected function_definition TestService")
	}
	if !hasSymbol(summary, "method_definition", "ServeHTTP") {
		t.Fatal("expected method_definition ServeHTTP")
	}
}

func hasSymbol(summary model.FileSummary, kind, name string) bool {
	for _, symbol := range summary.Symbols {
		if symbol.Kind == kind && symbol.Name == name {
			return true
		}
	}
	return false
}
