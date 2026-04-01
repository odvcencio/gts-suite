package deps

import (
	"testing"
)

func TestDetectCycles_NoCycles(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 0 {
		t.Errorf("expected 0 cycles, got %d", len(cycles))
	}
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}
	if len(cycles[0].Path) < 3 { // a -> b -> a
		t.Errorf("cycle too short: %v", cycles[0].Path)
	}
}

func TestDetectCycles_TriangleCycle(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}
	if len(cycles[0].Path) != 4 { // a -> b -> c -> a
		t.Errorf("expected path length 4, got %d: %v", len(cycles[0].Path), cycles[0].Path)
	}
}

func TestDetectCycles_MultipleCycles(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"a"},
		"x": {"y"},
		"y": {"x"},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestDetectCycles_SelfLoop(t *testing.T) {
	graph := map[string][]string{
		"a": {"a"},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}
	if len(cycles[0].Path) != 2 { // a -> a
		t.Errorf("expected path length 2, got %d: %v", len(cycles[0].Path), cycles[0].Path)
	}
}

func TestDetectCycles_DisjointGraphs(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {},
		"c": {"d"},
		"d": {"e"},
		"e": {"c"},
	}
	cycles := DetectCycles(graph)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
}

func TestGraphFromEdges(t *testing.T) {
	edges := []Edge{
		{From: "a", To: "b", Internal: true},
		{From: "b", To: "c", Internal: true},
		{From: "c", To: "ext/lib", Internal: false},
	}
	graph := GraphFromEdges(edges)
	if len(graph) != 2 {
		t.Errorf("expected 2 entries, got %d", len(graph))
	}
	if len(graph["a"]) != 1 || graph["a"][0] != "b" {
		t.Errorf("unexpected adjacency for a: %v", graph["a"])
	}
}

func TestDeduplicateCycles_Rotations(t *testing.T) {
	// A->B->C->A and B->C->A->B are the same cycle
	cycles := []Cycle{
		{Path: []string{"a", "b", "c", "a"}},
		{Path: []string{"b", "c", "a", "b"}},
		{Path: []string{"c", "a", "b", "c"}},
	}
	result := deduplicateCycles(cycles)
	if len(result) != 1 {
		t.Errorf("expected 1 unique cycle, got %d: %v", len(result), result)
	}
}
