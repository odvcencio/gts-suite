package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gts-suite/internal/deps"
)

func runDeps(args []string) error {
	flags := flag.NewFlagSet("gtsdeps", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":    true,
		"--cache":   true,
		"-by":       true,
		"--by":      true,
		"-top":      true,
		"--top":     true,
		"-focus":    true,
		"--focus":   true,
		"-depth":    true,
		"--depth":   true,
		"-reverse":  false,
		"--reverse": false,
		"-edges":    false,
		"--edges":   false,
		"-json":     false,
		"--json":    false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	by := flags.String("by", "package", "graph mode: package or file")
	top := flags.Int("top", 10, "number of top nodes to show")
	focus := flags.String("focus", "", "focus node to inspect incoming/outgoing edges")
	depth := flags.Int("depth", 1, "transitive depth for focus traversal")
	reverse := flags.Bool("reverse", false, "walk reverse dependencies from focus")
	includeEdges := flags.Bool("edges", false, "include full edge list in output")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsdeps accepts at most one path")
	}
	if *top <= 0 {
		return fmt.Errorf("top must be > 0")
	}
	if *depth <= 0 {
		return fmt.Errorf("depth must be > 0")
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	report, err := deps.Build(idx, deps.Options{
		Mode:         *by,
		Top:          *top,
		Focus:        *focus,
		Depth:        *depth,
		Reverse:      *reverse,
		IncludeEdges: *includeEdges || *jsonOutput,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf(
		"deps: mode=%s nodes=%d edges=%d internal=%d external=%d module=%s\n",
		report.Mode,
		report.NodeCount,
		report.EdgeCount,
		report.InternalEdgeCount,
		report.ExternalEdgeCount,
		report.Module,
	)

	if len(report.TopOutgoing) > 0 {
		fmt.Printf("top outgoing (limit=%d):\n", *top)
		for _, item := range report.TopOutgoing {
			fmt.Printf("  %s out=%d in=%d project=%t\n", item.Node, item.Outgoing, item.Incoming, item.IsProject)
		}
	}

	if len(report.TopIncoming) > 0 {
		fmt.Printf("top incoming (limit=%d):\n", *top)
		for _, item := range report.TopIncoming {
			fmt.Printf("  %s in=%d out=%d project=%t\n", item.Node, item.Incoming, item.Outgoing, item.IsProject)
		}
	}

	if report.Focus != "" {
		fmt.Printf("focus: %s direction=%s depth=%d\n", report.Focus, report.FocusDirection, report.FocusDepth)
		if len(report.FocusOutgoing) > 0 {
			fmt.Printf("  outgoing: %s\n", strings.Join(report.FocusOutgoing, ", "))
		}
		if len(report.FocusIncoming) > 0 {
			fmt.Printf("  incoming: %s\n", strings.Join(report.FocusIncoming, ", "))
		}
		if len(report.FocusWalk) > 0 {
			fmt.Printf("  walk: %s\n", strings.Join(report.FocusWalk, ", "))
		}
	}

	if *includeEdges {
		fmt.Println("edges:")
		for _, edge := range report.Edges {
			label := "external"
			if edge.Internal {
				label = "internal"
			}
			fmt.Printf("  %s -> %s (%s)\n", edge.From, edge.To, label)
		}
	}

	return nil
}
