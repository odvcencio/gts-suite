package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gts-suite/pkg/index"
	"github.com/odvcencio/gts-suite/pkg/model"
	"github.com/odvcencio/gts-suite/pkg/xref"
)

func loadOrBuild(cachePath string, target string, noCache bool) (*model.Index, error) {
	if strings.TrimSpace(cachePath) != "" {
		return index.Load(cachePath)
	}
	if !noCache {
		autoPath := filepath.Join(target, ".gts", "index.json")
		if _, err := os.Stat(autoPath); err == nil {
			if idx, err := index.Load(autoPath); err == nil {
				return idx, nil
			}
		}
	}
	return index.NewBuilder().BuildPath(target)
}

func emitJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func compactNodeText(text string) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxLen = 160
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func symbolLabel(name, signature string) string {
	if strings.TrimSpace(signature) != "" {
		return signature
	}
	return name
}

func definitionLabel(definition xref.Definition) string {
	if strings.TrimSpace(definition.Signature) != "" {
		return definition.Signature
	}
	return definition.Name
}
