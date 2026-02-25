package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/odvcencio/gts-suite/pkg/index"
	"github.com/odvcencio/gts-suite/pkg/model"
	"github.com/odvcencio/gts-suite/pkg/scope"
)

// Service holds workspace state and handles LSP requests.
type Service struct {
	mu         sync.RWMutex
	rootURI    string
	rootPath   string
	idx        *model.Index
	builder    *index.Builder
	scopeGraph *scope.Graph
}

func NewService() *Service {
	return &Service{
		builder: index.NewBuilder(),
	}
}

// Register wires all LSP handlers onto a Server.
func (s *Service) Register(srv *Server) {
	srv.Handle("initialize", s.handleInitialize)
	srv.Handle("shutdown", s.handleShutdown)
	srv.Handle("textDocument/documentSymbol", s.handleDocumentSymbol)
	srv.Handle("workspace/symbol", s.handleWorkspaceSymbol)
	srv.Handle("textDocument/definition", s.handleDefinition)
	srv.Handle("textDocument/references", s.handleReferences)
	srv.Handle("textDocument/hover", s.handleHover)
	srv.Handle("textDocument/rename", s.handleRename)

	srv.OnNotify("initialized", func(params json.RawMessage) {
		s.buildIndex()
	})
	srv.OnNotify("textDocument/didOpen", s.handleDidOpen)
	srv.OnNotify("textDocument/didSave", s.handleDidSave)
	srv.OnNotify("exit", func(params json.RawMessage) {})
}

func (s *Service) handleInitialize(params json.RawMessage) (any, error) {
	var p InitializeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	s.rootURI = p.RootURI
	s.rootPath = uriToPath(p.RootURI)
	if s.rootPath == "" {
		s.rootPath = p.RootPath
	}

	return InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:        SyncFull,
			DocumentSymbolProvider:  true,
			WorkspaceSymbolProvider: true,
			DefinitionProvider:      true,
			ReferencesProvider:      true,
			HoverProvider:           true,
			RenameProvider:          true,
		},
		ServerInfo: &ServerInfo{Name: "gtsls", Version: "0.1.0"},
	}, nil
}

func (s *Service) handleShutdown(params json.RawMessage) (any, error) {
	return nil, nil
}

func (s *Service) buildIndex() {
	if s.rootPath == "" {
		return
	}
	idx, err := s.builder.BuildPath(s.rootPath)
	if err != nil {
		return
	}

	// Build scope graph
	var sg *scope.Graph
	if g, sgErr := scope.BuildFromIndex(idx, s.rootPath); sgErr == nil {
		sg = g
	}

	s.mu.Lock()
	s.idx = idx
	s.scopeGraph = sg
	s.mu.Unlock()
}

func (s *Service) handleDocumentSymbol(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	path := uriToPath(p.TextDocument.URI)
	relPath := relativeTo(path, s.rootPath)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return []DocumentSymbol{}, nil
	}

	for _, f := range s.idx.Files {
		if f.Path == relPath {
			return symbolsToDocumentSymbols(f.Symbols), nil
		}
	}
	return []DocumentSymbol{}, nil
}

func (s *Service) handleWorkspaceSymbol(params json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return []SymbolInformation{}, nil
	}

	query := strings.ToLower(p.Query)
	var results []SymbolInformation
	for _, f := range s.idx.Files {
		for _, sym := range f.Symbols {
			if query == "" || strings.Contains(strings.ToLower(sym.Name), query) {
				results = append(results, SymbolInformation{
					Name: sym.Name,
					Kind: symbolKindFromModel(sym.Kind),
					Location: LSPLocation{
						URI:   pathToURI(f.Path, s.rootPath),
						Range: symbolRange(sym),
					},
				})
			}
		}
	}
	return results, nil
}

func (s *Service) handleDidOpen(params json.RawMessage) {
	// no-op for now
}

func (s *Service) handleDidSave(params json.RawMessage) {
	if s.rootPath == "" {
		return
	}
	s.mu.RLock()
	prev := s.idx
	s.mu.RUnlock()

	newIdx, _, err := s.builder.BuildPathIncremental(s.rootPath, prev)
	if err != nil {
		return
	}

	// Rebuild scope graph
	var sg *scope.Graph
	if g, sgErr := scope.BuildFromIndex(newIdx, s.rootPath); sgErr == nil {
		sg = g
	}

	s.mu.Lock()
	s.idx = newIdx
	s.scopeGraph = sg
	s.mu.Unlock()
}

