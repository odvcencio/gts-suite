// Package mcp exposes gts-suite tools as an MCP stdio server for AI agent integration via JSON-RPC.
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

	"gts-suite/internal/bridge"
	"gts-suite/internal/chunk"
	"gts-suite/internal/contextpack"
	"gts-suite/internal/deps"
	"gts-suite/internal/files"
	"gts-suite/internal/index"
	"gts-suite/internal/lint"
	"gts-suite/internal/model"
	"gts-suite/internal/query"
	"gts-suite/internal/refactor"
	gtsscope "gts-suite/internal/scope"
	"gts-suite/internal/stats"
	"gts-suite/internal/structdiff"
	"gts-suite/internal/xref"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Service struct {
	defaultRoot  string
	defaultCache string
	allowWrites  bool
}

type ServiceOptions struct {
	AllowWrites bool
}

func NewService(defaultRoot, defaultCache string) *Service {
	return NewServiceWithOptions(defaultRoot, defaultCache, ServiceOptions{})
}

func NewServiceWithOptions(defaultRoot, defaultCache string, opts ServiceOptions) *Service {
	root := strings.TrimSpace(defaultRoot)
	if root == "" {
		root = "."
	}
	return &Service{
		defaultRoot:  root,
		defaultCache: strings.TrimSpace(defaultCache),
		allowWrites:  opts.AllowWrites,
	}
}

func (s *Service) Tools() []Tool {
	tools := []Tool{
		{
			Name:        "gts_grep",
			Description: "Run structural selector matches across indexed symbols",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{"type": "string"},
					"path":     map[string]any{"type": "string"},
					"cache":    map[string]any{"type": "string"},
				},
				"required": []string{"selector"},
			},
		},
		{
			Name:        "gts_map",
			Description: "Emit table-of-contents structural summaries for indexed files",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"cache": map[string]any{"type": "string"},
				},
			},
		},
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
		{
			Name:        "gts_callgraph",
			Description: "Traverse resolved call graph from matching callable roots",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"regex":   map[string]any{"type": "boolean"},
					"path":    map[string]any{"type": "string"},
					"cache":   map[string]any{"type": "string"},
					"depth":   map[string]any{"type": "integer"},
					"reverse": map[string]any{"type": "boolean"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "gts_dead",
			Description: "List callable definitions with zero incoming call references",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":                map[string]any{"type": "string"},
					"cache":               map[string]any{"type": "string"},
					"kind":                map[string]any{"type": "string"},
					"include_entrypoints": map[string]any{"type": "boolean"},
					"include_tests":       map[string]any{"type": "boolean"},
				},
			},
		},
		{
			Name:        "gts_chunk",
			Description: "Split code into AST-boundary chunks for retrieval/indexing",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string"},
					"cache":  map[string]any{"type": "string"},
					"tokens": map[string]any{"type": "integer"},
				},
			},
		},
		{
			Name:        "gts_lint",
			Description: "Run structural lint rules and query-pattern rules against index",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"cache":   map[string]any{"type": "string"},
					"rule":    map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}},
					"pattern": map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}},
				},
			},
		},
		{
			Name:        "gts_refactor",
			Description: "Apply structural declaration renames (dry-run by default)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector":      map[string]any{"type": "string"},
					"new_name":      map[string]any{"type": "string"},
					"path":          map[string]any{"type": "string"},
					"cache":         map[string]any{"type": "string"},
					"engine":        map[string]any{"type": "string"},
					"callsites":     map[string]any{"type": "boolean"},
					"cross_package": map[string]any{"type": "boolean"},
					"write":         map[string]any{"type": "boolean"},
				},
				"required": []string{"selector", "new_name"},
			},
		},
		{
			Name:        "gts_diff",
			Description: "Structural diff between two snapshots (path or cache sources)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"before_path":  map[string]any{"type": "string"},
					"before_cache": map[string]any{"type": "string"},
					"after_path":   map[string]any{"type": "string"},
					"after_cache":  map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "gts_stats",
			Description: "Report structural codebase metrics from an index",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"cache": map[string]any{"type": "string"},
					"top":   map[string]any{"type": "integer"},
				},
			},
		},
		{
			Name:        "gts_files",
			Description: "List indexed files with structural density filters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string"},
					"cache":       map[string]any{"type": "string"},
					"language":    map[string]any{"type": "string"},
					"min_symbols": map[string]any{"type": "integer"},
					"sort":        map[string]any{"type": "string"},
					"top":         map[string]any{"type": "integer"},
				},
			},
		},
		{
			Name:        "gts_bridge",
			Description: "Map cross-component dependency bridges",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"cache":   map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer"},
					"focus":   map[string]any{"type": "string"},
					"depth":   map[string]any{"type": "integer"},
					"reverse": map[string]any{"type": "boolean"},
				},
			},
		},
	}
	for i := range tools {
		finalizeToolSchema(&tools[i])
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func finalizeToolSchema(tool *Tool) {
	if tool == nil {
		return
	}

	schema := tool.InputSchema
	if schema == nil {
		schema = map[string]any{}
	}
	schema["type"] = "object"

	properties := normalizeSchemaProperties(schema["properties"])
	schema["properties"] = properties

	required := normalizeRequiredKeys(schema["required"], properties)
	if len(required) > 0 {
		schema["required"] = required
	} else {
		delete(schema, "required")
	}

	if _, ok := schema["additionalProperties"]; !ok {
		schema["additionalProperties"] = false
	}
	tool.InputSchema = schema
}

func normalizeSchemaProperties(raw any) map[string]any {
	switch typed := raw.(type) {
	case map[string]any:
		if typed == nil {
			return map[string]any{}
		}
		return typed
	default:
		return map[string]any{}
	}
}

func normalizeRequiredKeys(raw any, properties map[string]any) []string {
	if len(properties) == 0 {
		return nil
	}

	keys := make([]string, 0, len(properties))
	switch typed := raw.(type) {
	case string:
		keys = append(keys, typed)
	case []string:
		keys = append(keys, typed...)
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				continue
			}
			keys = append(keys, value)
		}
	}

	seen := map[string]bool{}
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		if _, ok := properties[key]; !ok {
			continue
		}
		seen[key] = true
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)
	return normalized
}

