package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func runMap(args []string) error {
	flags := flag.NewFlagSet("gtsmap", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":  true,
		"--cache": true,
		"-json":   false,
		"--json":  false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsmap accepts at most one path")
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	if *jsonOutput {
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
}
