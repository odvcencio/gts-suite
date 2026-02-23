package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gts-suite/internal/structdiff"
)

func TestNewCLI_HasCoreCommandsAndAliases(t *testing.T) {
	app := newCLI()

	for _, id := range []string{"gtsindex", "gtsmap", "gtsfiles", "gtsstats", "gtsdeps", "gtsbridge", "gtsgrep", "gtsdiff", "gtsrefactor", "gtschunk", "gtsscope", "gtscontext", "gtslint"} {
		if _, ok := app.specs[id]; !ok {
			t.Fatalf("missing command spec for %q", id)
		}
		if mapped, ok := app.aliasToID[id]; !ok || mapped != id {
			t.Fatalf("missing canonical alias for %q", id)
		}
	}

	for alias, id := range map[string]string{
		"index":    "gtsindex",
		"map":      "gtsmap",
		"files":    "gtsfiles",
		"stats":    "gtsstats",
		"deps":     "gtsdeps",
		"bridge":   "gtsbridge",
		"grep":     "gtsgrep",
		"diff":     "gtsdiff",
		"refactor": "gtsrefactor",
		"chunk":    "gtschunk",
		"scope":    "gtsscope",
		"context":  "gtscontext",
		"lint":     "gtslint",
	} {
		if mapped, ok := app.aliasToID[alias]; !ok || mapped != id {
			t.Fatalf("alias %q mapped to %q (ok=%v), want %q", alias, mapped, ok, id)
		}
	}
}

func TestCLI_RunUnknownCommand(t *testing.T) {
	app := newCLI()
	if err := app.Run([]string{"unknown-command"}); err == nil {
		t.Fatal("expected unknown command to return error")
	}
}

func TestCLI_HelpSubcommand(t *testing.T) {
	app := newCLI()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := app.Run([]string{"help", "gtsgrep"})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Usage:   gts gtsgrep") {
		t.Fatalf("expected command usage in help output, got %q", text)
	}
}

func TestNormalizeFlagArgs_ReordersInterspersedFlags(t *testing.T) {
	args := []string{"function_definition[name=/^Test/]", "--cache", ".gts/index.json", "--json"}
	got := normalizeFlagArgs(args, map[string]bool{
		"--cache": true,
		"--json":  false,
	})
	want := []string{"--cache", ".gts/index.json", "--json", "function_definition[name=/^Test/]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeFlagArgs mismatch\nwant=%v\ngot=%v", want, got)
	}
}

func TestResolveDiffSources_Positional(t *testing.T) {
	before, after, err := resolveDiffSources([]string{"old", "new"}, "", "")
	if err != nil {
		t.Fatalf("resolveDiffSources returned error: %v", err)
	}
	if before != "old" || after != "new" {
		t.Fatalf("unexpected targets before=%q after=%q", before, after)
	}
}

func TestResolveDiffSources_CacheAndPositionalMix(t *testing.T) {
	before, after, err := resolveDiffSources([]string{"new"}, "before.json", "")
	if err != nil {
		t.Fatalf("resolveDiffSources returned error: %v", err)
	}
	if before != "" || after != "new" {
		t.Fatalf("unexpected targets before=%q after=%q", before, after)
	}
}

func TestResolveDiffSources_MissingInputs(t *testing.T) {
	if _, _, err := resolveDiffSources([]string{}, "", ""); err == nil {
		t.Fatal("expected error for missing before/after inputs")
	}
	if _, _, err := resolveDiffSources([]string{}, "before.json", ""); err == nil {
		t.Fatal("expected error when after source is missing")
	}
}

func TestShouldSkipWatchDir(t *testing.T) {
	root := filepath.Clean("/tmp/repo")
	cases := []struct {
		path string
		name string
		want bool
	}{
		{path: root, name: "repo", want: false},
		{path: filepath.Join(root, ".git"), name: ".git", want: true},
		{path: filepath.Join(root, "vendor"), name: "vendor", want: true},
		{path: filepath.Join(root, ".hidden"), name: ".hidden", want: true},
		{path: filepath.Join(root, "src"), name: "src", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipWatchDir(root, tc.path, tc.name)
		if got != tc.want {
			t.Fatalf("shouldSkipWatchDir(%q,%q)=%v want=%v", tc.path, tc.name, got, tc.want)
		}
	}
}

func TestShouldIgnoreWatchPath(t *testing.T) {
	ignored := map[string]bool{
		filepath.Clean("/tmp/repo/.gts/index.json"): true,
	}

	if !shouldIgnoreWatchPath(filepath.Clean("/tmp/repo/.gts/index.json"), ignored) {
		t.Fatal("expected explicit ignored path to be ignored")
	}
	if !shouldIgnoreWatchPath(filepath.Clean("/tmp/repo/.#file.go"), ignored) {
		t.Fatal("expected editor lockfile to be ignored")
	}
	if shouldIgnoreWatchPath(filepath.Clean("/tmp/repo/main.go"), ignored) {
		t.Fatal("did not expect regular source file to be ignored")
	}
}

func TestWatchRootsDirectoryAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package sample\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	dirRoots, err := watchRoots(tmpDir)
	if err != nil {
		t.Fatalf("watchRoots(dir) returned error: %v", err)
	}
	if len(dirRoots) != 1 || filepath.Clean(dirRoots[0]) != filepath.Clean(tmpDir) {
		t.Fatalf("unexpected dir roots: %v", dirRoots)
	}

	fileRoots, err := watchRoots(filePath)
	if err != nil {
		t.Fatalf("watchRoots(file) returned error: %v", err)
	}
	if len(fileRoots) != 1 || filepath.Clean(fileRoots[0]) != filepath.Clean(tmpDir) {
		t.Fatalf("unexpected file roots: %v", fileRoots)
	}
}