func (s *Service) handleDefinition(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	path := uriToPath(p.TextDocument.URI)
	relPath := relativeTo(path, s.rootPath)
	line := p.Position.Line + 1 // LSP is 0-based, model is 1-based

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return nil, nil
	}

	// Try scope graph resolution first
	if s.scopeGraph != nil {
		fs := s.scopeGraph.FileScope(relPath)
		if fs != nil {
			for i := range fs.Refs {
				ref := &fs.Refs[i]
				if ref.Loc.StartLine == line && ref.Resolved != nil {
					return LSPLocation{
						URI: pathToURI(ref.Resolved.Loc.File, s.rootPath),
						Range: Range{
							Start: Position{Line: ref.Resolved.Loc.StartLine - 1, Character: ref.Resolved.Loc.StartCol},
							End:   Position{Line: ref.Resolved.Loc.EndLine - 1, Character: ref.Resolved.Loc.EndCol},
						},
					}, nil
				}
			}
		}
	}

	// Fall back to name-based resolution
	symbolName := s.symbolNameAtPosition(relPath, line, p.Position.Character)
	if symbolName == "" {
		return nil, nil
	}

	// Search for matching definition across the index
	for _, f := range s.idx.Files {
		for _, sym := range f.Symbols {
			if sym.Name == symbolName {
				return LSPLocation{
					URI:   pathToURI(f.Path, s.rootPath),
					Range: symbolRange(sym),
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *Service) handleReferences(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	path := uriToPath(p.TextDocument.URI)
	relPath := relativeTo(path, s.rootPath)
	line := p.Position.Line + 1

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return []LSPLocation{}, nil
	}

	symbolName := s.symbolNameAtPosition(relPath, line, p.Position.Character)
	if symbolName == "" {
		return []LSPLocation{}, nil
	}

	var locs []LSPLocation
	for _, f := range s.idx.Files {
		for _, ref := range f.References {
			if ref.Name == symbolName {
				locs = append(locs, LSPLocation{
					URI: pathToURI(f.Path, s.rootPath),
					Range: Range{
						Start: Position{Line: ref.StartLine - 1, Character: ref.StartColumn},
						End:   Position{Line: ref.EndLine - 1, Character: ref.EndColumn},
					},
				})
			}
		}
	}
	return locs, nil
}

func (s *Service) handleHover(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	path := uriToPath(p.TextDocument.URI)
	relPath := relativeTo(path, s.rootPath)
	line := p.Position.Line + 1

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return nil, nil
	}

	for _, f := range s.idx.Files {
		if f.Path != relPath {
			continue
		}
		for _, sym := range f.Symbols {
			if sym.StartLine <= line && line <= sym.EndLine {
				content := sym.Kind + " " + sym.Name
				if sym.Signature != "" {
					content = sym.Name + sym.Signature
				}
				return Hover{
					Contents: MarkupContent{Kind: "markdown", Value: "```" + f.Language + "\n" + content + "\n```"},
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *Service) handleRename(params json.RawMessage) (any, error) {
	var p RenameParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	path := uriToPath(p.TextDocument.URI)
	relPath := relativeTo(path, s.rootPath)
	line := p.Position.Line + 1

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.idx == nil {
		return nil, fmt.Errorf("index not ready")
	}

	symbolName := s.symbolNameAtPosition(relPath, line, p.Position.Character)
	if symbolName == "" {
		return nil, fmt.Errorf("no symbol at position")
	}

	// Collect all edits: definitions + references
	changes := make(map[string][]TextEdit)
	for _, f := range s.idx.Files {
		uri := pathToURI(f.Path, s.rootPath)
		for _, sym := range f.Symbols {
			if sym.Name == symbolName {
				changes[uri] = append(changes[uri], TextEdit{
					Range:   symbolNameRange(sym),
					NewText: p.NewName,
				})
			}
		}
		for _, ref := range f.References {
			if ref.Name == symbolName {
				changes[uri] = append(changes[uri], TextEdit{
					Range: Range{
						Start: Position{Line: ref.StartLine - 1, Character: ref.StartColumn},
						End:   Position{Line: ref.EndLine - 1, Character: ref.EndColumn},
					},
					NewText: p.NewName,
				})
			}
		}
	}

	return WorkspaceEdit{Changes: changes}, nil
}

func symbolNameRange(sym model.Symbol) Range {
	// Approximate: use the start line, first column
	return Range{
		Start: Position{Line: sym.StartLine - 1, Character: 0},
		End:   Position{Line: sym.StartLine - 1, Character: len(sym.Name)},
	}
}

// symbolNameAtPosition finds the symbol/reference name at a given cursor position.
func (s *Service) symbolNameAtPosition(relPath string, line, col int) string {
	for _, f := range s.idx.Files {
		if f.Path != relPath {
			continue
		}
		// Check symbols
		for _, sym := range f.Symbols {
			if sym.StartLine <= line && line <= sym.EndLine {
				return sym.Name
			}
		}
		// Check references
		for _, ref := range f.References {
			if ref.StartLine == line {
				return ref.Name
			}
		}
	}
	return ""
}

// --- Helpers ---

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

func pathToURI(relPath, root string) string {
	if root != "" {
		return "file://" + root + "/" + relPath
	}
	return "file://" + relPath
}

func relativeTo(abs, root string) string {
	if root != "" && strings.HasPrefix(abs, root) {
		rel := strings.TrimPrefix(abs, root)
		return strings.TrimPrefix(rel, "/")
	}
	return abs
}

func symbolRange(sym model.Symbol) Range {
	return Range{
		Start: Position{Line: sym.StartLine - 1, Character: 0},
		End:   Position{Line: sym.EndLine - 1, Character: 0},
	}
}

func symbolsToDocumentSymbols(syms []model.Symbol) []DocumentSymbol {
	result := make([]DocumentSymbol, 0, len(syms))
	for _, sym := range syms {
		r := symbolRange(sym)
		result = append(result, DocumentSymbol{
			Name:           sym.Name,
			Kind:           symbolKindFromModel(sym.Kind),
			Range:          r,
			SelectionRange: r,
		})
	}
	return result
}

func symbolKindFromModel(kind string) int {
	switch kind {
	case "function_definition":
		return SKFunction
	case "method_definition":
		return SKMethod
	case "class_definition":
		return SKClass
	case "interface_definition":
		return SKInterface
	case "struct_definition":
		return SKStruct
	case "enum_definition":
		return SKEnum
	case "type_definition":
		return SKClass
	case "constant_definition":
		return SKConstant
	case "variable_definition":
		return SKVariable
	case "module_definition":
		return SKModule
	case "constructor_definition":
		return SKConstructor
	default:
		return SKVariable
	}
}
