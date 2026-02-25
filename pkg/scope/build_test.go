package scope

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func mustParseGo(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	entry := grammars.DetectLanguage("test.go")
	if entry == nil {
		t.Fatal("Go grammar not found")
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	ts := entry.TokenSourceFactory(srcBytes, lang)
	tree, err := parser.ParseWithTokenSource(srcBytes, ts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree, lang
}

func TestBuildGoFunctionDef(t *testing.T) {
	src := `package main

func hello() {
}
`
	tree, lang := mustParseGo(t, src)
	rules, err := LoadRules("go", lang)
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}

	scope := BuildFileScope(tree, lang, []byte(src), rules, "main.go")

	// Should have a definition for "hello"
	found := false
	for _, d := range scope.Defs {
		if d.Name == "hello" && d.Kind == DefFunction {
			found = true
		}
	}
	if !found {
		t.Errorf("expected definition for 'hello', got defs: %+v", scope.Defs)
	}
}

func TestBuildGoImport(t *testing.T) {
	src := `package main

import "fmt"

func main() {
	fmt.Println("hi")
}
`
	tree, lang := mustParseGo(t, src)
	rules, err := LoadRules("go", lang)
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}

	scope := BuildFileScope(tree, lang, []byte(src), rules, "main.go")

	// Should have import def for "fmt"
	found := false
	for _, d := range scope.Defs {
		if d.Kind == DefImport && d.ImportPath == `"fmt"` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected import def for fmt, got defs: %+v", scope.Defs)
	}
}

func TestBuildGoVarWithType(t *testing.T) {
	src := `package main

var x int
`
	tree, lang := mustParseGo(t, src)
	rules, err := LoadRules("go", lang)
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}

	scope := BuildFileScope(tree, lang, []byte(src), rules, "main.go")

	found := false
	for _, d := range scope.Defs {
		if d.Name == "x" && d.Kind == DefVariable {
			found = true
			if d.TypeAnnot == "" {
				t.Error("expected type annotation 'int'")
			}
		}
	}
	if !found {
		t.Errorf("expected variable def for x")
	}
}
