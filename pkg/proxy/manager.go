package proxy

import (
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// BackendSpec describes how to launch a backend LSP.
type BackendSpec struct {
	Command    string
	Args       []string
	Extensions []string
}

// Manager manages backend LSP processes and routes requests.
type Manager struct {
	backends map[string]*Backend
	specs    map[string]BackendSpec
	logger   *slog.Logger
}

// NewManager creates a proxy manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		backends: make(map[string]*Backend),
		specs:    DefaultBackendSpecs(),
		logger:   logger,
	}
}

// Register adds a backend for a language.
func (m *Manager) Register(b *Backend) {
	m.backends[b.Lang] = b
}

// BackendForLang returns the backend for a language, or nil.
func (m *Manager) BackendForLang(lang string) *Backend {
	return m.backends[lang]
}

// BackendForFile returns the backend for a file based on extension.
func (m *Manager) BackendForFile(file string) *Backend {
	lang := langFromExt(file)
	if lang == "" {
		return nil
	}
	return m.backends[lang]
}

// DetectAndSpawn checks which backends are available on PATH.
// Returns the number of backends detected.
func (m *Manager) DetectAndSpawn(workspaceRoot string) int {
	detected := 0
	for lang, spec := range m.specs {
		if _, exists := m.backends[lang]; exists {
			continue
		}
		path, err := exec.LookPath(spec.Command)
		if err != nil {
			continue
		}
		m.logger.Info("detected backend LSP", "lang", lang, "command", path)
		detected++
	}
	return detected
}

// Shutdown sends shutdown/exit to all backends.
func (m *Manager) Shutdown() {
	for lang, b := range m.backends {
		m.logger.Info("shutting down backend", "lang", lang)
		_ = b.Notify("shutdown", nil)
		_ = b.Notify("exit", nil)
		if b.stdin != nil {
			b.stdin.Close()
		}
	}
}

// DefaultBackendSpecs returns the built-in backend specifications.
func DefaultBackendSpecs() map[string]BackendSpec {
	return map[string]BackendSpec{
		"go": {
			Command:    "gopls",
			Args:       []string{"serve"},
			Extensions: []string{".go"},
		},
		"python": {
			Command:    "pyright-langserver",
			Args:       []string{"--stdio"},
			Extensions: []string{".py", ".pyi"},
		},
		"typescript": {
			Command:    "typescript-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
		"rust": {
			Command:    "rust-analyzer",
			Extensions: []string{".rs"},
		},
		"c": {
			Command:    "clangd",
			Extensions: []string{".c", ".h", ".cpp", ".hpp", ".cc"},
		},
		"java": {
			Command:    "jdtls",
			Extensions: []string{".java"},
		},
	}
}

func langFromExt(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".go":
		return "go"
	case ".py", ".pyi":
		return "python"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".c", ".h", ".cpp", ".hpp", ".cc":
		return "c"
	case ".java":
		return "java"
	default:
		return ""
	}
}