func (s *Service) Call(name string, args map[string]any) (any, error) {
	switch strings.TrimSpace(name) {
	case "gts_grep":
		return s.callGrep(args)
	case "gts_map":
		return s.callMap(args)
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
	case "gts_callgraph":
		return s.callCallgraph(args)
	case "gts_dead":
		return s.callDead(args)
	case "gts_chunk":
		return s.callChunk(args)
	case "gts_lint":
		return s.callLint(args)
	case "gts_refactor":
		return s.callRefactor(args)
	case "gts_diff":
		return s.callDiff(args)
	case "gts_stats":
		return s.callStats(args)
	case "gts_files":
		return s.callFiles(args)
	case "gts_bridge":
		return s.callBridge(args)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func (s *Service) callGrep(args map[string]any) (any, error) {
	selectorRaw, err := requiredStringArg(args, "selector")
	if err != nil {
		return nil, err
	}
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	selector, err := query.ParseSelector(selectorRaw)
	if err != nil {
		return nil, err
	}

	type grepMatch struct {
		File      string `json:"file"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Signature string `json:"signature,omitempty"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}

	matches := make([]grepMatch, 0, idx.SymbolCount())
	for _, file := range idx.Files {
		for _, symbol := range file.Symbols {
			if !selector.Match(symbol) {
				continue
			}
			matches = append(matches, grepMatch{
				File:      file.Path,
				Kind:      symbol.Kind,
				Name:      symbol.Name,
				Signature: symbol.Signature,
				StartLine: symbol.StartLine,
				EndLine:   symbol.EndLine,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				return matches[i].Name < matches[j].Name
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

func (s *Service) callMap(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	type mapFileSummary struct {
		Path           string            `json:"path"`
		Language       string            `json:"language"`
		Imports        []string          `json:"imports,omitempty"`
		Symbols        []model.Symbol    `json:"symbols,omitempty"`
		References     []model.Reference `json:"references,omitempty"`
		SymbolCount    int               `json:"symbol_count"`
		ReferenceCount int               `json:"reference_count"`
	}

	files := make([]mapFileSummary, 0, len(idx.Files))
	for _, file := range idx.Files {
		files = append(files, mapFileSummary{
			Path:           file.Path,
			Language:       file.Language,
			Imports:        append([]string(nil), file.Imports...),
			Symbols:        append([]model.Symbol(nil), file.Symbols...),
			References:     append([]model.Reference(nil), file.References...),
			SymbolCount:    len(file.Symbols),
			ReferenceCount: len(file.References),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return map[string]any{
		"root":            idx.Root,
		"generated_at":    idx.GeneratedAt,
		"file_count":      len(files),
		"symbol_count":    idx.SymbolCount(),
		"reference_count": idx.ReferenceCount(),
		"files":           files,
		"errors":          idx.Errors,
	}, nil
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

func (s *Service) callCallgraph(args map[string]any) (any, error) {
	name, err := requiredStringArg(args, "name")
	if err != nil {
		return nil, err
	}
	regexMode := boolArg(args, "regex", false)
	depth := intArg(args, "depth", 2)
	reverse := boolArg(args, "reverse", false)
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	graph, err := xref.Build(idx)
	if err != nil {
		return nil, err
	}
	roots, err := graph.FindDefinitions(name, regexMode)
	if err != nil {
		return nil, err
	}
	rootIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		rootIDs = append(rootIDs, root.ID)
	}
	walk := graph.Walk(rootIDs, depth, reverse)

	return map[string]any{
		"roots":                 walk.Roots,
		"nodes":                 walk.Nodes,
		"edges":                 walk.Edges,
		"depth":                 walk.Depth,
		"reverse":               walk.Reverse,
		"unresolved_call_count": len(graph.Unresolved),
	}, nil
}

func (s *Service) callDead(args map[string]any) (any, error) {
	mode := strings.ToLower(strings.TrimSpace(s.stringArgOrDefault(args, "kind", "callable")))
	switch mode {
	case "callable", "function", "method":
	default:
		return nil, fmt.Errorf("unsupported kind %q (expected callable|function|method)", mode)
	}

	includeEntrypoints := boolArg(args, "include_entrypoints", false)
	includeTests := boolArg(args, "include_tests", false)
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	graph, err := xref.Build(idx)
	if err != nil {
		return nil, err
	}

	type deadMatch struct {
		File      string `json:"file"`
		Package   string `json:"package"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Signature string `json:"signature,omitempty"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Incoming  int    `json:"incoming"`
		Outgoing  int    `json:"outgoing"`
	}

	matches := make([]deadMatch, 0, 64)
	scanned := 0
	for _, definition := range graph.Definitions {
		if !deadKindAllowed(definition, mode) {
			continue
		}
		if !includeEntrypoints && isEntrypointDefinition(definition) {
			continue
		}
		if !includeTests && isTestSourceFile(definition.File) {
			continue
		}

		scanned++
		incoming := graph.IncomingCount(definition.ID)
		if incoming > 0 {
			continue
		}
		matches = append(matches, deadMatch{
			File:      definition.File,
			Package:   definition.Package,
			Kind:      definition.Kind,
			Name:      definition.Name,
			Signature: definition.Signature,
			StartLine: definition.StartLine,
			EndLine:   definition.EndLine,
			Incoming:  incoming,
			Outgoing:  graph.OutgoingCount(definition.ID),
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				return matches[i].Name < matches[j].Name
			}
			return matches[i].StartLine < matches[j].StartLine
		}
		return matches[i].File < matches[j].File
	})

	return map[string]any{
		"kind":    mode,
		"scanned": scanned,
		"count":   len(matches),
		"matches": matches,
	}, nil
}

func (s *Service) callChunk(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	tokens := intArg(args, "tokens", 800)
	if tokens <= 0 {
		return nil, fmt.Errorf("tokens must be > 0")
	}

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}
	filterPath := ""
	if strings.TrimSpace(cachePath) != "" && strings.TrimSpace(target) != "" {
		filterPath = target
	}
	report, err := chunk.Build(idx, chunk.Options{
		TokenBudget: tokens,
		FilterPath:  filterPath,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callLint(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	rawRules := stringSliceArg(args, "rule")
	rawPatterns := stringSliceArg(args, "pattern")
	if len(rawRules) == 0 && len(rawPatterns) == 0 {
		return nil, fmt.Errorf("at least one rule or pattern is required")
	}

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	rules := make([]lint.Rule, 0, len(rawRules))
	for _, rawRule := range rawRules {
		rule, parseErr := lint.ParseRule(rawRule)
		if parseErr != nil {
			return nil, fmt.Errorf("parse rule %q: %w", rawRule, parseErr)
		}
		rules = append(rules, rule)
	}

	patterns := make([]lint.QueryPattern, 0, len(rawPatterns))
	for _, rawPattern := range rawPatterns {
		pattern, loadErr := lint.LoadQueryPattern(rawPattern)
		if loadErr != nil {
			return nil, fmt.Errorf("load pattern %q: %w", rawPattern, loadErr)
		}
		patterns = append(patterns, pattern)
	}

	violations := lint.Evaluate(idx, rules)
	patternViolations, err := lint.EvaluatePatterns(idx, patterns)
	if err != nil {
		return nil, err
	}
	violations = append(violations, patternViolations...)
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File == violations[j].File {
			if violations[i].StartLine == violations[j].StartLine {
				if violations[i].RuleID == violations[j].RuleID {
					return violations[i].Name < violations[j].Name
				}
				return violations[i].RuleID < violations[j].RuleID
			}
			return violations[i].StartLine < violations[j].StartLine
		}
		return violations[i].File < violations[j].File
	})

	return map[string]any{
		"rules":      rules,
		"patterns":   patterns,
		"violations": violations,
		"count":      len(violations),
	}, nil
}

func (s *Service) callRefactor(args map[string]any) (any, error) {
	selectorRaw, err := requiredStringArg(args, "selector")
	if err != nil {
		return nil, err
	}
	newName, err := requiredStringArg(args, "new_name")
	if err != nil {
		return nil, err
	}
	engine := s.stringArgOrDefault(args, "engine", "go")
	updateCallsites := boolArg(args, "callsites", false)
	crossPackage := boolArg(args, "cross_package", false)
	writeChanges := boolArg(args, "write", false)
	if writeChanges && !s.allowWrites {
		return nil, fmt.Errorf("write operations are disabled for this MCP server")
	}
	if crossPackage && !updateCallsites {
		return nil, fmt.Errorf("cross_package requires callsites=true")
	}

	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	selector, err := query.ParseSelector(selectorRaw)
	if err != nil {
		return nil, err
	}
	report, err := refactor.RenameDeclarations(idx, selector, newName, refactor.Options{
		Write:                 writeChanges,
		UpdateCallsites:       updateCallsites,
		CrossPackageCallsites: crossPackage,
		Engine:                engine,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callDiff(args map[string]any) (any, error) {
	beforePath := stringArg(args, "before_path")
	beforeCache := stringArg(args, "before_cache")
	afterPath := stringArg(args, "after_path")
	afterCache := stringArg(args, "after_cache")

	if beforePath == "" && beforeCache == "" {
		return nil, fmt.Errorf("missing before source: set before_path or before_cache")
	}
	if afterPath == "" && afterCache == "" {
		return nil, fmt.Errorf("missing after source: set after_path or after_cache")
	}

	beforeIndex, err := s.loadIndexFromSource(beforePath, beforeCache)
	if err != nil {
		return nil, fmt.Errorf("load before source: %w", err)
	}
	afterIndex, err := s.loadIndexFromSource(afterPath, afterCache)
	if err != nil {
		return nil, fmt.Errorf("load after source: %w", err)
	}

	report := structdiff.Compare(beforeIndex, afterIndex)
	return report, nil
}

func (s *Service) callStats(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	top := intArg(args, "top", 10)
	if top <= 0 {
		return nil, fmt.Errorf("top must be > 0")
	}

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}
	report, err := stats.Build(idx, stats.Options{
		TopFiles: top,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callFiles(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}
	report, err := files.Build(idx, files.Options{
		Language:   stringArg(args, "language"),
		MinSymbols: intArg(args, "min_symbols", 0),
		SortBy:     s.stringArgOrDefault(args, "sort", "symbols"),
		Top:        intArg(args, "top", 50),
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) callBridge(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	top := intArg(args, "top", 20)
	depth := intArg(args, "depth", 1)
	if top <= 0 {
		return nil, fmt.Errorf("top must be > 0")
	}
	if depth <= 0 {
		return nil, fmt.Errorf("depth must be > 0")
	}

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}
	report, err := bridge.Build(idx, bridge.Options{
		Top:     top,
		Focus:   stringArg(args, "focus"),
		Depth:   depth,
		Reverse: boolArg(args, "reverse", false),
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
