package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gts-suite/internal/contextpack"
)

func runContext(args []string) error {
	flags := flag.NewFlagSet("gtscontext", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":           true,
		"--cache":          true,
		"-root":            true,
		"--root":           true,
		"-line":            true,
		"--line":           true,
		"-tokens":          true,
		"--tokens":         true,
		"-semantic":        false,
		"--semantic":       false,
		"-semantic-depth":  true,
		"--semantic-depth": true,
		"-json":            false,
		"--json":           false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	rootPath := flags.String("root", ".", "parse root path when cache is not provided")
	line := flags.Int("line", 1, "cursor line (1-based)")
	tokens := flags.Int("tokens", 800, "token budget")
	semantic := flags.Bool("semantic", false, "pack semantic dependency context when possible")
	semanticDepth := flags.Int("semantic-depth", 1, "dependency traversal depth in semantic mode")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gtscontext <file>")
	}

	filePath := flags.Arg(0)
	idx, err := loadOrBuild(*cachePath, *rootPath)
	if err != nil {
		return err
	}

	report, err := contextpack.Build(idx, contextpack.Options{
		FilePath:      filePath,
		Line:          *line,
		TokenBudget:   *tokens,
		Semantic:      *semantic,
		SemanticDepth: *semanticDepth,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
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
}
