package main

import (
	"flag"
	"fmt"
	"os"

	"gts-suite/internal/files"
)

func runFiles(args []string) error {
	flags := flag.NewFlagSet("gtsfiles", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":        true,
		"--cache":       true,
		"-language":     true,
		"--language":    true,
		"-min-symbols":  true,
		"--min-symbols": true,
		"-sort":         true,
		"--sort":        true,
		"-top":          true,
		"--top":         true,
		"-json":         false,
		"--json":        false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	language := flags.String("language", "", "filter by language (e.g. go)")
	minSymbols := flags.Int("min-symbols", 0, "minimum symbols per file")
	sortBy := flags.String("sort", "symbols", "sort by symbols|imports|size|path")
	top := flags.Int("top", 50, "maximum files to show")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsfiles accepts at most one path")
	}
	if *minSymbols < 0 {
		return fmt.Errorf("min-symbols must be >= 0")
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

	report, err := files.Build(idx, files.Options{
		Language:   *language,
		MinSymbols: *minSymbols,
		SortBy:     *sortBy,
		Top:        *top,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
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
}
