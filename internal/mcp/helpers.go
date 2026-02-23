package mcp

import (
	"fmt"
	"strings"

	"gts-suite/internal/index"
	"gts-suite/internal/model"
	"gts-suite/internal/xref"
)

func (s *Service) loadOrBuild(cachePath string, target string) (*model.Index, error) {
	if strings.TrimSpace(cachePath) != "" {
		return index.Load(cachePath)
	}

	if strings.TrimSpace(target) == "" {
		target = s.defaultRoot
	}
	builder := index.NewBuilder()
	return builder.BuildPath(target)
}

func (s *Service) loadIndexFromSource(pathArg, cacheArg string) (*model.Index, error) {
	cachePath := strings.TrimSpace(cacheArg)
	if cachePath != "" {
		return index.Load(cachePath)
	}

	target := strings.TrimSpace(pathArg)
	if target == "" {
		target = s.defaultRoot
	}
	builder := index.NewBuilder()
	return builder.BuildPath(target)
}

func requiredStringArg(args map[string]any, key string) (string, error) {
	value := stringArg(args, key)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	return value, nil
}

func stringArg(args map[string]any, key string) string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func (s *Service) stringArgOrDefault(args map[string]any, key, fallback string) string {
	value := stringArg(args, key)
	if value == "" {
		return fallback
	}
	return value
}

func intArg(args map[string]any, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch typed := raw.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return fallback
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	value, ok := raw.(bool)
	if !ok {
		return fallback
	}
	return value
}

func stringSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return nil
		}
		return []string{value}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			values = append(values, text)
		}
		return values
	default:
		return nil
	}
}

func compactNodeText(text string) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxLen = 160
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func deadKindAllowed(definition xref.Definition, mode string) bool {
	switch mode {
	case "callable":
		return definition.Callable
	case "function":
		return definition.Kind == "function_definition"
	case "method":
		return definition.Kind == "method_definition"
	default:
		return false
	}
}

func isEntrypointDefinition(definition xref.Definition) bool {
	if definition.Kind != "function_definition" {
		return false
	}
	return definition.Name == "main" || definition.Name == "init"
}

func isTestSourceFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), "_test.go")
}
