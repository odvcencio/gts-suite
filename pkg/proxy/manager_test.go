package proxy

import (
	"log/slog"
	"testing"
)

func TestManagerBackendForLang(t *testing.T) {
	m := NewManager(slog.Default())

	if b := m.BackendForLang("go"); b != nil {
		t.Error("expected nil backend before registration")
	}

	b := &Backend{Name: "gopls", Lang: "go", ready: make(chan struct{})}
	close(b.ready)
	m.Register(b)

	if got := m.BackendForLang("go"); got == nil {
		t.Error("expected backend after registration")
	} else if got.Name != "gopls" {
		t.Errorf("backend.Name = %q, want gopls", got.Name)
	}

	if m.BackendForLang("python") != nil {
		t.Error("expected nil for unregistered language")
	}
}

func TestManagerBackendForFile(t *testing.T) {
	m := NewManager(slog.Default())
	b := &Backend{Name: "gopls", Lang: "go", ready: make(chan struct{})}
	close(b.ready)
	m.Register(b)

	if got := m.BackendForFile("main.go"); got == nil || got.Name != "gopls" {
		t.Error("expected gopls for .go file")
	}
	if m.BackendForFile("main.py") != nil {
		t.Error("expected nil for .py file")
	}
}

func TestDefaultBackendSpecs(t *testing.T) {
	specs := DefaultBackendSpecs()
	if len(specs) == 0 {
		t.Fatal("expected default backend specs")
	}
	goSpec, ok := specs["go"]
	if !ok {
		t.Fatal("missing go spec")
	}
	if goSpec.Command != "gopls" {
		t.Errorf("go command = %q, want gopls", goSpec.Command)
	}
}
