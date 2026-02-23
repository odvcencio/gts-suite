package xref

import (
	"testing"

	"gts-suite/internal/model"
)

func TestBuildAndWalk(t *testing.T) {
	idx := &model.Index{
		Root: "/tmp/repo",
		Files: []model.FileSummary{
			{
				Path: "a.go",
				Symbols: []model.Symbol{
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "A",
						StartLine: 1,
						EndLine:   1,
					},
					{
						File:      "a.go",
						Kind:      "function_definition",
						Name:      "B",
						StartLine: 3,
						EndLine:   5,
					},
				},
				References: []model.Reference{
					{
						File:        "a.go",
						Kind:        "reference.call",
						Name:        "A",
						StartLine:   4,
						EndLine:     4,
						StartColumn: 2,
						EndColumn:   3,
					},
				},
			},
			{
				Path: "c.go",
				Symbols: []model.Symbol{
					{
						File:      "c.go",
						Kind:      "function_definition",
						Name:      "C",
						StartLine: 1,
						EndLine:   3,
					},
				},
				References: []model.Reference{
					{
						File:        "c.go",
						Kind:        "reference.call",
						Name:        "B",
						StartLine:   2,
						EndLine:     2,
						StartColumn: 2,
						EndColumn:   3,
					},
				},
			},
		},
	}

	graph, err := Build(idx)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(graph.Definitions) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(graph.Definitions))
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(graph.Edges))
	}

	matches, err := graph.FindDefinitions("A", false)
	if err != nil {
		t.Fatalf("FindDefinitions returned error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 root for A, got %d", len(matches))
	}

	walk := graph.Walk([]string{matches[0].ID}, 2, true)
	if len(walk.Edges) != 2 {
		t.Fatalf("expected reverse walk to include 2 edges, got %d", len(walk.Edges))
	}
	if len(walk.Nodes) != 3 {
		t.Fatalf("expected reverse walk to include 3 nodes, got %d", len(walk.Nodes))
	}
	if graph.IncomingCount(matches[0].ID) != 1 {
		t.Fatalf("expected A incoming count to be 1, got %d", graph.IncomingCount(matches[0].ID))
	}
}

func TestBuildAmbiguousGlobalCall(t *testing.T) {
	idx := &model.Index{
		Root: "/tmp/repo",
		Files: []model.FileSummary{
			{
				Path: "x/a.go",
				Symbols: []model.Symbol{
					{
						File:      "x/a.go",
						Kind:      "function_definition",
						Name:      "Foo",
						StartLine: 1,
						EndLine:   1,
					},
				},
			},
			{
				Path: "y/b.go",
				Symbols: []model.Symbol{
					{
						File:      "y/b.go",
						Kind:      "function_definition",
						Name:      "Foo",
						StartLine: 1,
						EndLine:   1,
					},
				},
			},
			{
				Path: "z/c.go",
				Symbols: []model.Symbol{
					{
						File:      "z/c.go",
						Kind:      "function_definition",
						Name:      "Caller",
						StartLine: 1,
						EndLine:   3,
					},
				},
				References: []model.Reference{
					{
						File:        "z/c.go",
						Kind:        "reference.call",
						Name:        "Foo",
						StartLine:   2,
						EndLine:     2,
						StartColumn: 2,
						EndColumn:   5,
					},
				},
			},
		},
	}

	graph, err := Build(idx)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("expected 0 resolved edges, got %d", len(graph.Edges))
	}
	if len(graph.Unresolved) != 1 {
		t.Fatalf("expected 1 unresolved call, got %d", len(graph.Unresolved))
	}
	if graph.Unresolved[0].Reason != "ambiguous_global" {
		t.Fatalf("unexpected unresolved reason %q", graph.Unresolved[0].Reason)
	}
	if graph.Unresolved[0].CandidateCount != 2 {
		t.Fatalf("expected candidate count 2, got %d", graph.Unresolved[0].CandidateCount)
	}
}
