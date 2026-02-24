package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/odvcencio/gts-suite/internal/stats"
)

func newStatsCmd() *cobra.Command {
	var cachePath string
	var top int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "stats [path]",
		Aliases: []string{"gtsstats"},
		Short:   "Report structural codebase metrics from an index",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if top <= 0 {
				return fmt.Errorf("top must be > 0")
			}

			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			idx, err := loadOrBuild(cachePath, target)
			if err != nil {
				return err
			}

			report, err := stats.Build(idx, stats.Options{
				TopFiles: top,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(report)
			}

			fmt.Printf(
				"stats: files=%d symbols=%d errors=%d root=%s\n",
				report.FileCount,
				report.SymbolCount,
				report.ParseErrorCount,
				report.Root,
			)
			if len(report.Languages) > 0 {
				fmt.Println("languages:")
				for _, language := range report.Languages {
					fmt.Printf("  %s files=%d symbols=%d\n", language.Language, language.Files, language.Symbols)
				}
			}
			if len(report.KindCounts) > 0 {
				fmt.Println("kinds:")
				for _, kind := range report.KindCounts {
					fmt.Printf("  %s count=%d\n", kind.Kind, kind.Count)
				}
			}
			if len(report.TopFiles) > 0 {
				fmt.Printf("top files (limit=%d):\n", top)
				for _, file := range report.TopFiles {
					fmt.Printf(
						"  %s symbols=%d imports=%d language=%s size=%d\n",
						file.Path,
						file.Symbols,
						file.Imports,
						file.Language,
						file.SizeBytes,
					)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().IntVar(&top, "top", 10, "number of top files by symbol count")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	return cmd
}

func runStats(args []string) error {
	cmd := newStatsCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
