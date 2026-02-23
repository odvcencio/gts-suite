package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gts-suite/internal/xref"
)

func runCallgraph(args []string) error {
	flags := flag.NewFlagSet("gtscallgraph", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":    true,
		"--cache":   true,
		"-regex":    false,
		"--regex":   false,
		"-depth":    true,
		"--depth":   true,
		"-reverse":  false,
		"--reverse": false,
		"-json":     false,
		"--json":    false,
		"-count":    false,
		"--count":   false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	regexMode := flags.Bool("regex", false, "treat the first argument as a regular expression")
	depth := flags.Int("depth", 2, "call graph traversal depth")
	reverse := flags.Bool("reverse", false, "walk incoming callers instead of outgoing callees")
	jsonOutput := flags.Bool("json", false, "emit JSON output")
	countOnly := flags.Bool("count", false, "print the number of traversed edges")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 || flags.NArg() > 2 {
		return errors.New("usage: gtscallgraph <name|regex> [path]")
	}
	if *depth <= 0 {
		return fmt.Errorf("depth must be > 0")
	}

	target := "."
	if flags.NArg() == 2 {
		target = flags.Arg(1)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	graph, err := xref.Build(idx)
	if err != nil {
		return err
	}

	roots, err := graph.FindDefinitions(flags.Arg(0), *regexMode)
	if err != nil {
		return err
	}
	rootIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		rootIDs = append(rootIDs, root.ID)
	}
	walk := graph.Walk(rootIDs, *depth, *reverse)

	if *jsonOutput {
		if *countOnly {
			return emitJSON(struct {
				RootCount      int `json:"root_count"`
				NodeCount      int `json:"node_count"`
				EdgeCount      int `json:"edge_count"`
				UnresolvedCall int `json:"unresolved_call_count"`
			}{
				RootCount:      len(walk.Roots),
				NodeCount:      len(walk.Nodes),
				EdgeCount:      len(walk.Edges),
				UnresolvedCall: len(graph.Unresolved),
			})
		}
		return emitJSON(struct {
			Roots               []xref.Definition `json:"roots,omitempty"`
			Nodes               []xref.Definition `json:"nodes,omitempty"`
			Edges               []xref.Edge       `json:"edges,omitempty"`
			Depth               int               `json:"depth"`
			Reverse             bool              `json:"reverse"`
			UnresolvedCallCount int               `json:"unresolved_call_count"`
		}{
			Roots:               walk.Roots,
			Nodes:               walk.Nodes,
			Edges:               walk.Edges,
			Depth:               walk.Depth,
			Reverse:             walk.Reverse,
			UnresolvedCallCount: len(graph.Unresolved),
		})
	}

	if *countOnly {
		fmt.Println(len(walk.Edges))
		return nil
	}

	fmt.Printf(
		"callgraph: roots=%d nodes=%d edges=%d depth=%d reverse=%t unresolved=%d\n",
		len(walk.Roots),
		len(walk.Nodes),
		len(walk.Edges),
		walk.Depth,
		walk.Reverse,
		len(graph.Unresolved),
	)
	for _, root := range walk.Roots {
		fmt.Printf("root: %s:%d %s %s\n", root.File, root.StartLine, root.Kind, definitionLabel(root))
	}
	for _, edge := range walk.Edges {
		fmt.Printf(
			"%s:%d %s -> %s:%d %s count=%d resolution=%s\n",
			edge.Caller.File,
			edge.Caller.StartLine,
			definitionLabel(edge.Caller),
			edge.Callee.File,
			edge.Callee.StartLine,
			definitionLabel(edge.Callee),
			edge.Count,
			edge.Resolution,
		)
	}
	return nil
}
