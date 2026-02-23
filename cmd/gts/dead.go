package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gts-suite/internal/xref"
)

func runDead(args []string) error {
	flags := flag.NewFlagSet("gtsdead", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":                true,
		"--cache":               true,
		"-kind":                 true,
		"--kind":                true,
		"-include-entrypoints":  false,
		"--include-entrypoints": false,
		"-include-tests":        false,
		"--include-tests":       false,
		"-json":                 false,
		"--json":                false,
		"-count":                false,
		"--count":               false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	kind := flags.String("kind", "callable", "filter dead definitions by callable|function|method")
	includeEntrypoints := flags.Bool("include-entrypoints", false, "include main/init functions in dead code results")
	includeTests := flags.Bool("include-tests", false, "include _test files in dead code results")
	jsonOutput := flags.Bool("json", false, "emit JSON output")
	countOnly := flags.Bool("count", false, "print the number of dead definitions")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("usage: gtsdead [path]")
	}

	mode := strings.ToLower(strings.TrimSpace(*kind))
	switch mode {
	case "callable", "function", "method":
	default:
		return fmt.Errorf("unsupported --kind %q (expected callable|function|method)", *kind)
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	graph, err := xref.Build(idx)
	if err != nil {
		return err
	}

	matches := make([]deadMatch, 0, 64)
	scanned := 0
	for _, definition := range graph.Definitions {
		if !deadKindAllowed(definition, mode) {
			continue
		}
		if !*includeEntrypoints && isEntrypointDefinition(definition) {
			continue
		}
		if !*includeTests && isTestSourceFile(definition.File) {
			continue
		}

		scanned++
		incoming := graph.IncomingCount(definition.ID)
		if incoming > 0 {
			continue
		}
		matches = append(matches, deadMatch{
			File:      definition.File,
			Package:   definition.Package,
			Kind:      definition.Kind,
			Name:      definition.Name,
			Signature: definition.Signature,
			StartLine: definition.StartLine,
			EndLine:   definition.EndLine,
			Incoming:  incoming,
			Outgoing:  graph.OutgoingCount(definition.ID),
		})
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
				Count   int `json:"count"`
				Scanned int `json:"scanned"`
			}{
				Count:   len(matches),
				Scanned: scanned,
			})
		}
		return emitJSON(struct {
			Kind    string      `json:"kind"`
			Scanned int         `json:"scanned"`
			Count   int         `json:"count"`
			Matches []deadMatch `json:"matches,omitempty"`
		}{
			Kind:    mode,
			Scanned: scanned,
			Count:   len(matches),
			Matches: matches,
		})
	}

	if *countOnly {
		fmt.Println(len(matches))
		return nil
	}

	for _, match := range matches {
		name := strings.TrimSpace(match.Signature)
		if name == "" {
			name = match.Name
		}
		fmt.Printf(
			"%s:%d:%d %s %s incoming=%d outgoing=%d\n",
			match.File,
			match.StartLine,
			match.EndLine,
			match.Kind,
			name,
			match.Incoming,
			match.Outgoing,
		)
	}
	fmt.Printf("dead: kind=%s scanned=%d matches=%d\n", mode, scanned, len(matches))
	return nil
}

func deadKindAllowed(definition xref.Definition, mode string) bool {
	switch mode {
	case "callable":
		return definition.Callable
	case "function":
		return definition.Kind == "function_definition"
	case "method":
		return definition.Kind == "method_definition"
	default:
		return false
	}
}

func isEntrypointDefinition(definition xref.Definition) bool {
	if definition.Kind != "function_definition" {
		return false
	}
	return definition.Name == "main" || definition.Name == "init"
}

func isTestSourceFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), "_test.go")
}
