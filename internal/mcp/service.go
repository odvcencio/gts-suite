// Package mcp exposes gts-suite tools as an MCP stdio server for AI agent integration via JSON-RPC.
package mcp

import (
	"fmt"
	"sort"
	"strings"
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
