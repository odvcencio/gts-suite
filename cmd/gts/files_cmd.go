package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"gts-suite/internal/files"
)

func newFilesCmd() *cobra.Command {
	var cachePath string
	var language string
	var minSymbols int
	var sortBy string
	var top int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "files [path]",
		Aliases: []string{"gtsfiles"},
		Short:   "List/index files with structural density filters",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if minSymbols < 0 {
				return fmt.Errorf("min-symbols must be >= 0")
			}
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

			report, err := files.Build(idx, files.Options{
				Language:   language,
				MinSymbols: minSymbols,
				SortBy:     sortBy,
				Top:        top,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(report)
			}

			fmt.Printf("files: total=%d shown=%d root=%s\n", report.TotalFiles, report.ShownFiles, report.Root)
			for _, entry := range report.Entries {
				fmt.Printf(
					"%s language=%s symbols=%d imports=%d size=%d\n",
					entry.Path,
					entry.Language,
					entry.Symbols,
					entry.Imports,
					entry.SizeBytes,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().StringVar(&language, "language", "", "filter by language (e.g. go)")
	cmd.Flags().IntVar(&minSymbols, "min-symbols", 0, "minimum symbols per file")
	cmd.Flags().StringVar(&sortBy, "sort", "symbols", "sort by symbols|imports|size|path")
	cmd.Flags().IntVar(&top, "top", 50, "maximum files to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	return cmd
}

func runFiles(args []string) error {
	cmd := newFilesCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
