package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gts-suite/internal/index"
	"gts-suite/internal/model"
	"gts-suite/internal/structdiff"
)

func runIndex(args []string) error {
	flags := flag.NewFlagSet("gtsindex", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-out":                  true,
		"--out":                 true,
		"-json":                 false,
		"--json":                false,
		"-incremental":          false,
		"--incremental":         false,
		"-watch":                false,
		"--watch":               false,
		"-subfile-incremental":  false,
		"--subfile-incremental": false,
		"-poll":                 false,
		"--poll":                false,
		"-report-changes":       false,
		"--report-changes":      false,
		"-once-if-changed":      false,
		"--once-if-changed":     false,
		"-interval":             true,
		"--interval":            true,
	})

	outPath := flags.String("out", ".gts/index.json", "output path for index cache")
	jsonOutput := flags.Bool("json", false, "emit index JSON to stdout")
	incremental := flags.Bool("incremental", true, "reuse unchanged files from previous index cache")
	watch := flags.Bool("watch", false, "watch for structural changes and rebuild continuously")
	subfileIncremental := flags.Bool("subfile-incremental", true, "reuse per-file parse trees for sub-file incremental updates in watch mode")
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

	fmt.Printf("watching: interval=%s target=%s subfile-incremental=%t\n", interval.String(), target, *subfileIncremental)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	watchState := index.NewWatchState()
	defer watchState.Release()

	current := idx
	onChange := func(changedPaths []string) {
		base := (*model.Index)(nil)
		if *incremental {
			base = current
		}

		var (
			next      *model.Index
			nextStats index.BuildStats
			err       error
		)
		useSubfile := *subfileIncremental && len(changedPaths) > 0
		if useSubfile {
			next, nextStats, err = builder.ApplyWatchChanges(current, changedPaths, watchState, index.WatchUpdateOptions{
				SubfileIncremental: true,
			})
		} else {
			next, nextStats, err = buildOnce(base)
			if *subfileIncremental {
				watchState.Clear()
			}
		}
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
			onChange(nil)
		}
	}
}
