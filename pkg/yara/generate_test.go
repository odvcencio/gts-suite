package yara

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gts-suite/pkg/model"
)

func TestGenerateEmpty(t *testing.T) {
	idx := &model.Index{}
	rules, err := Generate(idx, "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected no rules from empty index, got %d", len(rules))
	}
}

func TestStringExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	src := `void malware() {
  char *url = "http://evil.com/beacon";
  char *key = "s3cr3t_k3y_value";
  char *cmd = "cmd.exe /c whoami";
  send(url);
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "sample.c"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := &model.Index{
		Root: tmpDir,
		Files: []model.FileSummary{
			{
				Path:     "sample.c",
				Language: "c",
				Symbols: []model.Symbol{
					{Kind: "function_definition", Name: "malware", StartLine: 1, EndLine: 6},
				},
				References: []model.Reference{
					{Kind: "reference.call", Name: "send", StartLine: 5},
				},
			},
		},
	}

	rules, err := Generate(idx, tmpDir, Options{RuleName: "test_mal", MinStrings: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	rule := rules[0]
	if rule.Name != "test_mal" {
		t.Fatalf("expected rule name test_mal, got %q", rule.Name)
	}

	foundURL := false
	for _, s := range rule.Strings {
		if strings.Contains(s, "evil.com") {
			foundURL = true
		}
	}
	if !foundURL {
		t.Fatal("expected URL string in rule")
	}
}

func TestYARAFormat(t *testing.T) {
	tmpDir := t.TempDir()
	src := `void f() {
  char *a = "unique_string_alpha";
  char *b = "unique_string_beta";
  char *c = "unique_string_gamma";
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "test.c"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := &model.Index{
		Root: tmpDir,
		Files: []model.FileSummary{
			{
				Path: "test.c", Language: "c",
				Symbols: []model.Symbol{
					{Kind: "function_definition", Name: "f", StartLine: 1, EndLine: 5},
				},
			},
		},
	}

	rules, err := Generate(idx, tmpDir, Options{RuleName: "format_test", MinStrings: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	yara := rules[0].YARAText
	if !strings.HasPrefix(yara, "rule format_test {") {
		t.Fatalf("bad rule header: %q", yara[:40])
	}
	if !strings.Contains(yara, "strings:") {
		t.Fatal("missing strings section")
	}
	if !strings.Contains(yara, "condition:") {
		t.Fatal("missing condition section")
	}
	if !strings.Contains(yara, "of them") {
		t.Fatal("missing 'of them' condition")
	}
}
