package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"

	"gts-suite/internal/query"
)

func runGrep(args []string) error {
	flags := flag.NewFlagSet("gtsgrep", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":  true,
		"--cache": true,
		"-json":   false,
		"--json":  false,
		"-count":  false,
		"--count": false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	jsonOutput := flags.Bool("json", false, "emit JSON output")
	countOnly := flags.Bool("count", false, "print the number of matches")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 || flags.NArg() > 2 {
		return errors.New("usage: gtsgrep <selector> [path]")
	}

	selector, err := query.ParseSelector(flags.Arg(0))
	if err != nil {
		return err
	}

	target := "."
	if flags.NArg() == 2 {
		target = flags.Arg(1)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	matches := make([]grepMatch, 0, idx.SymbolCount())
	for _, file := range idx.Files {
		for _, symbol := range file.Symbols {
			if !selector.Match(symbol) {
				continue
			}
			matches = append(matches, grepMatch{
				File:      file.Path,
				Kind:      symbol.Kind,
				Name:      symbol.Name,
				Signature: symbol.Signature,
				StartLine: symbol.StartLine,
				EndLine:   symbol.EndLine,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				return matches[i].Name < matches[j].Name
			}
			return matches[i].StartLine < matches[j].StartLine
		}
		return matches[i].File < matches[j].File
	})

	if *jsonOutput {
		if *countOnly {
			return emitJSON(struct {
				Count int `json:"count"`
			}{
				Count: len(matches),
			})
		}
		return emitJSON(matches)
	}

	if *countOnly {
		fmt.Println(len(matches))
		return nil
	}

	for _, match := range matches {
		if match.Signature != "" {
			fmt.Printf("%s:%d:%d %s %s\n", match.File, match.StartLine, match.EndLine, match.Kind, match.Signature)
			continue
		}
		fmt.Printf("%s:%d:%d %s %s\n", match.File, match.StartLine, match.EndLine, match.Kind, match.Name)
	}
	return nil
}
