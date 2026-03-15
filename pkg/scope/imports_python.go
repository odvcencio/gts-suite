package scope

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolvePythonImport resolves a Python import path to a directory on disk.
// Searches: project root, then common site-packages locations.
func ResolvePythonImport(importPath string, projectRoot string) string {
	// Convert dotted import to path: "os.path" → "os/path"
	relPath := strings.ReplaceAll(importPath, ".", "/")

	// 1. Check project root
	candidates := []string{
		filepath.Join(projectRoot, relPath),
		filepath.Join(projectRoot, relPath+".py"),
		filepath.Join(projectRoot, relPath, "__init__.py"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if strings.HasSuffix(c, ".py") {
				return filepath.Dir(c)
			}
			return c
		}
	}

	// 2. Check common virtualenv/site-packages paths
	venvDirs := []string{
		filepath.Join(projectRoot, ".venv", "lib"),
		filepath.Join(projectRoot, "venv", "lib"),
	}
	for _, venv := range venvDirs {
		if info, err := os.Stat(venv); err == nil && info.IsDir() {
			// Walk to find site-packages
			entries, _ := os.ReadDir(venv)
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "python") {
					sp := filepath.Join(venv, e.Name(), "site-packages", relPath)
					if _, err := os.Stat(sp); err == nil {
						return sp
					}
					spFile := sp + ".py"
					if _, err := os.Stat(spFile); err == nil {
						return filepath.Dir(spFile)
					}
				}
			}
		}
	}

	return ""
}