func TestSummarizeChangesByFile(t *testing.T) {
	report := structdiff.Report{
		AddedSymbols: []structdiff.SymbolRef{
			{File: "a.go"},
			{File: "a.go"},
			{File: "b.go"},
		},
		RemovedSymbols: []structdiff.SymbolRef{
			{File: "a.go"},
		},
		ModifiedSymbols: []structdiff.ModifiedSymbol{
			{After: structdiff.SymbolRef{File: "b.go"}},
		},
		ImportChanges: []structdiff.FileImportChange{
			{File: "a.go", Added: []string{"fmt"}, Removed: []string{"os", "io"}},
		},
	}

	summaries := summarizeChangesByFile(report)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 file summaries, got %d", len(summaries))
	}
	if summaries[0].File != "a.go" || summaries[0].Added != 2 || summaries[0].Removed != 1 || summaries[0].ImportAdded != 1 || summaries[0].ImportRemoved != 2 {
		t.Fatalf("unexpected summary for a.go: %+v", summaries[0])
	}
	if summaries[1].File != "b.go" || summaries[1].Added != 1 || summaries[1].Modified != 1 {
		t.Fatalf("unexpected summary for b.go: %+v", summaries[1])
	}
}

func TestRunIndexOnceIfChanged(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, ".gts", "index.json")
	sourcePath := filepath.Join(tmpDir, "main.go")

	writeSource := func(body string) {
		t.Helper()
		if err := os.WriteFile(sourcePath, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	writeSource("package sample\n\nfunc A() {}\n")

	// First run: no baseline cache means changed.
	err := runIndex([]string{tmpDir, "--out", outPath, "--once-if-changed"})
	if err == nil {
		t.Fatal("expected once-if-changed to return change exit on first run")
	}
	assertExitCode(t, err, 2)

	// Second run: unchanged input should return success.
	if err := runIndex([]string{tmpDir, "--out", outPath, "--once-if-changed"}); err != nil {
		t.Fatalf("expected unchanged second run to succeed, got %v", err)
	}

	// Third run: modify source so changes are detected again.
	time.Sleep(2 * time.Millisecond)
	writeSource("package sample\n\nfunc A() {}\nfunc B() {}\n")
	err = runIndex([]string{tmpDir, "--out", outPath, "--once-if-changed"})
	if err == nil {
		t.Fatal("expected changed third run to return change exit")
	}
	assertExitCode(t, err, 2)
}

func TestRunLint_MaxLinesViolation(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func short() {
	println("ok")
}

func long() {
	println("1")
	println("2")
	println("3")
	println("4")
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := runLint([]string{
		tmpDir,
		"--rule", "no function longer than 5 lines",
	})
	if err == nil {
		t.Fatal("expected lint rule to fail with violation")
	}
	assertExitCode(t, err, 3)
}

func TestRunLint_NoImportViolation(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

import "fmt"

func A() {
	fmt.Println("ok")
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := runLint([]string{
		tmpDir,
		"--rule", "no import fmt",
	})
	if err == nil {
		t.Fatal("expected lint import rule to fail with violation")
	}
	assertExitCode(t, err, 3)
}

func TestRunStats(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

type Service struct{}

func Work() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runStats([]string{
		tmpDir,
		"--top", "5",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runStats returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"stats: files=1 symbols=2", "languages:", "kinds:", "top files"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

type Service struct{}

func Work() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runFiles([]string{
		tmpDir,
		"--language", "go",
		"--min-symbols", "1",
		"--sort", "symbols",
		"--top", "5",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runFiles returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"files: total=1 shown=1", "main.go language=go symbols=2"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunDeps(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal", "x"), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	mainSource := `package main

import (
	"fmt"
	"sample/internal/x"
)

func main() {
	_ = fmt.Sprintf("%v", x.Value)
}
`
	xSource := `package x

const Value = 1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module sample\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainSource), 0o644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "x", "x.go"), []byte(xSource), 0o644); err != nil {
		t.Fatalf("WriteFile x.go failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runDeps([]string{
		tmpDir,
		"--by", "package",
		"--top", "5",
		"--focus", ".",
		"--depth", "2",
		"--reverse",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runDeps returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"deps: mode=package", "top outgoing", "top incoming", "focus: . direction=reverse depth=2"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunBridge(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal", "x"), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	mainSource := `package main

import (
	"fmt"
	"sample/internal/x"
)

func main() {
	_ = fmt.Sprintf("%v", x.Value)
}
`
	xSource := `package x

const Value = 1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module sample\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainSource), 0o644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "x", "x.go"), []byte(xSource), 0o644); err != nil {
		t.Fatalf("WriteFile x.go failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runBridge([]string{
		tmpDir,
		"--top", "5",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runBridge returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"bridge:", "components:", "top bridges", "external pressure"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunGrepCount(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func A() {}
func B() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runGrep([]string{
		"function_definition[name=/./]",
		tmpDir,
		"--count",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runGrep returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if strings.TrimSpace(output.String()) != "2" {
		t.Fatalf("unexpected count output %q", output.String())
	}
}

func TestRunScope(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

import "fmt"

func work(input string) {
	value := input
	fmt.Println(value)
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runScope([]string{
		sourcePath,
		"--root", tmpDir,
		"--line", "7",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runScope returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"package: sample", "input (param)", "value (local_var)", "fmt (import)"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunChunk(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func A() {}
func B() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := runChunk([]string{
		tmpDir,
		"--tokens", "200",
	})
	_ = writePipe.Close()
	if runErr != nil {
		t.Fatalf("runChunk returned error: %v", runErr)
	}

	var output bytes.Buffer
	if _, err := output.ReadFrom(readPipe); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"chunks:", "function_definition", "func A()"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunRefactorDryRunAndWrite(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func OldName() {}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Dry-run should not mutate.
	if err := runRefactor([]string{
		"function_definition[name=/^OldName$/]",
		"NewName",
		tmpDir,
	}); err != nil {
		t.Fatalf("runRefactor dry-run returned error: %v", err)
	}

	afterDryRun, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile after dry-run failed: %v", err)
	}
	if !strings.Contains(string(afterDryRun), "OldName") {
		t.Fatalf("expected dry-run to preserve original name, got:\n%s", string(afterDryRun))
	}

	// Write mode should apply rename.
	if err := runRefactor([]string{
		"function_definition[name=/^OldName$/]",
		"NewName",
		tmpDir,
		"--write",
	}); err != nil {
		t.Fatalf("runRefactor write returned error: %v", err)
	}

	afterWrite, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile after write failed: %v", err)
	}
	if !strings.Contains(string(afterWrite), "NewName") {
		t.Fatalf("expected write to apply rename, got:\n%s", string(afterWrite))
	}
}

func TestRunRefactorCallsites(t *testing.T) {
	tmpDir := t.TempDir()
	defPath := filepath.Join(tmpDir, "a.go")
	usePath := filepath.Join(tmpDir, "b.go")

	defSource := `package sample

func OldName() {}
`
	useSource := `package sample

func Use() {
	OldName()
}
`
	if err := os.WriteFile(defPath, []byte(defSource), 0o644); err != nil {
		t.Fatalf("WriteFile a.go failed: %v", err)
	}
	if err := os.WriteFile(usePath, []byte(useSource), 0o644); err != nil {
		t.Fatalf("WriteFile b.go failed: %v", err)
	}

	if err := runRefactor([]string{
		"function_definition[name=/^OldName$/]",
		"NewName",
		tmpDir,
		"--callsites",
		"--write",
	}); err != nil {
		t.Fatalf("runRefactor callsites write returned error: %v", err)
	}

	afterUse, err := os.ReadFile(usePath)
	if err != nil {
		t.Fatalf("ReadFile b.go failed: %v", err)
	}
	if !strings.Contains(string(afterUse), "NewName()") {
		t.Fatalf("expected callsite to be renamed, got:\n%s", string(afterUse))
	}
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	withCode, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Fatalf("expected error with exit code, got %T (%v)", err, err)
	}
	if got := withCode.ExitCode(); got != want {
		t.Fatalf("unexpected exit code: got=%d want=%d err=%v", got, want, err)
	}
}
