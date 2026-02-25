package lsp

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/odvcencio/gts-suite/pkg/index"
	"github.com/odvcencio/gts-suite/pkg/model"
)

// Service holds workspace state and handles LSP requests.
type Service struct {
	mu       sync.RWMutex
	rootURI  string
	rootPath string
	idx      *model.Index
	builder  *index.Builder
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
	s.mu.Lock()
	s.idx = idx
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
	s.mu.Lock()
	s.idx = newIdx
	s.mu.Unlock()
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
