package main

import (
	"flag"
	"fmt"
	"os"

	"gts-suite/internal/stats"
)

func runStats(args []string) error {
	flags := flag.NewFlagSet("gtsstats", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":  true,
		"--cache": true,
		"-top":    true,
		"--top":   true,
		"-json":   false,
		"--json":  false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	top := flags.Int("top", 10, "number of top files by symbol count")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsstats accepts at most one path")
	}
	if *top <= 0 {
		return fmt.Errorf("top must be > 0")
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	report, err := stats.Build(idx, stats.Options{
		TopFiles: *top,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
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
		fmt.Printf("top files (limit=%d):\n", *top)
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
}
