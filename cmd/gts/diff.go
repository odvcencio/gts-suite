package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gts-suite/internal/structdiff"
)

func runDiff(args []string) error {
	flags := flag.NewFlagSet("gtsdiff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-before-cache":  true,
		"--before-cache": true,
		"-after-cache":   true,
		"--after-cache":  true,
		"-json":          false,
		"--json":         false,
	})

	beforeCache := flags.String("before-cache", "", "load before snapshot from cache file")
	afterCache := flags.String("after-cache", "", "load after snapshot from cache file")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 2 {
		return errors.New("usage: gtsdiff [before-path] [after-path]")
	}

	beforeTarget, afterTarget, err := resolveDiffSources(flags.Args(), *beforeCache, *afterCache)
	if err != nil {
		return err
	}

	beforeIndex, err := loadOrBuild(*beforeCache, beforeTarget)
	if err != nil {
		return fmt.Errorf("load before snapshot: %w", err)
	}
	afterIndex, err := loadOrBuild(*afterCache, afterTarget)
	if err != nil {
		return fmt.Errorf("load after snapshot: %w", err)
	}

	report := structdiff.Compare(beforeIndex, afterIndex)
	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf("changed files: %d\n", report.Stats.ChangedFiles)
	fmt.Printf("symbols: +%d -%d ~%d\n", report.Stats.AddedSymbols, report.Stats.RemovedSymbols, report.Stats.ModifiedSymbols)

	for _, item := range report.AddedSymbols {
		fmt.Printf("+ %s:%d:%d %s %s\n", item.File, item.StartLine, item.EndLine, item.Kind, symbolLabel(item.Name, item.Signature))
	}
	for _, item := range report.RemovedSymbols {
		fmt.Printf("- %s:%d:%d %s %s\n", item.File, item.StartLine, item.EndLine, item.Kind, symbolLabel(item.Name, item.Signature))
	}
	for _, item := range report.ModifiedSymbols {
		fmt.Printf("~ %s:%d:%d %s %s fields=%s\n",
			item.After.File,
			item.After.StartLine,
			item.After.EndLine,
			item.After.Kind,
			symbolLabel(item.After.Name, item.After.Signature),
			strings.Join(item.Fields, ","))
	}
	for _, change := range report.ImportChanges {
		parts := make([]string, 0, 2)
		if len(change.Added) > 0 {
			parts = append(parts, "added="+strings.Join(change.Added, ","))
		}
		if len(change.Removed) > 0 {
			parts = append(parts, "removed="+strings.Join(change.Removed, ","))
		}
		fmt.Printf("i %s %s\n", change.File, strings.Join(parts, " "))
	}
	return nil
}

func resolveDiffSources(args []string, beforeCache, afterCache string) (string, string, error) {
	positionals := append([]string(nil), args...)

	beforeTarget := ""
	afterTarget := ""

	if strings.TrimSpace(beforeCache) == "" {
		if len(positionals) == 0 {
			return "", "", errors.New("missing before source: provide [before-path] or --before-cache")
		}
		beforeTarget = positionals[0]
		positionals = positionals[1:]
	}

	if strings.TrimSpace(afterCache) == "" {
		if len(positionals) == 0 {
			return "", "", errors.New("missing after source: provide [after-path] or --after-cache")
		}
		afterTarget = positionals[0]
		positionals = positionals[1:]
	}

	if len(positionals) > 0 {
		return "", "", fmt.Errorf("unexpected positional arguments: %s", strings.Join(positionals, " "))
	}

	return beforeTarget, afterTarget, nil
}
