package scope

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ResolveTypeScriptImport resolves a TypeScript/JavaScript import path to a file.
// Handles: relative paths, node_modules, @types.
func ResolveTypeScriptImport(importPath string, sourceFile string, projectRoot string) string {
	// Strip quotes
	importPath = strings.Trim(importPath, "\"'`")

	// 1. Relative imports
	if strings.HasPrefix(importPath, ".") {
		dir := filepath.Dir(sourceFile)
		return resolveRelativeTSImport(dir, importPath)
	}

	// 2. Node modules
	nodeModules := filepath.Join(projectRoot, "node_modules")

	// Check @types first (for type declarations)
	typesDir := filepath.Join(nodeModules, "@types", importPath)
	if resolved := resolveTSModuleDir(typesDir); resolved != "" {
		return resolved
	}

	// Check direct module
	modDir := filepath.Join(nodeModules, importPath)
	if resolved := resolveTSModuleDir(modDir); resolved != "" {
		return resolved
	}

	return ""
}

// resolveRelativeTSImport resolves a relative import like "./foo" or "../bar".
func resolveRelativeTSImport(fromDir string, importPath string) string {
	target := filepath.Join(fromDir, importPath)

	// Try exact file extensions
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"} {
		if _, err := os.Stat(target + ext); err == nil {
			return target + ext
		}
	}

	// Try index file in directory
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		for _, idx := range []string{"index.ts", "index.tsx", "index.js", "index.d.ts"} {
			p := filepath.Join(target, idx)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// resolveTSModuleDir resolves a node_modules package directory.
func resolveTSModuleDir(dir string) string {
	// Check for package.json "types" or "main" field
	pkgJSON := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		var pkg struct {
			Types   string `json:"types"`
			Typings string `json:"typings"`
			Main    string `json:"main"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			if pkg.Types != "" {
				p := filepath.Join(dir, pkg.Types)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
			if pkg.Typings != "" {
				p := filepath.Join(dir, pkg.Typings)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
			if pkg.Main != "" {
				p := filepath.Join(dir, pkg.Main)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}

	// Check for index files
	for _, idx := range []string{"index.d.ts", "index.ts", "index.js"} {
		p := filepath.Join(dir, idx)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}
