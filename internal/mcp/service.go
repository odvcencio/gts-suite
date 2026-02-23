package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"gts-suite/internal/contextpack"
	"gts-suite/internal/deps"
	"gts-suite/internal/index"
	"gts-suite/internal/model"
	gtsscope "gts-suite/internal/scope"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Service struct {
	defaultRoot  string
	defaultCache string
}

func NewService(defaultRoot, defaultCache string) *Service {
	root := strings.TrimSpace(defaultRoot)
	if root == "" {
		root = "."
	}
	return &Service{
		defaultRoot:  root,
		defaultCache: strings.TrimSpace(defaultCache),
	}
}

func (s *Service) Tools() []Tool {
	return []Tool{
		{
			Name:        "gts_query",
			Description: "Run a raw tree-sitter S-expression query across indexed files",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "tree-sitter query pattern"},
					"path":    map[string]any{"type": "string", "description": "index root path override"},
					"cache":   map[string]any{"type": "string", "description": "index cache path override"},
					"capture": map[string]any{
						"oneOf": []any{
							map[string]any{"type": "string"},
							map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "gts_refs",
			Description: "Find indexed references by symbol name or regex",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string"},
					"regex": map[string]any{"type": "boolean"},
					"path":  map[string]any{"type": "string"},
					"cache": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "gts_context",
			Description: "Pack focused context for a file and line",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":           map[string]any{"type": "string"},
					"line":           map[string]any{"type": "integer"},
					"tokens":         map[string]any{"type": "integer"},
					"semantic":       map[string]any{"type": "boolean"},
					"semantic_depth": map[string]any{"type": "integer"},
					"root":           map[string]any{"type": "string"},
					"cache":          map[string]any{"type": "string"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "gts_scope",
			Description: "Resolve symbols in scope for a file and line",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":  map[string]any{"type": "string"},
					"line":  map[string]any{"type": "integer"},
					"root":  map[string]any{"type": "string"},
					"cache": map[string]any{"type": "string"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "gts_deps",
			Description: "Analyze dependency graph from structural imports",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"cache":   map[string]any{"type": "string"},
					"by":      map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer"},
					"focus":   map[string]any{"type": "string"},
					"depth":   map[string]any{"type": "integer"},
					"reverse": map[string]any{"type": "boolean"},
					"edges":   map[string]any{"type": "boolean"},
				},
			},
		},
	}
}

func (s *Service) Call(name string, args map[string]any) (any, error) {
	switch strings.TrimSpace(name) {
	case "gts_query":
		return s.callQuery(args)
	case "gts_refs":
		return s.callRefs(args)
	case "gts_context":
		return s.callContext(args)
	case "gts_scope":
		return s.callScope(args)
	case "gts_deps":
		return s.callDeps(args)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func (s *Service) callQuery(args map[string]any) (any, error) {
	pattern, err := requiredStringArg(args, "pattern")
	if err != nil {
		return nil, err
	}
	captures := stringSliceArg(args, "capture")
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	captureFilter := map[string]bool{}
	for _, capture := range captures {
		captureFilter[strings.TrimSpace(capture)] = true
	}

	entriesByLanguage := map[string]grammars.LangEntry{}
	for _, entry := range grammars.AllLanguages() {
		if strings.TrimSpace(entry.Name) == "" || entry.Language == nil {
			continue
		}
		entriesByLanguage[entry.Name] = entry
	}

	queryByLanguage := map[string]*gotreesitter.Query{}
	queryErrorByLanguage := map[string]string{}
	langByName := map[string]*gotreesitter.Language{}
	parserByLanguage := map[string]*gotreesitter.Parser{}

	type queryCaptureMatch struct {
		File        string `json:"file"`
		Language    string `json:"language"`
		Pattern     int    `json:"pattern"`
		Capture     string `json:"capture"`
		NodeType    string `json:"node_type"`
		Text        string `json:"text"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		StartColumn int    `json:"start_column"`
		EndColumn   int    `json:"end_column"`
	}
	type queryLanguageError struct {
		Language string `json:"language"`
		Error    string `json:"error"`
	}

	results := make([]queryCaptureMatch, 0, idx.SymbolCount())
	for _, file := range idx.Files {
		entry, ok := entriesByLanguage[file.Language]
		if !ok {
			continue
		}
		if _, failed := queryErrorByLanguage[file.Language]; failed {
			continue
		}

		lang, ok := langByName[file.Language]
		if !ok {
			lang = entry.Language()
			if lang == nil {
				queryErrorByLanguage[file.Language] = "language loader returned nil"
				continue
			}
			langByName[file.Language] = lang
		}

		queryForLanguage, ok := queryByLanguage[file.Language]
		if !ok {
			compiled, compileErr := gotreesitter.NewQuery(pattern, lang)
			if compileErr != nil {
				queryErrorByLanguage[file.Language] = compileErr.Error()
				continue
			}
			queryByLanguage[file.Language] = compiled
			queryForLanguage = compiled
		}

		sourcePath := filepath.Join(idx.Root, filepath.FromSlash(file.Path))
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			return nil, readErr
		}

		parser, ok := parserByLanguage[file.Language]
		if !ok {
			parser = gotreesitter.NewParser(lang)
			parserByLanguage[file.Language] = parser
		}

		var tree *gotreesitter.Tree
		if entry.TokenSourceFactory != nil {
			tokenSource := entry.TokenSourceFactory(source, lang)
			if tokenSource != nil {
				tree = parser.ParseWithTokenSource(source, tokenSource)
			}
		}
		if tree == nil {
			tree = parser.Parse(source)
		}
		if tree == nil || tree.RootNode() == nil {
			continue
		}

		matches := queryForLanguage.Execute(tree)
		for _, match := range matches {
			for _, capture := range match.Captures {
				if len(captureFilter) > 0 && !captureFilter[capture.Name] {
					continue
				}
				node := capture.Node
				if node == nil {
					continue
				}
				startLine := int(node.StartPoint().Row) + 1
				endLine := int(node.EndPoint().Row) + 1
				if endLine < startLine {
					endLine = startLine
				}
				startColumn := int(node.StartPoint().Column) + 1
				endColumn := int(node.EndPoint().Column) + 1
				if endColumn < startColumn {
					endColumn = startColumn
				}
				results = append(results, queryCaptureMatch{
					File:        file.Path,
					Language:    file.Language,
					Pattern:     match.PatternIndex,
					Capture:     capture.Name,
					NodeType:    node.Type(lang),
					Text:        compactNodeText(node.Text(source)),
					StartLine:   startLine,
					EndLine:     endLine,
					StartColumn: startColumn,
					EndColumn:   endColumn,
				})
			}
		}
		tree.Release()
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].File == results[j].File {
			if results[i].StartLine == results[j].StartLine {
				if results[i].StartColumn == results[j].StartColumn {
					return results[i].Capture < results[j].Capture
				}
				return results[i].StartColumn < results[j].StartColumn
			}
			return results[i].StartLine < results[j].StartLine
		}
		return results[i].File < results[j].File
	})

	languageErrors := make([]queryLanguageError, 0, len(queryErrorByLanguage))
	for language, value := range queryErrorByLanguage {
		languageErrors = append(languageErrors, queryLanguageError{
			Language: language,
			Error:    value,
		})
	}
	sort.Slice(languageErrors, func(i, j int) bool {
		return languageErrors[i].Language < languageErrors[j].Language
	})

	return map[string]any{
		"matches":         results,
		"count":           len(results),
		"language_errors": languageErrors,
	}, nil
}

func (s *Service) callRefs(args map[string]any) (any, error) {
	name, err := requiredStringArg(args, "name")
	if err != nil {
		return nil, err
	}
	regexMode := boolArg(args, "regex", false)
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	matchReference := func(candidate string) bool { return candidate == name }
	if regexMode {
		compiled, compileErr := regexp.Compile(name)
		if compileErr != nil {
			return nil, fmt.Errorf("compile regex: %w", compileErr)
		}
		matchReference = compiled.MatchString
	}

	type referenceMatch struct {
		File        string `json:"file"`
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		StartColumn int    `json:"start_column"`
		EndColumn   int    `json:"end_column"`
	}
	matches := make([]referenceMatch, 0, idx.ReferenceCount())
	for _, file := range idx.Files {
		for _, reference := range file.References {
			if !matchReference(reference.Name) {
				continue
			}
			matches = append(matches, referenceMatch{
				File:        file.Path,
				Kind:        reference.Kind,
				Name:        reference.Name,
				StartLine:   reference.StartLine,
				EndLine:     reference.EndLine,
				StartColumn: reference.StartColumn,
				EndColumn:   reference.EndColumn,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				if matches[i].StartColumn == matches[j].StartColumn {
					return matches[i].Name < matches[j].Name
				}
				return matches[i].StartColumn < matches[j].StartColumn
			}
			return matches[i].StartLine < matches[j].StartLine
		}
		return matches[i].File < matches[j].File
	})

	return map[string]any{
		"matches": matches,
		"count":   len(matches),
	}, nil
}

func (s *Service) callContext(args map[string]any) (any, error) {
	filePath, err := requiredStringArg(args, "file")
	if err != nil {
		return nil, err
	}

	rootPath := s.stringArgOrDefault(args, "root", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	line := intArg(args, "line", 1)
	tokens := intArg(args, "tokens", 800)
	semantic := boolArg(args, "semantic", false)
	semanticDepth := intArg(args, "semantic_depth", 1)

	idx, err := s.loadOrBuild(cachePath, rootPath)
	if err != nil {
		return nil, err
	}

	report, err := contextpack.Build(idx, contextpack.Options{
		FilePath:      filePath,
		Line:          line,
		TokenBudget:   tokens,
		Semantic:      semantic,
		SemanticDepth: semanticDepth,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callScope(args map[string]any) (any, error) {
	filePath, err := requiredStringArg(args, "file")
	if err != nil {
		return nil, err
	}

	rootPath := s.stringArgOrDefault(args, "root", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	line := intArg(args, "line", 1)

	idx, err := s.loadOrBuild(cachePath, rootPath)
	if err != nil {
		return nil, err
	}

	report, err := gtsscope.Build(idx, gtsscope.Options{
		FilePath: filePath,
		Line:     line,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callDeps(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	report, err := deps.Build(idx, deps.Options{
		Mode:         s.stringArgOrDefault(args, "by", "package"),
		Top:          intArg(args, "top", 10),
		Focus:        stringArg(args, "focus"),
		Depth:        intArg(args, "depth", 1),
		Reverse:      boolArg(args, "reverse", false),
		IncludeEdges: boolArg(args, "edges", false),
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

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
