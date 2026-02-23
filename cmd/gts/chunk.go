package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gts-suite/internal/chunk"
)

func runChunk(args []string) error {
	flags := flag.NewFlagSet("gtschunk", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":   true,
		"--cache":  true,
		"-tokens":  true,
		"--tokens": true,
		"-json":    false,
		"--json":   false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	tokens := flags.Int("tokens", 800, "token budget per chunk")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("usage: gtschunk [path]")
	}
	if *tokens <= 0 {
		return fmt.Errorf("tokens must be > 0")
	}

	target := "."
	filter := ""
	if flags.NArg() == 1 {
		target = flags.Arg(0)
		if strings.TrimSpace(*cachePath) != "" {
			filter = target
		}
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	report, err := chunk.Build(idx, chunk.Options{
		TokenBudget: *tokens,
		FilterPath:  filter,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
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
}
