package contextpack

import (
	"os"
	"path/filepath"
	"testing"

	"gts-suite/internal/model"
)

func TestBuild_FocusAndSnippet(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "sample.go")
	source := `package sample

import "fmt"

type Service struct{}

func (s *Service) Work() {
	fmt.Println("ok")
}
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	idx := &model.Index{
		Root: tmpDir,
		Files: []model.FileSummary{
			{
				Path:    "sample.go",
				Imports: []string{"fmt"},
				Symbols: []model.Symbol{
					{
						File:      "sample.go",
						Kind:      "type_definition",
						Name:      "Service",
						Signature: "type Service struct{}",
						StartLine: 5,
						EndLine:   5,
					},
					{
						File:      "sample.go",
						Kind:      "method_definition",
						Name:      "Work",
						Signature: "func (s *Service) Work()",
						StartLine: 7,
						EndLine:   9,
					},
				},
			},
		},
	}

	report, err := Build(idx, Options{
		FilePath:    sourcePath,
		Line:        8,
		TokenBudget: 400,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if report.File != "sample.go" {
		t.Fatalf("expected report file sample.go, got %q", report.File)
	}
	if report.Focus == nil || report.Focus.Name != "Work" {
		t.Fatalf("expected focus symbol Work, got %#v", report.Focus)
	}
	if report.SnippetStart != 7 || report.SnippetEnd != 9 {
		t.Fatalf("unexpected snippet bounds %d:%d", report.SnippetStart, report.SnippetEnd)
	}
	if len(report.Imports) != 1 || report.Imports[0] != "fmt" {
		t.Fatalf("unexpected imports %v", report.Imports)
	}
}

func TestBuild_RequiresFilePath(t *testing.T) {
	_, err := Build(&model.Index{}, Options{})
	if err == nil {
		t.Fatal("expected Build to fail when file path is missing")
	}
}
