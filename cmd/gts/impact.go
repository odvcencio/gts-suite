package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/odvcencio/gts-suite/pkg/impact"
)

func newImpactCmd() *cobra.Command {
	var cachePath string
	var jsonOutput bool
	var changed string
	var diffRef string
	var maxDepth int

	cmd := &cobra.Command{
		Use:     "impact [path]",
		Aliases: []string{"gtsimpact"},
		Short:   "Compute blast radius of changed symbols via reverse call graph",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			idx, err := loadOrBuild(cachePath, target)
			if err != nil {
				return err
			}

			opts := impact.Options{
				DiffRef:  strings.TrimSpace(diffRef),
				Root:     target,
				MaxDepth: maxDepth,
			}

			if strings.TrimSpace(changed) != "" {
				parts := strings.Split(changed, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						opts.Changed = append(opts.Changed, part)
					}
				}
			}

			result, err := impact.Analyze(idx, opts)
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(result)
			}

			for _, sym := range result.Affected {
				fmt.Printf(
					"%s:%d-%d %s distance=%d risk=%.2f\n",
					sym.File,
					sym.StartLine,
					sym.EndLine,
					sym.Name,
					sym.Distance,
					sym.Risk,
				)
			}
			fmt.Printf(
				"impact: changed=%d affected=%d files=%d\n",
				len(result.Changed),
				result.TotalAffected,
				len(result.AffectedFiles),
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	cmd.Flags().StringVar(&changed, "changed", "", "comma-separated list of changed symbol names")
	cmd.Flags().StringVar(&diffRef, "diff", "", "git diff ref (e.g. HEAD~1)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 10, "max reverse walk depth")
	return cmd
}
