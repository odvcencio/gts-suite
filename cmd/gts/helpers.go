package main

import (
	"encoding/json"
	"os"
	"strings"

	"gts-suite/internal/index"
	"gts-suite/internal/model"
	"gts-suite/internal/xref"
)

func loadOrBuild(cachePath string, target string) (*model.Index, error) {
	if strings.TrimSpace(cachePath) != "" {
		return index.Load(cachePath)
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
