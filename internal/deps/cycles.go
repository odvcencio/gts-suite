package deps

import "sort"

// Cycle represents a circular import chain.
type Cycle struct {
	Path []string `json:"path"` // e.g. ["pkg/a", "pkg/b", "pkg/a"]
}

// DetectCycles finds all import cycles in the dependency graph.
// The graph maps each node to its list of direct dependencies.
func DetectCycles(graph map[string][]string) []Cycle {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var cycles []Cycle

	var dfs func(node string, stack []string)
	dfs = func(node string, stack []string) {
		if inStack[node] {
			// Found cycle — extract it
			start := -1
			for i, s := range stack {
				if s == node {
					start = i
					break
				}
			}
			if start >= 0 {
				cycle := make([]string, len(stack)-start+1)
				copy(cycle, stack[start:])
				cycle[len(cycle)-1] = node
				cycles = append(cycles, Cycle{Path: cycle})
			}
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		inStack[node] = true
		for _, neighbor := range graph[node] {
			dfs(neighbor, append(stack, node))
		}
		inStack[node] = false
	}

	// Sort nodes for deterministic output
	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	for _, node := range nodes {
		dfs(node, nil)
	}

	return deduplicateCycles(cycles)
}

// GraphFromEdges builds the adjacency map that DetectCycles expects,
// using only internal edges from a deps report.
func GraphFromEdges(edges []Edge) map[string][]string {
	graph := make(map[string][]string)
	for _, edge := range edges {
		if !edge.Internal {
			continue
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	// Sort adjacency lists for deterministic traversal
	for node := range graph {
		sort.Strings(graph[node])
	}
	return graph
}

// deduplicateCycles removes rotational duplicates.
// A->B->A is the same cycle as B->A->B.
func deduplicateCycles(cycles []Cycle) []Cycle {
	if len(cycles) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var unique []Cycle

	for _, c := range cycles {
		key := canonicalKey(c.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, c)
	}

	// Sort by canonical key for deterministic output
	sort.Slice(unique, func(i, j int) bool {
		return canonicalKey(unique[i].Path) < canonicalKey(unique[j].Path)
	})

	return unique
}

// canonicalKey returns a string that is identical for all rotations of the
// same cycle. It does this by finding the lexicographically smallest rotation
// of the cycle body (excluding the repeated tail element).
func canonicalKey(path []string) string {
	if len(path) < 2 {
		return ""
	}
	// The cycle body is everything except the last element (which repeats the first).
	body := path[:len(path)-1]
	n := len(body)

	// Find the rotation that starts with the lexicographically smallest element.
	minIdx := 0
	for i := 1; i < n; i++ {
		if body[i] < body[minIdx] {
			minIdx = i
		}
	}

	// Build canonical form from that rotation.
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = body[(minIdx+i)%n]
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " -> "
		}
		result += p
	}
	return result
}
