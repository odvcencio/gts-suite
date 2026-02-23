package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	gtsscope "gts-suite/internal/scope"
)

func runScope(args []string) error {
	flags := flag.NewFlagSet("gtsscope", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":  true,
		"--cache": true,
		"-root":   true,
		"--root":  true,
		"-line":   true,
		"--line":  true,
		"-json":   false,
		"--json":  false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	rootPath := flags.String("root", ".", "parse root path when cache is not provided")
	line := flags.Int("line", 1, "cursor line (1-based)")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gtsscope <file>")
	}

	filePath := flags.Arg(0)
	idx, err := loadOrBuild(*cachePath, *rootPath)
	if err != nil {
		return err
	}

	report, err := gtsscope.Build(idx, gtsscope.Options{
		FilePath: filePath,
		Line:     *line,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf("file: %s\n", report.File)
	fmt.Printf("line: %d\n", report.Line)
	fmt.Printf("package: %s\n", report.Package)
	if report.Focus != nil {
		fmt.Printf("focus: %s %s [%d:%d]\n", report.Focus.Kind, symbolLabel(report.Focus.Name, report.Focus.Signature), report.Focus.StartLine, report.Focus.EndLine)
	}
	fmt.Printf("symbols: %d\n", len(report.Symbols))
	for _, symbol := range report.Symbols {
		if symbol.Detail != "" {
			fmt.Printf("  %s (%s) line=%d detail=%s\n", symbol.Name, symbol.Kind, symbol.DeclLine, symbol.Detail)
			continue
		}
		fmt.Printf("  %s (%s) line=%d\n", symbol.Name, symbol.Kind, symbol.DeclLine)
	}

	return nil
}
