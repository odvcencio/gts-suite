package lsp

import "encoding/json"

// JSON-RPC 2.0 message types
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LSP types -- minimal set for initialize
type InitializeParams struct {
	RootURI  string `json:"rootUri"`
	RootPath string `json:"rootPath"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ServerCapabilities struct {
	TextDocumentSync        int  `json:"textDocumentSync,omitempty"`
	DocumentSymbolProvider  bool `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider bool `json:"workspaceSymbolProvider,omitempty"`
	DefinitionProvider      bool `json:"definitionProvider,omitempty"`
	ReferencesProvider      bool `json:"referencesProvider,omitempty"`
	HoverProvider           bool `json:"hoverProvider,omitempty"`
	CompletionProvider      any  `json:"completionProvider,omitempty"`
	RenameProvider          bool `json:"renameProvider,omitempty"`
	DiagnosticProvider      any  `json:"diagnosticProvider,omitempty"`
}

// Text document types
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type Position struct {
	Line      int `json:"line"`      // 0-based
	Character int `json:"character"` // 0-based
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type LSPLocation struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type SymbolInformation struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Location LSPLocation `json:"location"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// LSP Symbol kinds
const (
	SKFile        = 1
	SKModule      = 2
	SKNamespace   = 3
	SKPackage     = 4
	SKClass       = 5
	SKMethod      = 6
	SKProperty    = 7
	SKField       = 8
	SKConstructor = 9
	SKEnum        = 10
	SKInterface   = 11
	SKFunction    = 12
	SKVariable    = 13
	SKConstant    = 14
	SKString      = 15
	SKNumber      = 16
	SKBoolean     = 17
	SKArray       = 18
	SKStruct      = 23
)

// TextDocumentSync kinds
const (
	SyncNone        = 0
	SyncFull        = 1
	SyncIncremental = 2
)
