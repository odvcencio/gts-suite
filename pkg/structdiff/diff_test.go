package structdiff

import (
	"testing"

	"gts-suite/pkg/model"
)

func TestCompare_SymbolAndImportChanges(t *testing.T) {
	before := &model.Index{
		Root: "/tmp/before",
		Files: []model.FileSummary{
			{
				Path:    "a.go",
				Imports: []string{"fmt", "os"},
				Symbols: []model.Symbol{
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "OldOnly",
						Signature: "func OldOnly()",
						StartLine: 3,
						EndLine:   3,
					},
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "Shared",
						Signature: "func Shared(a int)",
						StartLine: 10,
						EndLine:   12,
					},
				},
			},
		},
	}

	after := &model.Index{
		Root: "/tmp/after",
		Files: []model.FileSummary{
			{
				Path:    "a.go",
				Imports: []string{"fmt", "strings"},
				Symbols: []model.Symbol{
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "NewOnly",
						Signature: "func NewOnly()",
						StartLine: 5,
						EndLine:   5,
					},
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "Shared",
						Signature: "func Shared(a int, b string)",
						StartLine: 10,
						EndLine:   13,
					},
				},
			},
		},
	}

	report := Compare(before, after)
	if report.Stats.AddedSymbols != 1 {
		t.Fatalf("expected 1 added symbol, got %d", report.Stats.AddedSymbols)
	}
	if report.Stats.RemovedSymbols != 1 {
		t.Fatalf("expected 1 removed symbol, got %d", report.Stats.RemovedSymbols)
	}
	if report.Stats.ModifiedSymbols != 1 {
		t.Fatalf("expected 1 modified symbol, got %d", report.Stats.ModifiedSymbols)
	}
	if report.Stats.ChangedFiles != 1 {
		t.Fatalf("expected 1 changed file, got %d", report.Stats.ChangedFiles)
	}

	if len(report.ImportChanges) != 1 {
		t.Fatalf("expected 1 import change entry, got %d", len(report.ImportChanges))
	}
	imp := report.ImportChanges[0]
	if imp.File != "a.go" {
		t.Fatalf("expected import change for a.go, got %s", imp.File)
	}
	if len(imp.Added) != 1 || imp.Added[0] != "strings" {
		t.Fatalf("unexpected added imports: %v", imp.Added)
	}
	if len(imp.Removed) != 1 || imp.Removed[0] != "os" {
		t.Fatalf("unexpected removed imports: %v", imp.Removed)
	}

	mod := report.ModifiedSymbols[0]
	if len(mod.Fields) != 2 {
		t.Fatalf("expected 2 modified fields, got %v", mod.Fields)
	}
}
