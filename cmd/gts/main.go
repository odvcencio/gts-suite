package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/odvcencio/fluffyui/keybind"

	"gts-suite/internal/bridge"
	"gts-suite/internal/chunk"
	"gts-suite/internal/contextpack"
	"gts-suite/internal/deps"
	"gts-suite/internal/files"
	"gts-suite/internal/index"
	"gts-suite/internal/lint"
	"gts-suite/internal/model"
	"gts-suite/internal/query"
	"gts-suite/internal/refactor"
	gtsscope "gts-suite/internal/scope"
	"gts-suite/internal/stats"
	"gts-suite/internal/structdiff"
)

func main() {
	if err := newCLI().Run(os.Args[1:]); err != nil {
		exitCode := 1
		if withCode, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = withCode.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCode)
	}
}

type grepMatch struct {
	File      string `json:"file"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	if e.err == nil {
		return "command failed"
	}
	return e.err.Error()
}

func (e exitCodeError) ExitCode() int {
	if e.code <= 0 {
		return 1
	}
	return e.code
}

type commandSpec struct {
	ID      string
	Aliases []string
	Summary string
	Usage   string
	Run     func(args []string) error
}

type cli struct {
	registry   *keybind.CommandRegistry
	specs      map[string]commandSpec
	aliasToID  map[string]string
	invokeArgs []string
	invokeErr  error
}

func newCLI() *cli {
	c := &cli{
		registry:  keybind.NewRegistry(),
		specs:     make(map[string]commandSpec),
		aliasToID: make(map[string]string),
	}

	commands := []commandSpec{
		{
			ID:      "gtsindex",
			Aliases: []string{"index"},
			Summary: "Build a structural index and optionally cache it",
			Usage:   "gtsindex [path] [--out .gts/index.json] [--incremental] [--watch] [--poll] [--interval 2s] [--report-changes] [--once-if-changed] [--json]",
			Run:     runIndex,
		},
		{
			ID:      "gtsmap",
			Aliases: []string{"map"},
			Summary: "Print structural summaries for indexed files",
			Usage:   "gtsmap [path] [--cache .gts/index.json] [--json]",
			Run:     runMap,
		},
		{
			ID:      "gtsfiles",
			Aliases: []string{"files"},
			Summary: "List/index files with structural density filters",
			Usage:   "gtsfiles [path] [--cache .gts/index.json] [--language go] [--min-symbols 0] [--sort symbols|imports|size|path] [--top 50] [--json]",
			Run:     runFiles,
		},
		{
			ID:      "gtsstats",
			Aliases: []string{"stats"},
			Summary: "Report structural codebase metrics from an index",
			Usage:   "gtsstats [path] [--cache .gts/index.json] [--top 10] [--json]",
			Run:     runStats,
		},
		{
			ID:      "gtsdeps",
			Aliases: []string{"deps"},
			Summary: "Analyze dependency graph from structural imports",
			Usage:   "gtsdeps [path] [--cache .gts/index.json] [--by package|file] [--top 10] [--focus node] [--depth N] [--reverse] [--edges] [--json]",
			Run:     runDeps,
		},
		{
			ID:      "gtsbridge",
			Aliases: []string{"bridge"},
			Summary: "Map cross-component dependency bridges",
			Usage:   "gtsbridge [path] [--cache .gts/index.json] [--top 20] [--json]",
			Run:     runBridge,
		},
		{
			ID:      "gtsgrep",
			Aliases: []string{"grep"},
			Summary: "Structural grep over indexed symbols",
			Usage:   "gtsgrep <selector> [path] [--cache .gts/index.json] [--count] [--json]",
			Run:     runGrep,
		},
		{
			ID:      "gtsdiff",
			Aliases: []string{"diff"},
			Summary: "Structural diff between two snapshots",
			Usage:   "gtsdiff [before-path] [after-path] [--before-cache file] [--after-cache file] [--json]",
			Run:     runDiff,
		},
		{
			ID:      "gtsrefactor",
			Aliases: []string{"refactor"},
			Summary: "Apply structural declaration renames (dry-run by default)",
			Usage:   "gtsrefactor <selector> <new-name> [path] [--cache file] [--callsites] [--write] [--json]",
			Run:     runRefactor,
		},
		{
			ID:      "gtschunk",
			Aliases: []string{"chunk"},
			Summary: "Split code into AST-boundary chunks for RAG/indexing",
			Usage:   "gtschunk [path] [--cache file] [--tokens N] [--json]",
			Run:     runChunk,
		},
		{
			ID:      "gtsscope",
			Aliases: []string{"scope"},
			Summary: "Resolve symbols in scope for a file and line",
			Usage:   "gtsscope <file> [--line N] [--root .] [--cache file] [--json]",
			Run:     runScope,
		},
		{
			ID:      "gtscontext",
			Aliases: []string{"context"},
			Summary: "Pack focused code context for a file and line",
			Usage:   "gtscontext <file> [--line N] [--tokens N] [--root .] [--cache file] [--json]",
			Run:     runContext,
		},
		{
			ID:      "gtslint",
			Aliases: []string{"lint"},
			Summary: "Run structural lint rules against indexed symbols",
			Usage:   "gtslint [path] [--cache .gts/index.json] --rule 'no function longer than 50 lines' [--rule ...] [--fail-on-violations] [--json]",
			Run:     runLint,
		},
	}

	for _, spec := range commands {
		specCopy := spec
		c.specs[specCopy.ID] = specCopy
		c.aliasToID[specCopy.ID] = specCopy.ID
		for _, alias := range specCopy.Aliases {
			c.aliasToID[strings.ToLower(alias)] = specCopy.ID
		}

		commandID := specCopy.ID
		c.registry.Register(keybind.Command{
			ID:          commandID,
			Title:       specCopy.ID,
			Description: specCopy.Summary,
			Handler: func(ctx keybind.Context) {
				c.invokeErr = c.specs[commandID].Run(c.invokeArgs)
			},
		})
	}

	return c
}

func (c *cli) Run(args []string) error {
	if len(args) == 0 {
		c.printHelp()
		return nil
	}

	name := strings.ToLower(strings.TrimSpace(args[0]))
	if name == "-h" || name == "--help" {
		c.printHelp()
		return nil
	}
	if name == "help" {
		if len(args) == 1 {
			c.printHelp()
			return nil
		}
		id, ok := c.aliasToID[strings.ToLower(strings.TrimSpace(args[1]))]
		if !ok {
			return fmt.Errorf("unknown command %q", args[1])
		}
		c.printCommandHelp(id)
		return nil
	}

	commandID, ok := c.aliasToID[name]
	if !ok {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if len(args) > 1 {
		firstArg := strings.TrimSpace(args[1])
		if firstArg == "-h" || firstArg == "--help" {
			c.printCommandHelp(commandID)
			return nil
		}
	}

	c.invokeArgs = args[1:]
	c.invokeErr = nil

	if ok := c.registry.Execute(commandID, keybind.Context{}); !ok {
		return fmt.Errorf("command %q is not executable", commandID)
	}
	return c.invokeErr
}

func (c *cli) printHelp() {
	ids := make([]string, 0, len(c.specs))
	for id := range c.specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Println("gts-suite CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gts <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, id := range ids {
		spec := c.specs[id]
		fmt.Printf("  %-10s %s\n", spec.ID, spec.Summary)
	}
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gts gtsindex . --out .gts/index.json")
	fmt.Println("  gts gtsindex . --out .gts/index.json --once-if-changed")
	fmt.Println("  gts gtsindex . --watch --interval 2s")
	fmt.Println("  gts gtsmap . --json")
	fmt.Println("  gts gtsfiles . --sort symbols --top 20")
	fmt.Println("  gts gtsstats . --top 15")
	fmt.Println("  gts gtsdeps . --by package --focus internal/query --depth 2 --reverse")
	fmt.Println("  gts gtsbridge . --top 20")
	fmt.Println("  gts gtsgrep 'function_definition[name=/^Test/]' .")
	fmt.Println("  gts gtsdiff --before-cache before.json --after-cache after.json")
	fmt.Println("  gts gtsrefactor 'function_definition[name=/^OldName$/]' NewName . --callsites --write")
	fmt.Println("  gts gtschunk . --tokens 500 --json")
	fmt.Println("  gts gtsscope cmd/gts/main.go --line 300")
	fmt.Println("  gts gtscontext cmd/gts/main.go --line 120 --tokens 600")
	fmt.Println("  gts gtslint . --rule 'no function longer than 50 lines'")
	fmt.Println("  gts help gtsgrep")
}

func (c *cli) printCommandHelp(id string) {
	spec, ok := c.specs[id]
	if !ok {
		return
	}

	fmt.Printf("%s\n", spec.ID)
	fmt.Println()
	fmt.Printf("Summary: %s\n", spec.Summary)
	fmt.Printf("Usage:   gts %s\n", spec.Usage)
	if len(spec.Aliases) > 0 {
		fmt.Printf("Aliases: %s\n", strings.Join(spec.Aliases, ", "))
	}
}

func runIndex(args []string) error {
	flags := flag.NewFlagSet("gtsindex", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-out":              true,
		"--out":             true,
		"-json":             false,
		"--json":            false,
		"-incremental":      false,
		"--incremental":     false,
		"-watch":            false,
		"--watch":           false,
		"-poll":             false,
		"--poll":            false,
		"-report-changes":   false,
		"--report-changes":  false,
		"-once-if-changed":  false,
		"--once-if-changed": false,
		"-interval":         true,
		"--interval":        true,
	})

	outPath := flags.String("out", ".gts/index.json", "output path for index cache")
	jsonOutput := flags.Bool("json", false, "emit index JSON to stdout")
	incremental := flags.Bool("incremental", true, "reuse unchanged files from previous index cache")
	watch := flags.Bool("watch", false, "watch for structural changes and rebuild continuously")
	poll := flags.Bool("poll", false, "force polling watch mode instead of fsnotify")
	reportChanges := flags.Bool("report-changes", false, "print grouped structural change summary against previous cache")
	onceIfChanged := flags.Bool("once-if-changed", false, "exit with code 2 when structural changes are detected")
	interval := flags.Duration("interval", 2*time.Second, "poll interval for watch mode")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsindex accepts at most one path")
	}
	if *watch && *interval <= 0 {
		return fmt.Errorf("interval must be > 0 in watch mode")
	}
	if *watch && *onceIfChanged {
		return fmt.Errorf("--once-if-changed cannot be used with --watch")
	}
	if *onceIfChanged && strings.TrimSpace(*outPath) == "" {
		return fmt.Errorf("--once-if-changed requires --out to provide a baseline cache path")
	}
	if *onceIfChanged {
		*reportChanges = true
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	builder := index.NewBuilder()
	var previous *model.Index
	hasBaseline := false
	if strings.TrimSpace(*outPath) != "" {
		cached, err := index.Load(*outPath)
		switch {
		case err == nil:
			previous = cached
			hasBaseline = true
		case os.IsNotExist(err):
		default:
			return fmt.Errorf("load cache %s: %w", *outPath, err)
		}
	}

	buildOnce := func(base *model.Index) (*model.Index, index.BuildStats, error) {
		if *incremental {
			return builder.BuildPathIncremental(target, base)
		}
		idx, err := builder.BuildPath(target)
		return idx, index.BuildStats{}, err
	}

	buildBase := (*model.Index)(nil)
	if *incremental {
		buildBase = previous
	}

	idx, stats, err := buildOnce(buildBase)
	if err != nil {
		return err
	}

	report := structdiff.Report{}
	changed := true
	if hasBaseline {
		report = structdiff.Compare(previous, idx)
		changed = report.Stats.ChangedFiles > 0 || !parseErrorsEqual(previous.Errors, idx.Errors)
	}

	if strings.TrimSpace(*outPath) != "" && (!*onceIfChanged || changed || !hasBaseline) {
		if err := index.Save(*outPath, idx); err != nil {
			return err
		}
	}

	if *jsonOutput {
		if err := emitJSON(idx); err != nil {
			return err
		}
	}

	if !*jsonOutput {
		printIndexSummary(idx, stats, *incremental)
		if strings.TrimSpace(*outPath) != "" {
			fmt.Printf("cache: %s\n", *outPath)
		}
		if *reportChanges {
			printChangeReport(report, hasBaseline)
		}
	}

	if *onceIfChanged {
		if changed {
			return exitCodeError{
				code: 2,
				err:  errors.New("structural changes detected"),
			}
		}
		if !*jsonOutput {
			fmt.Println("once-if-changed: no structural changes")
		}
		return nil
	}

	if !*watch {
		return nil
	}

	fmt.Printf("watching: interval=%s target=%s\n", interval.String(), target)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	current := idx
	onChange := func() {
		base := (*model.Index)(nil)
		if *incremental {
			base = current
		}

		next, nextStats, err := buildOnce(base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "watch build error: %v\n", err)
			return
		}

		report := structdiff.Compare(current, next)
		changed := report.Stats.ChangedFiles > 0 || !parseErrorsEqual(current.Errors, next.Errors)
		if !changed {
			return
		}

		current = next
		if strings.TrimSpace(*outPath) != "" {
			if err := index.Save(*outPath, next); err != nil {
				fmt.Fprintf(os.Stderr, "watch save error: %v\n", err)
			}
		}

		if *jsonOutput {
			if err := emitJSON(next); err != nil {
				fmt.Fprintf(os.Stderr, "watch json error: %v\n", err)
			}
			return
		}

		fmt.Printf("watch: changed files=%d symbols=+%d -%d ~%d\n",
			report.Stats.ChangedFiles,
			report.Stats.AddedSymbols,
			report.Stats.RemovedSymbols,
			report.Stats.ModifiedSymbols)
		printIndexSummary(next, nextStats, *incremental)
		if *reportChanges {
			printChangeReport(report, true)
		}
	}

	ignorePaths := map[string]bool{}
	if strings.TrimSpace(*outPath) != "" {
		if absOut, err := filepath.Abs(*outPath); err == nil {
			ignorePaths[filepath.Clean(absOut)] = true
		}
	}

	if !*poll {
		if err := watchWithFSNotify(ctx, target, *interval, ignorePaths, onChange); err == nil {
			fmt.Println("watch: stopped")
			return nil
		} else {
			fmt.Fprintf(os.Stderr, "watch backend fallback to polling: %v\n", err)
		}
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("watch: stopped")
			return nil
		case <-ticker.C:
			onChange()
		}
	}
}

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

func runDeps(args []string) error {
	flags := flag.NewFlagSet("gtsdeps", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":    true,
		"--cache":   true,
		"-by":       true,
		"--by":      true,
		"-top":      true,
		"--top":     true,
		"-focus":    true,
		"--focus":   true,
		"-depth":    true,
		"--depth":   true,
		"-reverse":  false,
		"--reverse": false,
		"-edges":    false,
		"--edges":   false,
		"-json":     false,
		"--json":    false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	by := flags.String("by", "package", "graph mode: package or file")
	top := flags.Int("top", 10, "number of top nodes to show")
	focus := flags.String("focus", "", "focus node to inspect incoming/outgoing edges")
	depth := flags.Int("depth", 1, "transitive depth for focus traversal")
	reverse := flags.Bool("reverse", false, "walk reverse dependencies from focus")
	includeEdges := flags.Bool("edges", false, "include full edge list in output")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsdeps accepts at most one path")
	}
	if *top <= 0 {
		return fmt.Errorf("top must be > 0")
	}
	if *depth <= 0 {
		return fmt.Errorf("depth must be > 0")
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	report, err := deps.Build(idx, deps.Options{
		Mode:         *by,
		Top:          *top,
		Focus:        *focus,
		Depth:        *depth,
		Reverse:      *reverse,
		IncludeEdges: *includeEdges || *jsonOutput,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf(
		"deps: mode=%s nodes=%d edges=%d internal=%d external=%d module=%s\n",
		report.Mode,
		report.NodeCount,
		report.EdgeCount,
		report.InternalEdgeCount,
		report.ExternalEdgeCount,
		report.Module,
	)

	if len(report.TopOutgoing) > 0 {
		fmt.Printf("top outgoing (limit=%d):\n", *top)
		for _, item := range report.TopOutgoing {
			fmt.Printf("  %s out=%d in=%d project=%t\n", item.Node, item.Outgoing, item.Incoming, item.IsProject)
		}
	}

	if len(report.TopIncoming) > 0 {
		fmt.Printf("top incoming (limit=%d):\n", *top)
		for _, item := range report.TopIncoming {
			fmt.Printf("  %s in=%d out=%d project=%t\n", item.Node, item.Incoming, item.Outgoing, item.IsProject)
		}
	}

	if report.Focus != "" {
		fmt.Printf("focus: %s direction=%s depth=%d\n", report.Focus, report.FocusDirection, report.FocusDepth)
		if len(report.FocusOutgoing) > 0 {
			fmt.Printf("  outgoing: %s\n", strings.Join(report.FocusOutgoing, ", "))
		}
		if len(report.FocusIncoming) > 0 {
			fmt.Printf("  incoming: %s\n", strings.Join(report.FocusIncoming, ", "))
		}
		if len(report.FocusWalk) > 0 {
			fmt.Printf("  walk: %s\n", strings.Join(report.FocusWalk, ", "))
		}
	}

	if *includeEdges {
		fmt.Println("edges:")
		for _, edge := range report.Edges {
			label := "external"
			if edge.Internal {
				label = "internal"
			}
			fmt.Printf("  %s -> %s (%s)\n", edge.From, edge.To, label)
		}
	}

	return nil
}

func runBridge(args []string) error {
	flags := flag.NewFlagSet("gtsbridge", flag.ContinueOnError)
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
	top := flags.Int("top", 20, "number of top bridge and external rows to show")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtsbridge accepts at most one path")
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

	report, err := bridge.Build(idx, bridge.Options{
		Top: *top,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	fmt.Printf(
		"bridge: components=%d packages=%d bridges=%d root=%s module=%s\n",
		report.ComponentCount,
		report.PackageCount,
		report.BridgeCount,
		report.Root,
		report.Module,
	)
	if len(report.Components) > 0 {
		fmt.Println("components:")
		for _, component := range report.Components {
			fmt.Printf(
				"  %s packages=%d files=%d imports:internal=%d external=%d\n",
				component.Name,
				component.PackageCount,
				component.FileCount,
				component.InternalImports,
				component.ExternalImports,
			)
		}
	}
	if len(report.TopBridges) > 0 {
		fmt.Printf("top bridges (limit=%d):\n", *top)
		for _, edge := range report.TopBridges {
			line := fmt.Sprintf("  %s -> %s count=%d", edge.From, edge.To, edge.Count)
			if len(edge.Samples) > 0 {
				line += " samples=" + strings.Join(edge.Samples, ",")
			}
			fmt.Println(line)
		}
	}
	if len(report.ExternalByComponent) > 0 {
		fmt.Printf("external pressure (limit=%d):\n", *top)
		for _, item := range report.ExternalByComponent {
			line := fmt.Sprintf("  %s count=%d", item.Component, item.Count)
			if len(item.TopImports) > 0 {
				line += " top=" + strings.Join(item.TopImports, ",")
			}
			fmt.Println(line)
		}
	}
	return nil
}

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

func runRefactor(args []string) error {
	flags := flag.NewFlagSet("gtsrefactor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":      true,
		"--cache":     true,
		"-callsites":  false,
		"--callsites": false,
		"-write":      false,
		"--write":     false,
		"-json":       false,
		"--json":      false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	updateCallsites := flags.Bool("callsites", false, "update resolved same-package callsites")
	writeChanges := flags.Bool("write", false, "apply edits in-place (default is dry-run)")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 2 || flags.NArg() > 3 {
		return errors.New("usage: gtsrefactor <selector> <new-name> [path]")
	}

	selector, err := query.ParseSelector(flags.Arg(0))
	if err != nil {
		return err
	}
	newName := flags.Arg(1)

	target := "."
	if flags.NArg() == 3 {
		target = flags.Arg(2)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	report, err := refactor.RenameDeclarations(idx, selector, newName, refactor.Options{
		Write:           *writeChanges,
		UpdateCallsites: *updateCallsites,
	})
	if err != nil {
		return err
	}

	if *jsonOutput {
		return emitJSON(report)
	}

	for _, edit := range report.Edits {
		if edit.Skipped {
			fmt.Printf(
				"%s:%d:%d %s %s %s -> %s skipped=%s\n",
				edit.File,
				edit.Line,
				edit.Column,
				edit.Category,
				edit.Kind,
				edit.OldName,
				edit.NewName,
				edit.SkipNote,
			)
			continue
		}
		status := "planned"
		if edit.Applied {
			status = "applied"
		}
		fmt.Printf("%s:%d:%d %s %s %s -> %s %s\n", edit.File, edit.Line, edit.Column, edit.Category, edit.Kind, edit.OldName, edit.NewName, status)
	}
	fmt.Printf(
		"refactor: selector=%q new=%q callsites=%t matches=%d planned=%d (decl=%d callsites=%d) applied=%d files=%d\n",
		report.Selector,
		report.NewName,
		report.UpdateCallsites,
		report.MatchCount,
		report.PlannedEdits,
		report.PlannedDeclEdits,
		report.PlannedUseEdits,
		report.AppliedEdits,
		report.ChangedFiles,
	)
	if !report.Write {
		fmt.Println("refactor: dry-run (add --write to apply edits)")
	}

	return nil
}

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

func runContext(args []string) error {
	flags := flag.NewFlagSet("gtscontext", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":   true,
		"--cache":  true,
		"-root":    true,
		"--root":   true,
		"-line":    true,
		"--line":   true,
		"-tokens":  true,
		"--tokens": true,
		"-json":    false,
		"--json":   false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	rootPath := flags.String("root", ".", "parse root path when cache is not provided")
	line := flags.Int("line", 1, "cursor line (1-based)")
	tokens := flags.Int("tokens", 800, "token budget")
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
		FilePath:    filePath,
		Line:        *line,
		TokenBudget: *tokens,
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

func runLint(args []string) error {
	flags := flag.NewFlagSet("gtslint", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":               true,
		"--cache":              true,
		"-rule":                true,
		"--rule":               true,
		"-fail-on-violations":  false,
		"--fail-on-violations": false,
		"-json":                false,
		"--json":               false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	failOnViolations := flags.Bool("fail-on-violations", true, "exit non-zero when violations are found")
	jsonOutput := flags.Bool("json", false, "emit JSON output")
	var rawRules stringList
	flags.Var(&rawRules, "rule", "lint rule expression (repeatable)")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("gtslint accepts at most one path")
	}
	if len(rawRules) == 0 {
		return errors.New("at least one --rule is required")
	}

	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}

	rules := make([]lint.Rule, 0, len(rawRules))
	for _, rawRule := range rawRules {
		rule, err := lint.ParseRule(rawRule)
		if err != nil {
			return fmt.Errorf("parse rule %q: %w", rawRule, err)
		}
		rules = append(rules, rule)
	}

	idx, err := loadOrBuild(*cachePath, target)
	if err != nil {
		return err
	}

	violations := lint.Evaluate(idx, rules)

	if *jsonOutput {
		return emitJSON(struct {
			Rules      []lint.Rule      `json:"rules"`
			Violations []lint.Violation `json:"violations,omitempty"`
			Count      int              `json:"count"`
		}{
			Rules:      rules,
			Violations: violations,
			Count:      len(violations),
		})
	}

	for _, violation := range violations {
		if violation.StartLine <= 0 {
			fmt.Printf(
				"%s %s %s rule=%s %s\n",
				violation.File,
				violation.Kind,
				violation.Name,
				violation.RuleID,
				violation.Message,
			)
			continue
		}
		fmt.Printf(
			"%s:%d:%d %s %s rule=%s %s\n",
			violation.File,
			violation.StartLine,
			violation.EndLine,
			violation.Kind,
			violation.Name,
			violation.RuleID,
			violation.Message,
		)
	}
	fmt.Printf("lint: rules=%d violations=%d\n", len(rules), len(violations))
	if len(idx.Errors) > 0 {
		fmt.Printf("lint: parse errors=%d (ignored)\n", len(idx.Errors))
	}

	if len(violations) > 0 && *failOnViolations {
		return exitCodeError{
			code: 3,
			err:  fmt.Errorf("%d lint violations", len(violations)),
		}
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

func symbolLabel(name, signature string) string {
	if strings.TrimSpace(signature) != "" {
		return signature
	}
	return name
}

func printIndexSummary(idx *model.Index, stats index.BuildStats, incremental bool) {
	if incremental {
		fmt.Printf(
			"indexed: files=%d symbols=%d errors=%d root=%s parsed=%d reused=%d\n",
			idx.FileCount(),
			idx.SymbolCount(),
			len(idx.Errors),
			idx.Root,
			stats.ParsedFiles,
			stats.ReusedFiles,
		)
		return
	}

	fmt.Printf("indexed: files=%d symbols=%d errors=%d root=%s\n", idx.FileCount(), idx.SymbolCount(), len(idx.Errors), idx.Root)
}

func parseErrorsEqual(left, right []model.ParseError) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Path != right[i].Path || left[i].Error != right[i].Error {
			return false
		}
	}
	return true
}

type fileChangeSummary struct {
	File          string
	Added         int
	Removed       int
	Modified      int
	ImportAdded   int
	ImportRemoved int
}

func printChangeReport(report structdiff.Report, hasBaseline bool) {
	if !hasBaseline {
		fmt.Println("changes: baseline cache not found; treating current index as changed")
		return
	}

	totalImportAdded := 0
	totalImportRemoved := 0
	for _, item := range report.ImportChanges {
		totalImportAdded += len(item.Added)
		totalImportRemoved += len(item.Removed)
	}

	fmt.Printf(
		"changes: files=%d symbols=+%d -%d ~%d imports=+%d -%d\n",
		report.Stats.ChangedFiles,
		report.Stats.AddedSymbols,
		report.Stats.RemovedSymbols,
		report.Stats.ModifiedSymbols,
		totalImportAdded,
		totalImportRemoved,
	)

	summaries := summarizeChangesByFile(report)
	for _, summary := range summaries {
		parts := make([]string, 0, 4)
		if summary.Added > 0 {
			parts = append(parts, fmt.Sprintf("+%d", summary.Added))
		}
		if summary.Removed > 0 {
			parts = append(parts, fmt.Sprintf("-%d", summary.Removed))
		}
		if summary.Modified > 0 {
			parts = append(parts, fmt.Sprintf("~%d", summary.Modified))
		}
		if summary.ImportAdded > 0 || summary.ImportRemoved > 0 {
			parts = append(parts, fmt.Sprintf("imports:+%d -%d", summary.ImportAdded, summary.ImportRemoved))
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Printf("  %s %s\n", summary.File, strings.Join(parts, " "))
	}
}

func summarizeChangesByFile(report structdiff.Report) []fileChangeSummary {
	byFile := map[string]*fileChangeSummary{}
	ensure := func(path string) *fileChangeSummary {
		if existing, ok := byFile[path]; ok {
			return existing
		}
		created := &fileChangeSummary{File: path}
		byFile[path] = created
		return created
	}

	for _, item := range report.AddedSymbols {
		ensure(item.File).Added++
	}
	for _, item := range report.RemovedSymbols {
		ensure(item.File).Removed++
	}
	for _, item := range report.ModifiedSymbols {
		ensure(item.After.File).Modified++
	}
	for _, item := range report.ImportChanges {
		summary := ensure(item.File)
		summary.ImportAdded += len(item.Added)
		summary.ImportRemoved += len(item.Removed)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	out := make([]fileChangeSummary, 0, len(files))
	for _, file := range files {
		out = append(out, *byFile[file])
	}
	return out
}

func watchWithFSNotify(ctx context.Context, target string, debounce time.Duration, ignorePaths map[string]bool, onChange func()) error {
	roots, err := watchRoots(target)
	if err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	for _, root := range roots {
		if err := addWatchRecursive(watcher, root); err != nil {
			return err
		}
	}

	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	pending := false

	resetDebounce := func() {
		if pending {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer.Reset(debounce)
		pending = true
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			eventPath := filepath.Clean(event.Name)
			if shouldIgnoreWatchPath(eventPath, ignorePaths) {
				continue
			}

			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(eventPath); statErr == nil && info.IsDir() {
					_ = addWatchRecursive(watcher, eventPath)
				}
			}

			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			resetDebounce()
		case <-timer.C:
			if pending {
				pending = false
				onChange()
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return watchErr
		}
	}
}

func watchRoots(target string) ([]string, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	absTarget = filepath.Clean(absTarget)

	info, err := os.Stat(absTarget)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return []string{absTarget}, nil
	}
	return []string{filepath.Dir(absTarget)}, nil
}

func addWatchRecursive(watcher *fsnotify.Watcher, root string) error {
	root = filepath.Clean(root)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if shouldSkipWatchDir(root, path, entry.Name()) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func shouldSkipWatchDir(root, path, name string) bool {
	if path == root {
		return false
	}

	if name == ".git" || name == ".hg" || name == ".svn" || name == "node_modules" || name == "vendor" {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return false
}

func shouldIgnoreWatchPath(path string, ignorePaths map[string]bool) bool {
	if ignorePaths[path] {
		return true
	}

	base := filepath.Base(path)
	if base == ".DS_Store" || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".swx") || strings.HasPrefix(base, ".#") {
		return true
	}
	return false
}

func normalizeFlagArgs(args []string, valueFlags map[string]bool) []string {
	if len(args) == 0 {
		return nil
	}

	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}

		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		if !valueFlags[arg] {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}

	return append(flags, positionals...)
}

type stringList []string

func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("value cannot be empty")
	}
	*s = append(*s, trimmed)
	return nil
}

func loadOrBuild(cachePath string, target string) (*model.Index, error) {
	if strings.TrimSpace(cachePath) != "" {
		return index.Load(cachePath)
	}
	return index.NewBuilder().BuildPath(target)
}

func emitJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
