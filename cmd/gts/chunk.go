package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/odvcencio/gts-suite/internal/chunk"
)

func newChunkCmd() *cobra.Command {
	var cachePath string
	var noCache bool
	var tokens int
	var jsonOutput bool
	var lang string
	var countOnly bool

	cmd := &cobra.Command{
		Use:     "chunk [path]",
		Aliases: []string{"gtschunk"},
		Short:   "Split code into AST-boundary chunks for RAG/indexing",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokens <= 0 {
				return fmt.Errorf("tokens must be > 0")
			}

			target := "."
			filter := ""
			if len(args) == 1 {
				target = args[0]
				if strings.TrimSpace(cachePath) != "" {
					filter = target
				}
			}

			idx, err := loadOrBuild(cachePath, target, noCache)
			if err != nil {
				return err
			}

			if lang != "" {
				filtered := idx.Files[:0]
				for _, f := range idx.Files {
					if strings.EqualFold(f.Language, lang) {
						filtered = append(filtered, f)
					}
				}
				idx.Files = filtered
			}

			report, err := chunk.Build(idx, chunk.Options{
				TokenBudget: tokens,
				FilterPath:  filter,
			})
			if err != nil {
				return err
			}

			if countOnly {
				fmt.Println(report.ChunkCount)
				return nil
			}

			if jsonOutput {
				return emitJSON(report)
			}

			fmt.Printf("chunks: %d budget=%d root=%s\n", report.ChunkCount, report.TokenBudget, report.Root)
			for _, item := range report.Chunks {
				suffix := ""
				if item.Truncated {
					suffix = " truncated=true"
				}
				fmt.Printf(
					"%s:%d:%d %s %s tokens=%d%s\n",
					item.File,
					item.StartLine,
					item.EndLine,
					item.Kind,
					strings.TrimSpace(item.Name),
					item.Tokens,
					suffix,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "skip auto-discovery of cached index")
	cmd.Flags().IntVar(&tokens, "tokens", 800, "token budget per chunk")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	cmd.Flags().StringVar(&lang, "lang", "", "filter by file language (e.g. go, python, typescript)")
	cmd.Flags().BoolVar(&countOnly, "count", false, "print only the count of chunks")
	return cmd
}

func runChunk(args []string) error {
	cmd := newChunkCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
