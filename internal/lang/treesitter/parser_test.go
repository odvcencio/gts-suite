package treesitter

import (
	"os"
	"path/filepath"
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

func TestParseIncrementalWithTree(t *testing.T) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		t.Fatal("expected go language entry")
	}

	parser, err := NewParser(*entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	oldSource := []byte(`package demo

func A() {}

func B() {
	A()
}
`)
	summaryOld, oldTree, err := parser.ParseWithTree("main.go", oldSource)
	if err != nil {
		t.Fatalf("ParseWithTree returned error: %v", err)
	}
	if oldTree == nil || oldTree.RootNode() == nil {
		t.Fatal("expected non-nil tree from ParseWithTree")
	}
	defer func() {
		if oldTree != nil {
			oldTree.Release()
		}
	}()

	newSource := []byte(`package demo

func A() {}

func B() {
	A()
	A()
}
`)
	summaryNew, newTree, err := parser.ParseIncrementalWithTree("main.go", newSource, oldSource, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalWithTree returned error: %v", err)
	}
	if newTree == nil || newTree.RootNode() == nil {
		t.Fatal("expected non-nil tree from ParseIncrementalWithTree")
	}
	if summaryNew.Language != "go" || summaryOld.Language != "go" {
		t.Fatalf("unexpected language old=%q new=%q", summaryOld.Language, summaryNew.Language)
	}

	callCount := 0
	for _, reference := range summaryNew.References {
		if reference.Kind == "reference.call" && reference.Name == "A" {
			callCount++
		}
	}
	if callCount != 2 {
		t.Fatalf("expected 2 references to A after incremental parse, got %d", callCount)
	}

	if newTree != oldTree {
		newTree.Release()
	}
}

func TestSingleEdit(t *testing.T) {
	oldSrc := []byte("line1\nline2\nline3\n")
	newSrc := []byte("line1\nline-two\nline3\n")

	edit, ok := singleEdit(oldSrc, newSrc)
	if !ok {
		t.Fatal("expected singleEdit to report change")
	}
	if edit.StartByte == edit.OldEndByte {
		t.Fatalf("expected non-zero old edit span: %+v", edit)
	}
	if edit.StartPoint.Row != 1 {
		t.Fatalf("expected edit to start on row 1, got %+v", edit.StartPoint)
	}
	if pointAtOffset(oldSrc, len(oldSrc)).Row != 3 {
		t.Fatalf("unexpected pointAtOffset row for source end")
	}
}

func TestParseIncrementalWithTree_FileIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")

	entry := grammars.DetectLanguage(sourcePath)
	if entry == nil {
		t.Fatal("expected go language entry")
	}
	parser, err := NewParser(*entry)
	if err != nil {
		t.Fatalf("NewParser returned error: %v", err)
	}

	original := []byte("package demo\n\nfunc A() {}\n")
	if err := os.WriteFile(sourcePath, original, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	summary, tree, err := parser.ParseWithTree(sourcePath, original)
	if err != nil {
		t.Fatalf("ParseWithTree returned error: %v", err)
	}
	if len(summary.Symbols) == 0 {
		t.Fatal("expected symbols from initial parse")
	}
	defer func() {
		if tree != nil {
			tree.Release()
		}
	}()

	edited := []byte("package demo\n\nfunc A() {}\nfunc B() { A() }\n")
	if err := os.WriteFile(sourcePath, edited, 0o644); err != nil {
		t.Fatalf("WriteFile edited failed: %v", err)
	}
	updated, newTree, err := parser.ParseIncrementalWithTree(sourcePath, edited, original, tree)
	if err != nil {
		t.Fatalf("ParseIncrementalWithTree returned error: %v", err)
	}
	if len(updated.Symbols) < 2 {
		t.Fatalf("expected updated symbols after edit, got %d", len(updated.Symbols))
	}
	if newTree != tree {
		newTree.Release()
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
