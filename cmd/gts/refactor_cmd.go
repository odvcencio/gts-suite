package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gts-suite/internal/query"
	"gts-suite/internal/refactor"
)

func runRefactor(args []string) error {
	flags := flag.NewFlagSet("gtsrefactor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-cache":          true,
		"--cache":         true,
		"-engine":         true,
		"--engine":        true,
		"-callsites":      false,
		"--callsites":     false,
		"-cross-package":  false,
		"--cross-package": false,
		"-write":          false,
		"--write":         false,
		"-json":           false,
		"--json":          false,
	})

	cachePath := flags.String("cache", "", "load index from cache instead of parsing")
	engine := flags.String("engine", "go", "refactor engine: go|treesitter")
	updateCallsites := flags.Bool("callsites", false, "update resolved same-package callsites")
	crossPackage := flags.Bool("cross-package", false, "update resolved cross-package callsites within the module")
	writeChanges := flags.Bool("write", false, "apply edits in-place (default is dry-run)")
	jsonOutput := flags.Bool("json", false, "emit JSON output")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 2 || flags.NArg() > 3 {
		return errors.New("usage: gtsrefactor <selector> <new-name> [path]")
	}
	if *crossPackage && !*updateCallsites {
		return errors.New("--cross-package requires --callsites")
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
		Write:                 *writeChanges,
		UpdateCallsites:       *updateCallsites,
		CrossPackageCallsites: *crossPackage,
		Engine:                *engine,
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
		"refactor: selector=%q new=%q engine=%q callsites=%t cross-package=%t matches=%d planned=%d (decl=%d callsites=%d) applied=%d files=%d\n",
		report.Selector,
		report.NewName,
		report.Engine,
		report.UpdateCallsites,
		report.CrossPackageCallsites,
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
