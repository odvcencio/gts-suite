// Package xref builds cross-reference graphs from structural indexes, enabling call graph traversal and dead code detection.
package xref

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gts-suite/internal/model"
)

type Definition struct {
	ID        string `json:"id"`
	File      string `json:"file"`
	Package   string `json:"package"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	Receiver  string `json:"receiver,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Callable  bool   `json:"callable"`
}

type CallSample struct {
	File        string `json:"file"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
}

type Edge struct {
	Caller     Definition   `json:"caller"`
	Callee     Definition   `json:"callee"`
	Resolution string       `json:"resolution"`
	Count      int          `json:"count"`
	Samples    []CallSample `json:"samples,omitempty"`
}

type UnresolvedCall struct {
	File           string      `json:"file"`
	Package        string      `json:"package"`
	Kind           string      `json:"kind"`
	Name           string      `json:"name"`
	StartLine      int         `json:"start_line"`
	StartColumn    int         `json:"start_column"`
	EndLine        int         `json:"end_line"`
	EndColumn      int         `json:"end_column"`
	Caller         *Definition `json:"caller,omitempty"`
	Reason         string      `json:"reason"`
	CandidateCount int         `json:"candidate_count,omitempty"`
}

type Graph struct {
	Root        string           `json:"root"`
	Definitions []Definition     `json:"definitions,omitempty"`
	Edges       []Edge           `json:"edges,omitempty"`
	Unresolved  []UnresolvedCall `json:"unresolved,omitempty"`

	defByID       map[string]Definition
	outgoingByDef map[string][]Edge
	incomingByDef map[string][]Edge
	outgoingCount map[string]int
	incomingCount map[string]int
}

type Walk struct {
	Roots   []Definition `json:"roots,omitempty"`
	Nodes   []Definition `json:"nodes,omitempty"`
	Edges   []Edge       `json:"edges,omitempty"`
	Depth   int          `json:"depth"`
	Reverse bool         `json:"reverse"`
}

func Build(idx *model.Index) (Graph, error) {
	if idx == nil {
		return Graph{}, fmt.Errorf("index is nil")
	}

	definitions := make([]Definition, 0, idx.SymbolCount())
	defByID := map[string]Definition{}
	callableByName := map[string][]Definition{}
	callableByPackageName := map[string][]Definition{}
	callableByFileName := map[string][]Definition{}
	callableByFile := map[string][]Definition{}

	for _, file := range idx.Files {
		pkg := packageFromPath(file.Path)
		for _, symbol := range file.Symbols {
			def := definitionFromSymbol(file.Path, pkg, symbol)
			definitions = append(definitions, def)
			defByID[def.ID] = def

			if !def.Callable {
				continue
			}
			callableByName[def.Name] = append(callableByName[def.Name], def)
			callableByPackageName[keyPackageName(pkg, def.Name)] = append(callableByPackageName[keyPackageName(pkg, def.Name)], def)
			callableByFileName[keyFileName(def.File, def.Name)] = append(callableByFileName[keyFileName(def.File, def.Name)], def)
			callableByFile[def.File] = append(callableByFile[def.File], def)
		}
	}

	sortDefinitions(definitions)
	for key := range callableByName {
		sortDefinitions(callableByName[key])
	}
	for key := range callableByPackageName {
		sortDefinitions(callableByPackageName[key])
	}
	for key := range callableByFileName {
		sortDefinitions(callableByFileName[key])
	}
	for key := range callableByFile {
		sortDefinitions(callableByFile[key])
	}

	edgeByPair := map[string]*Edge{}
	unresolved := make([]UnresolvedCall, 0, 32)

	for _, file := range idx.Files {
		pkg := packageFromPath(file.Path)
		callablesInFile := callableByFile[file.Path]
		for _, ref := range file.References {
			if !isCallReference(ref.Kind) {
				continue
			}

			caller := findEnclosingCallable(callablesInFile, ref.StartLine)
			if caller == nil {
				unresolved = append(unresolved, unresolvedFromRef(file.Path, pkg, ref, nil, "outside_callable", 0))
				continue
			}

			callee, resolution, reason, candidateCount, ok := resolveCallee(file.Path, pkg, ref.Name, callableByFileName, callableByPackageName, callableByName)
			if !ok {
				callerCopy := *caller
				unresolved = append(unresolved, unresolvedFromRef(file.Path, pkg, ref, &callerCopy, reason, candidateCount))
				continue
			}

			pairKey := keyPair(caller.ID, callee.ID)
			edge, exists := edgeByPair[pairKey]
			if !exists {
				edge = &Edge{
					Caller:     *caller,
					Callee:     callee,
					Resolution: resolution,
					Count:      0,
					Samples:    make([]CallSample, 0, 3),
				}
				edgeByPair[pairKey] = edge
			}
			edge.Count++
			if len(edge.Samples) < 3 {
				edge.Samples = append(edge.Samples, CallSample{
					File:        file.Path,
					StartLine:   ref.StartLine,
					StartColumn: ref.StartColumn,
					Kind:        ref.Kind,
					Name:        ref.Name,
				})
			}
		}
	}

	edges := make([]Edge, 0, len(edgeByPair))
	outgoingByDef := map[string][]Edge{}
	incomingByDef := map[string][]Edge{}
	outgoingCount := map[string]int{}
	incomingCount := map[string]int{}
	for _, edge := range edgeByPair {
		edges = append(edges, *edge)
		outgoingByDef[edge.Caller.ID] = append(outgoingByDef[edge.Caller.ID], *edge)
		incomingByDef[edge.Callee.ID] = append(incomingByDef[edge.Callee.ID], *edge)
		outgoingCount[edge.Caller.ID] += edge.Count
		incomingCount[edge.Callee.ID] += edge.Count
	}

	sort.Slice(edges, func(i, j int) bool {
		return edgeLess(edges[i], edges[j])
	})
	for key := range outgoingByDef {
		sort.Slice(outgoingByDef[key], func(i, j int) bool {
			return edgeLess(outgoingByDef[key][i], outgoingByDef[key][j])
		})
	}
	for key := range incomingByDef {
		sort.Slice(incomingByDef[key], func(i, j int) bool {
			return edgeLess(incomingByDef[key][i], incomingByDef[key][j])
		})
	}

	sort.Slice(unresolved, func(i, j int) bool {
		if unresolved[i].File == unresolved[j].File {
			if unresolved[i].StartLine == unresolved[j].StartLine {
				if unresolved[i].StartColumn == unresolved[j].StartColumn {
					return unresolved[i].Name < unresolved[j].Name
				}
				return unresolved[i].StartColumn < unresolved[j].StartColumn
			}
			return unresolved[i].StartLine < unresolved[j].StartLine
		}
		return unresolved[i].File < unresolved[j].File
	})

	return Graph{
		Root:          idx.Root,
		Definitions:   definitions,
		Edges:         edges,
		Unresolved:    unresolved,
		defByID:       defByID,
		outgoingByDef: outgoingByDef,
		incomingByDef: incomingByDef,
		outgoingCount: outgoingCount,
		incomingCount: incomingCount,
	}, nil
}

func (g Graph) FindDefinitions(pattern string, regexMode bool) ([]Definition, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("definition matcher cannot be empty")
	}

	match := func(name string) bool { return name == pattern }
	if regexMode {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compile regex: %w", err)
		}
		match = compiled.MatchString
	}

	matches := make([]Definition, 0, 16)
	for _, definition := range g.Definitions {
		if !definition.Callable {
			continue
		}
		if !match(definition.Name) {
			continue
		}
		matches = append(matches, definition)
	}
	sortDefinitions(matches)
	return matches, nil
}

func (g Graph) IncomingCount(defID string) int {
	return g.incomingCount[defID]
}

func (g Graph) OutgoingCount(defID string) int {
	return g.outgoingCount[defID]
}

func (g Graph) OutgoingEdges(defID string) []Edge {
	edges := g.outgoingByDef[defID]
	if len(edges) == 0 {
		return nil
	}
	out := make([]Edge, len(edges))
	copy(out, edges)
	return out
}

func (g Graph) IncomingEdges(defID string) []Edge {
	edges := g.incomingByDef[defID]
	if len(edges) == 0 {
		return nil
	}
	out := make([]Edge, len(edges))
	copy(out, edges)
	return out
}

func (g Graph) Walk(rootIDs []string, depth int, reverse bool) Walk {
	if depth <= 0 {
		depth = 1
	}

	roots := make([]Definition, 0, len(rootIDs))
	rootSet := map[string]bool{}
	for _, rootID := range rootIDs {
		rootID = strings.TrimSpace(rootID)
		if rootID == "" || rootSet[rootID] {
			continue
		}
		root, ok := g.defByID[rootID]
		if !ok {
			continue
		}
		rootSet[rootID] = true
		roots = append(roots, root)
	}
	sortDefinitions(roots)

	type queueItem struct {
		id    string
		depth int
	}
	queue := make([]queueItem, 0, len(roots))
	visitedNodes := map[string]bool{}
	for _, root := range roots {
		visitedNodes[root.ID] = true
		queue = append(queue, queueItem{id: root.ID, depth: 0})
	}

	edgeSet := map[string]Edge{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= depth {
			continue
		}

		var candidateEdges []Edge
		if reverse {
			candidateEdges = g.incomingByDef[current.id]
		} else {
			candidateEdges = g.outgoingByDef[current.id]
		}

		for _, edge := range candidateEdges {
			edgeSet[keyPair(edge.Caller.ID, edge.Callee.ID)] = edge

			nextID := edge.Callee.ID
			if reverse {
				nextID = edge.Caller.ID
			}
			if visitedNodes[nextID] {
				continue
			}
			visitedNodes[nextID] = true
			queue = append(queue, queueItem{id: nextID, depth: current.depth + 1})
		}
	}

	nodes := make([]Definition, 0, len(visitedNodes))
	for id := range visitedNodes {
		if definition, ok := g.defByID[id]; ok {
			nodes = append(nodes, definition)
		}
	}
	sortDefinitions(nodes)

	edges := make([]Edge, 0, len(edgeSet))
	for _, edge := range edgeSet {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edgeLess(edges[i], edges[j])
	})

	return Walk{
		Roots:   roots,
		Nodes:   nodes,
		Edges:   edges,
		Depth:   depth,
		Reverse: reverse,
	}
}

func resolveCallee(
	filePath, pkg, name string,
	callableByFileName map[string][]Definition,
	callableByPackageName map[string][]Definition,
	callableByName map[string][]Definition,
) (Definition, string, string, int, bool) {
	if candidates := uniqueDefinitions(callableByFileName[keyFileName(filePath, name)]); len(candidates) > 0 {
		if len(candidates) == 1 {
			return candidates[0], "file", "", 1, true
		}
		return Definition{}, "", "ambiguous_file", len(candidates), false
	}

	if candidates := uniqueDefinitions(callableByPackageName[keyPackageName(pkg, name)]); len(candidates) > 0 {
		if len(candidates) == 1 {
			return candidates[0], "package", "", 1, true
		}
		return Definition{}, "", "ambiguous_package", len(candidates), false
	}

	if candidates := uniqueDefinitions(callableByName[name]); len(candidates) > 0 {
		if len(candidates) == 1 {
			return candidates[0], "global", "", 1, true
		}
		return Definition{}, "", "ambiguous_global", len(candidates), false
	}

	return Definition{}, "", "not_found", 0, false
}

func unresolvedFromRef(filePath, pkg string, ref model.Reference, caller *Definition, reason string, candidateCount int) UnresolvedCall {
	return UnresolvedCall{
		File:           filePath,
		Package:        pkg,
		Kind:           ref.Kind,
		Name:           ref.Name,
		StartLine:      ref.StartLine,
		StartColumn:    ref.StartColumn,
		EndLine:        ref.EndLine,
		EndColumn:      ref.EndColumn,
		Caller:         caller,
		Reason:         reason,
		CandidateCount: candidateCount,
	}
}

func definitionFromSymbol(filePath, pkg string, symbol model.Symbol) Definition {
	return Definition{
		ID:        keyDefinition(filePath, symbol.Kind, symbol.Name, symbol.StartLine),
		File:      filePath,
		Package:   pkg,
		Kind:      symbol.Kind,
		Name:      symbol.Name,
		Signature: symbol.Signature,
		Receiver:  symbol.Receiver,
		StartLine: symbol.StartLine,
		EndLine:   symbol.EndLine,
		Callable:  isCallableKind(symbol.Kind),
	}
}

func findEnclosingCallable(definitions []Definition, line int) *Definition {
	if len(definitions) == 0 {
		return nil
	}

	bestIndex := -1
	bestSpan := 0
	for i := range definitions {
		definition := definitions[i]
		if line < definition.StartLine || line > definition.EndLine {
			continue
		}
		span := definition.EndLine - definition.StartLine
		if bestIndex == -1 || span < bestSpan || (span == bestSpan && definition.StartLine > definitions[bestIndex].StartLine) {
			bestIndex = i
			bestSpan = span
		}
	}

	if bestIndex == -1 {
		return nil
	}
	copyDef := definitions[bestIndex]
	return &copyDef
}

func isCallableKind(kind string) bool {
	switch kind {
	case "function_definition", "method_definition":
		return true
	default:
		return false
	}
}

func isCallReference(kind string) bool {
	return strings.HasPrefix(strings.TrimSpace(kind), "reference.call")
}

func sortDefinitions(items []Definition) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].File == items[j].File {
			if items[i].StartLine == items[j].StartLine {
				if items[i].Kind == items[j].Kind {
					return items[i].Name < items[j].Name
				}
				return items[i].Kind < items[j].Kind
			}
			return items[i].StartLine < items[j].StartLine
		}
		return items[i].File < items[j].File
	})
}

func edgeLess(left, right Edge) bool {
	if left.Caller.File == right.Caller.File {
		if left.Caller.StartLine == right.Caller.StartLine {
			if left.Caller.Name == right.Caller.Name {
				if left.Callee.File == right.Callee.File {
					if left.Callee.StartLine == right.Callee.StartLine {
						return left.Callee.Name < right.Callee.Name
					}
					return left.Callee.StartLine < right.Callee.StartLine
				}
				return left.Callee.File < right.Callee.File
			}
			return left.Caller.Name < right.Caller.Name
		}
		return left.Caller.StartLine < right.Caller.StartLine
	}
	return left.Caller.File < right.Caller.File
}

func uniqueDefinitions(items []Definition) []Definition {
	if len(items) == 0 {
		return nil
	}

	seen := map[string]bool{}
	unique := make([]Definition, 0, len(items))
	for _, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		unique = append(unique, item)
	}
	sortDefinitions(unique)
	return unique
}

func packageFromPath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	dir := filepath.ToSlash(filepath.Dir(cleaned))
	if dir == "." || dir == "/" {
		return "."
	}
	return dir
}

func keyDefinition(filePath, kind, name string, startLine int) string {
	return filePath + "\x00" + kind + "\x00" + name + "\x00" + fmt.Sprintf("%d", startLine)
}

func keyFileName(filePath, name string) string {
	return filePath + "\x00" + name
}

func keyPackageName(pkg, name string) string {
	return pkg + "\x00" + name
}

func keyPair(leftID, rightID string) string {
	return leftID + "\x00" + rightID
}
