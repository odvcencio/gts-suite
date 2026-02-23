package lint

import (
	"os"
	"path/filepath"
	"testing"

	"gts-suite/internal/model"
)

func TestParseRule_MaxLinesFunction(t *testing.T) {
	rule, err := ParseRule("no function longer than 50 lines")
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	if rule.Type != "max_lines" {
		t.Fatalf("unexpected rule type %q", rule.Type)
	}
	if rule.Kind != "function_definition" {
		t.Fatalf("unexpected kind %q", rule.Kind)
	}
	if rule.MaxLines != 50 {
		t.Fatalf("unexpected max lines %d", rule.MaxLines)
	}
}

func TestParseRule_Unsupported(t *testing.T) {
	_, err := ParseRule("ban globals")
	if err == nil {
		t.Fatal("expected ParseRule to fail for unsupported format")
	}
}

func TestEvaluate_MaxLinesViolations(t *testing.T) {
	idx := &model.Index{
		Files: []model.FileSummary{
			{
				Path: "main.go",
				Symbols: []model.Symbol{
					{
						File:      "main.go",
						Kind:      "function_definition",
						Name:      "Short",
						StartLine: 10,
						EndLine:   12,
					},
					{
						File:      "main.go",
						Kind:      "function_definition",
						Name:      "Long",
						StartLine: 20,
						EndLine:   40,
					},
				},
			},
		},
	}

	rule, err := ParseRule("no function longer than 5 lines")
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}

	violations := Evaluate(idx, []Rule{rule})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Name != "Long" {
		t.Fatalf("unexpected violation: %+v", violations[0])
	}
}

func TestParseRule_NoImport(t *testing.T) {
	rule, err := ParseRule(`no import "fmt"`)
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	if rule.Type != "no_import" {
		t.Fatalf("unexpected rule type %q", rule.Type)
	}
	if rule.ImportPath != "fmt" {
		t.Fatalf("unexpected import path %q", rule.ImportPath)
	}
}

func TestEvaluate_NoImportViolation(t *testing.T) {
	idx := &model.Index{
		Files: []model.FileSummary{
			{
				Path:    "main.go",
				Imports: []string{"fmt", "strings"},
			},
		},
	}
	rule, err := ParseRule("no import fmt")
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	violations := Evaluate(idx, []Rule{rule})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Kind != "import" || violations[0].Name != "fmt" {
		t.Fatalf("unexpected violation: %+v", violations[0])
	}
}

func TestLoadQueryPatternMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	patternPath := filepath.Join(tmpDir, "rule.scm")
	content := `; id: no-empty-functions
; message: avoid empty function bodies
(function_declaration (block) @violation)
`
	if err := os.WriteFile(patternPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	pattern, err := LoadQueryPattern(patternPath)
	if err != nil {
		t.Fatalf("LoadQueryPattern returned error: %v", err)
	}
	if pattern.ID != "no-empty-functions" {
		t.Fatalf("unexpected pattern id %q", pattern.ID)
	}
	if pattern.Message != "avoid empty function bodies" {
		t.Fatalf("unexpected pattern message %q", pattern.Message)
	}
}

func TestEvaluatePatterns(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func Empty() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile source failed: %v", err)
	}

	patternPath := filepath.Join(tmpDir, "empty.scm")
	patternBody := `(function_declaration (block) @violation)`
	if err := os.WriteFile(patternPath, []byte(patternBody), 0o644); err != nil {
		t.Fatalf("WriteFile pattern failed: %v", err)
	}

	pattern, err := LoadQueryPattern(patternPath)
	if err != nil {
		t.Fatalf("LoadQueryPattern returned error: %v", err)
	}

	idx := &model.Index{
		Root: tmpDir,
		Files: []model.FileSummary{
			{
				Path:     "main.go",
				Language: "go",
			},
		},
	}

	violations, err := EvaluatePatterns(idx, []QueryPattern{pattern})
	if err != nil {
		t.Fatalf("EvaluatePatterns returned error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != pattern.ID {
		t.Fatalf("unexpected rule id %q", violations[0].RuleID)
	}
	if violations[0].Kind != "query_pattern" {
		t.Fatalf("unexpected violation kind %q", violations[0].Kind)
	}
}
