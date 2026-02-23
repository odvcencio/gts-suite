package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

func runRefs(args []string) error {
	flags := flag.NewFlagSet("gtsrefs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":  true,
		"--cache": true,
		"-regex":  false,
		"--regex": false,
		"-json":   false,
		"--json":  false,
		"-count":  false,
		"--count": false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	regexMode := flags.Bool("regex", false, "treat the first argument as a regular expression")
	jsonOutput := flags.Bool("json", false, "emit JSON output")
	countOnly := flags.Bool("count", false, "print the number of matches")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 || flags.NArg() > 2 {
		return errors.New("usage: gtsrefs <name|regex> [path]")
	}

	target := "."
	if flags.NArg() == 2 {
		target = flags.Arg(1)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	pattern := strings.TrimSpace(flags.Arg(0))
	if pattern == "" {
		return errors.New("reference matcher cannot be empty")
	}

	matchReference := func(name string) bool { return name == pattern }
	if *regexMode {
		compiled, compileErr := regexp.Compile(pattern)
		if compileErr != nil {
			return fmt.Errorf("compile regex: %w", compileErr)
		}
		matchReference = compiled.MatchString
	}

	matches := make([]referenceMatch, 0, idx.ReferenceCount())
	for _, file := range idx.Files {
		for _, reference := range file.References {
			if !matchReference(reference.Name) {
				continue
			}
			matches = append(matches, referenceMatch{
				File:        file.Path,
				Kind:        reference.Kind,
				Name:        reference.Name,
				StartLine:   reference.StartLine,
				EndLine:     reference.EndLine,
				StartColumn: reference.StartColumn,
				EndColumn:   reference.EndColumn,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				if matches[i].StartColumn == matches[j].StartColumn {
					return matches[i].Name < matches[j].Name
				}
				return matches[i].StartColumn < matches[j].StartColumn
			}
			return matches[i].StartLine < matches[j].StartLine
		}
		return matches[i].File < matches[j].File
	})

	if *jsonOutput {
		if *countOnly {
			return emitJSON(struct {
				Count int `json:"count"`
			}{Count: len(matches)})
		}
		return emitJSON(matches)
	}

	if *countOnly {
		fmt.Println(len(matches))
		return nil
	}
	for _, match := range matches {
		fmt.Printf("%s:%d:%d %s %s\n", match.File, match.StartLine, match.StartColumn, match.Kind, match.Name)
	}
	return nil
}
