package stats

import (
	"testing"

	"gts-suite/internal/model"
)

func TestBuildAggregatesCounts(t *testing.T) {
	idx := &model.Index{
		Root: "/tmp/repo",
		Files: []model.FileSummary{
			{
				Path:     "a.go",
				Language: "go",
				Imports:  []string{"fmt"},
				Symbols: []model.Symbol{
					{Kind: "function_definition", Name: "A"},
					{Kind: "type_definition", Name: "T"},
				},
			},
			{
				Path:     "b.go",
				Language: "go",
				Symbols: []model.Symbol{
					{Kind: "function_definition", Name: "B"},
				},
			},
		},
		Errors: []model.ParseError{{Path: "bad.go", Error: "parse error"}},
	}

	report, err := Build(idx, Options{TopFiles: 1})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if report.FileCount != 2 || report.SymbolCount != 3 || report.ParseErrorCount != 1 {
		t.Fatalf("unexpected report totals: %+v", report)
	}
	if len(report.KindCounts) < 2 {
		t.Fatalf("expected at least 2 kind counts, got %d", len(report.KindCounts))
	}
	if report.KindCounts[0].Kind != "function_definition" || report.KindCounts[0].Count != 2 {
		t.Fatalf("unexpected top kind count: %+v", report.KindCounts[0])
	}
	if len(report.TopFiles) != 1 || report.TopFiles[0].Path != "a.go" {
		t.Fatalf("unexpected top files: %+v", report.TopFiles)
	}
	if len(report.Languages) != 1 || report.Languages[0].Language != "go" || report.Languages[0].Files != 2 {
		t.Fatalf("unexpected language counts: %+v", report.Languages)
	}
}

func TestBuildNilIndex(t *testing.T) {
	_, err := Build(nil, Options{})
	if err == nil {
		t.Fatal("expected nil index to fail")
	}
}
