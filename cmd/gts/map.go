package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newMapCmd() *cobra.Command {
	var cachePath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "map [path]",
		Aliases: []string{"gtsmap"},
		Short:   "Print structural summaries for indexed files",
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

			if jsonOutput {
				return emitJSON(idx)
			}

			for _, file := range idx.Files {
				fmt.Printf("%s (%s)\n", file.Path, file.Language)
				if len(file.Imports) > 0 {
					fmt.Printf("  imports: %s\n", strings.Join(file.Imports, ", "))
				}
				for _, symbol := range file.Symbols {
					if symbol.Signature != "" {
						fmt.Printf("  %s %s [%d:%d]\n", symbol.Kind, symbol.Signature, symbol.StartLine, symbol.EndLine)
						continue
					}
					fmt.Printf("  %s %s [%d:%d]\n", symbol.Kind, symbol.Name, symbol.StartLine, symbol.EndLine)
				}
			}

			if len(idx.Errors) > 0 {
				fmt.Printf("errors: %d\n", len(idx.Errors))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	return cmd
}

func runMap(args []string) error {
	cmd := newMapCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
