package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gts-suite/internal/bridge"
)

func runBridge(args []string) error {
	flags := flag.NewFlagSet("gtsbridge", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":    true,
		"--cache":   true,
		"-top":      true,
		"--top":     true,
		"-focus":    true,
		"--focus":   true,
		"-depth":    true,
		"--depth":   true,
		"-reverse":  false,
		"--reverse": false,
		"-json":     false,
		"--json":    false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	top := flags.Int("top", 20, "number of top bridge and external rows to show")
	focus := flags.String("focus", "", "focus component for bridge traversal")
	depth := flags.Int("depth", 1, "transitive traversal depth from focus")
	reverse := flags.Bool("reverse", false, "walk reverse bridge direction from focus")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsbridge accepts at most one path")
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

	report, err := bridge.Build(idx, bridge.Options{
		Top:     *top,
		Focus:   *focus,
		Depth:   *depth,
		Reverse: *reverse,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf(
		"bridge: components=%d packages=%d bridges=%d root=%s module=%s\n",
		report.ComponentCount,
		report.PackageCount,
		report.BridgeCount,
		report.Root,
		report.Module,
	)
	if len(report.Components) > 0 {
		fmt.Println("components:")
		for _, component := range report.Components {
			fmt.Printf(
				"  %s packages=%d files=%d imports:internal=%d external=%d\n",
				component.Name,
				component.PackageCount,
				component.FileCount,
				component.InternalImports,
				component.ExternalImports,
			)
		}
	}
	if len(report.TopBridges) > 0 {
		fmt.Printf("top bridges (limit=%d):\n", *top)
		for _, edge := range report.TopBridges {
			line := fmt.Sprintf("  %s -> %s count=%d", edge.From, edge.To, edge.Count)
			if len(edge.Samples) > 0 {
				line += " samples=" + strings.Join(edge.Samples, ",")
			}
			fmt.Println(line)
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
	if len(report.ExternalByComponent) > 0 {
		fmt.Printf("external pressure (limit=%d):\n", *top)
		for _, item := range report.ExternalByComponent {
			line := fmt.Sprintf("  %s count=%d", item.Component, item.Count)
			if len(item.TopImports) > 0 {
				line += " top=" + strings.Join(item.TopImports, ",")
			}
			fmt.Println(line)
		}
	}
	return nil
}
