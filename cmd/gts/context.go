package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gts-suite/internal/contextpack"
)

func newContextCmd() *cobra.Command {
	var cachePath string
	var rootPath string
	var line int
	var tokens int
	var semantic bool
	var semanticDepth int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "context <file>",
		Aliases: []string{"gtscontext"},
		Short:   "Pack focused code context for a file and line",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			idx, err := loadOrBuild(cachePath, rootPath)
			if err != nil {
				return err
			}

			report, err := contextpack.Build(idx, contextpack.Options{
				FilePath:      filePath,
				Line:          line,
				TokenBudget:   tokens,
				Semantic:      semantic,
				SemanticDepth: semanticDepth,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(report)
			}

			fmt.Printf("file: %s\n", report.File)
			fmt.Printf("line: %d\n", report.Line)
			fmt.Printf("budget: %d (estimated: %d)\n", report.TokenBudget, report.EstimatedTokens)
			fmt.Printf("semantic: %t\n", report.Semantic)
			if report.Semantic {
				fmt.Printf("semantic-depth: %d\n", report.SemanticDepth)
			}
			if report.Focus != nil {
				fmt.Printf("focus: %s %s [%d:%d]\n", report.Focus.Kind, symbolLabel(report.Focus.Name, report.Focus.Signature), report.Focus.StartLine, report.Focus.EndLine)
			}
			if len(report.Imports) > 0 {
				fmt.Printf("imports: %s\n", strings.Join(report.Imports, ", "))
			}
			fmt.Printf("snippet [%d:%d]:\n", report.SnippetStart, report.SnippetEnd)
			fmt.Print(report.Snippet)
			if len(report.Related) > 0 {
				fmt.Println("related:")
				for _, symbol := range report.Related {
					fmt.Printf("  %s %s [%d:%d]\n", symbol.Kind, symbolLabel(symbol.Name, symbol.Signature), symbol.StartLine, symbol.EndLine)
				}
			}
			if report.Truncated {
				fmt.Println("truncated: true")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().StringVar(&rootPath, "root", ".", "parse root path when cache is not provided")
	cmd.Flags().IntVar(&line, "line", 1, "cursor line (1-based)")
	cmd.Flags().IntVar(&tokens, "tokens", 800, "token budget")
	cmd.Flags().BoolVar(&semantic, "semantic", false, "pack semantic dependency context when possible")
	cmd.Flags().IntVar(&semanticDepth, "semantic-depth", 1, "dependency traversal depth in semantic mode")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	return cmd
}

func runContext(args []string) error {
	cmd := newContextCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
