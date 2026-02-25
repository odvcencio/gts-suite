package scope

import (
	"os"
	"path/filepath"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gts-suite/pkg/model"
)

// BuildFromIndex constructs a scope graph for all files in an index.
func BuildFromIndex(idx *model.Index, rootPath string) (*Graph, error) {
	graph := NewGraph()

	for _, f := range idx.Files {
		entry := grammars.DetectLanguage(f.Path)
		if entry == nil {
			continue
		}
		lang := entry.Language()
		rules, err := LoadRules(entry.Name, lang)
		if err != nil {
			// No scope rules for this language — skip
			continue
		}

		absPath := filepath.Join(rootPath, f.Path)
		src, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		parser := gotreesitter.NewParser(lang)
		var tree *gotreesitter.Tree
		if entry.TokenSourceFactory != nil {
			ts := entry.TokenSourceFactory(src, lang)
			tree, err = parser.ParseWithTokenSource(src, ts)
		} else {
			tree, err = parser.Parse(src)
		}
		if err != nil {
			continue
		}

		fileScope := BuildFileScope(tree, lang, src, rules, f.Path)
		graph.AddFileScope(f.Path, fileScope)
	}

	// Resolve all references within each file scope
	for _, fs := range graph.FileScopes {
		ResolveAll(fs)
	}

	return graph, nil
}
