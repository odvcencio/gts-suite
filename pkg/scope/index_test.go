package scope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gts-suite/pkg/index"
)

func TestBuildFromIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
	), 0644)

	builder := index.NewBuilder()
	idx, err := builder.BuildPath(dir)
	if err != nil {
		t.Fatalf("build index: %v", err)
	}

	graph, err := BuildFromIndex(idx, dir)
	if err != nil {
		t.Fatalf("build scope graph: %v", err)
	}

	// Should have a file scope for main.go
	fs := graph.FileScope("main.go")
	if fs == nil {
		t.Fatalf("no file scope for main.go")
	}

	// Should have defs for "main" and import "fmt"
	hasFn := false
	hasImport := false
	for _, d := range fs.Defs {
		if d.Name == "main" && d.Kind == DefFunction {
			hasFn = true
		}
		if d.Kind == DefImport && d.Name == "fmt" {
			hasImport = true
		}
	}
	if !hasFn {
		t.Error("expected function def 'main'")
	}
	if !hasImport {
		t.Error("expected import def 'fmt'")
	}
}
