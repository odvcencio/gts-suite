package lint

import (
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
