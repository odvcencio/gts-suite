package bridge

import (
	"os"
	"path/filepath"
	"testing"

	"gts-suite/internal/model"
)

func TestBuildBridgeReport(t *testing.T) {
	tmpDir := t.TempDir()
	goMod := "module example.com/repo\n\ngo 1.25\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod failed: %v", err)
	}

	idx := &model.Index{
		Root: tmpDir,
		Files: []model.FileSummary{
			{
				Path:    "cmd/api/main.go",
				Imports: []string{"fmt", "example.com/repo/internal/store"},
			},
			{
				Path:    "internal/store/store.go",
				Imports: []string{"database/sql", "example.com/repo/internal/model"},
			},
			{
				Path:    "internal/model/model.go",
				Imports: []string{},
			},
		},
	}

	report, err := Build(idx, Options{Top: 10})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if report.Module != "example.com/repo" {
		t.Fatalf("unexpected module %q", report.Module)
	}
	if report.ComponentCount < 3 {
		t.Fatalf("expected at least 3 components, got %d", report.ComponentCount)
	}
	if report.BridgeCount == 0 {
		t.Fatalf("expected at least one bridge edge, got %d", report.BridgeCount)
	}
	if len(report.TopBridges) == 0 {
		t.Fatal("expected top bridges")
	}
	if len(report.ExternalByComponent) == 0 {
		t.Fatal("expected external metrics")
	}
}

func TestBuildNilIndex(t *testing.T) {
	if _, err := Build(nil, Options{}); err == nil {
		t.Fatal("expected nil index to fail")
	}
}

func TestComponentForPackage(t *testing.T) {
	cases := map[string]string{
		".":               "root",
		"cmd/gts":         "cmd/gts",
		"internal/query":  "internal/query",
		"pkg/contracts":   "pkg/contracts",
		"service/payment": "service",
		"single":          "single",
	}
	for input, want := range cases {
		if got := componentForPackage(input); got != want {
			t.Fatalf("componentForPackage(%q)=%q want=%q", input, got, want)
		}
	}
}
