package scope

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// GoImportResolver resolves Go import paths to filesystem directories.
type GoImportResolver struct {
	rootDir    string
	modulePath string
	goRoot     string
	goModCache string
}

func NewGoImportResolver(rootDir, modulePath string) *GoImportResolver {
	goRoot := runtime.GOROOT()
	goModCache := os.Getenv("GOMODCACHE")
	if goModCache == "" {
		goPath := os.Getenv("GOPATH")
		if goPath == "" {
			home, _ := os.UserHomeDir()
			goPath = filepath.Join(home, "go")
		}
		goModCache = filepath.Join(goPath, "pkg", "mod")
	}
	return &GoImportResolver{
		rootDir:    rootDir,
		modulePath: modulePath,
		goRoot:     goRoot,
		goModCache: goModCache,
	}
}

// Resolve maps an import path to a directory on disk.
func (r *GoImportResolver) Resolve(importPath string) (string, error) {
	// 1. Local module package
	if r.modulePath != "" && strings.HasPrefix(importPath, r.modulePath) {
		rel := strings.TrimPrefix(importPath, r.modulePath)
		rel = strings.TrimPrefix(rel, "/")
		dir := filepath.Join(r.rootDir, rel)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}

	// 2. Standard library
	stdDir := filepath.Join(r.goRoot, "src", importPath)
	if info, err := os.Stat(stdDir); err == nil && info.IsDir() {
		return stdDir, nil
	}

	// 3. Module cache (simplified: scan for versioned dirs)
	if r.goModCache != "" {
		matches, _ := filepath.Glob(filepath.Join(r.goModCache, importPath+"@*"))
		if len(matches) > 0 {
			return matches[len(matches)-1], nil
		}
		// Try parent module with subpath
		parts := strings.Split(importPath, "/")
		for i := len(parts) - 1; i >= 2; i-- {
			parent := strings.Join(parts[:i], "/")
			sub := strings.Join(parts[i:], "/")
			pMatches, _ := filepath.Glob(filepath.Join(r.goModCache, parent+"@*"))
			if len(pMatches) > 0 {
				dir := filepath.Join(pMatches[len(pMatches)-1], sub)
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					return dir, nil
				}
			}
		}
	}

	return "", fmt.Errorf("cannot resolve import %q", importPath)
}
