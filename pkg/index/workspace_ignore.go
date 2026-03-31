package index

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/odvcencio/gts-suite/pkg/generated"
	"github.com/odvcencio/gts-suite/pkg/ignore"
)

// workspaceIgnoreFiles lists the config files that anchor a workspace root.
var workspaceIgnoreFiles = []string{".graftignore", ".gtsignore", ".gtsgenerated"}

// workspaceIgnoreRoot walks up from target (resolved to absolute) looking for a
// directory containing any of the workspace config files. Returns the directory
// if found, or target itself (resolved) if none is found.
func workspaceIgnoreRoot(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	dir := abs
	if !info.IsDir() {
		dir = filepath.Dir(abs)
	}

	for {
		for _, name := range workspaceIgnoreFiles {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// No config file found; return the original resolved path.
	if info.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}

// LoadWorkspaceIgnoreMatcher finds the workspace root and loads ignore patterns
// from .graftignore and .gtsignore files found there.
func LoadWorkspaceIgnoreMatcher(target string) (*ignore.Matcher, error) {
	root, err := workspaceIgnoreRoot(target)
	if err != nil {
		return nil, err
	}

	var allPatterns []string
	for _, name := range []string{".graftignore", ".gtsignore"} {
		p := filepath.Join(root, name)
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return nil, readErr
		}
		allPatterns = append(allPatterns, splitLines(string(data))...)
	}

	if len(allPatterns) == 0 {
		return nil, nil
	}
	return ignore.ParsePatterns(allPatterns), nil
}

// splitLines splits s into lines without trailing newlines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// LoadWorkspaceGeneratedConfig finds the workspace root and loads .gtsgenerated
// config entries. Returns nil entries (no error) when the file is absent.
func LoadWorkspaceGeneratedConfig(target string) ([]generated.ConfigEntry, error) {
	root, err := workspaceIgnoreRoot(target)
	if err != nil {
		return nil, err
	}
	return generated.LoadConfigFile(filepath.Join(root, ".gtsgenerated"))
}

// NewBuilderWithWorkspaceIgnores creates a Builder pre-configured with ignore
// patterns and generated-file detection from the workspace config files found
// at or above target.
func NewBuilderWithWorkspaceIgnores(target string) (*Builder, error) {
	builder := NewBuilder()
	matcher, err := LoadWorkspaceIgnoreMatcher(target)
	if err != nil {
		return nil, err
	}
	if matcher != nil {
		builder.SetIgnore(matcher)
	}
	configs, err := LoadWorkspaceGeneratedConfig(target)
	if err != nil {
		return nil, err
	}
	builder.SetDetector(generated.NewDetector(configs))
	return builder, nil
}
