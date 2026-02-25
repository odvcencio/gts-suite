package scope

import "testing"

func TestResolveLocalVariable(t *testing.T) {
	// func main() { x := 1; print(x) }
	file := NewScope(ScopeFile, nil)
	fn := NewScope(ScopeFunction, file)
	fn.AddDef(Definition{Name: "x", Kind: DefVariable})
	fn.AddRef(Ref{Name: "x"})

	ResolveAll(file)

	if fn.Refs[0].Resolved == nil {
		t.Fatal("x should resolve to local def")
	}
	if fn.Refs[0].Resolved.Name != "x" {
		t.Errorf("expected resolved to 'x', got %q", fn.Refs[0].Resolved.Name)
	}
}

func TestResolveFromParentScope(t *testing.T) {
	// package-level func used inside nested func
	file := NewScope(ScopeFile, nil)
	file.AddDef(Definition{Name: "helper", Kind: DefFunction})
	fn := NewScope(ScopeFunction, file)
	fn.AddRef(Ref{Name: "helper"})

	ResolveAll(file)

	if fn.Refs[0].Resolved == nil {
		t.Fatal("helper should resolve from parent scope")
	}
}

func TestResolveInnermostWins(t *testing.T) {
	// Inner x shadows outer x
	file := NewScope(ScopeFile, nil)
	file.AddDef(Definition{Name: "x", Kind: DefVariable, Loc: Location{StartLine: 1}})
	fn := NewScope(ScopeFunction, file)
	fn.AddDef(Definition{Name: "x", Kind: DefVariable, Loc: Location{StartLine: 5}})
	fn.AddRef(Ref{Name: "x"})

	ResolveAll(file)

	if fn.Refs[0].Resolved == nil {
		t.Fatal("x should resolve")
	}
	if fn.Refs[0].Resolved.Loc.StartLine != 5 {
		t.Error("inner x should shadow outer x")
	}
}

func TestResolveUnresolved(t *testing.T) {
	file := NewScope(ScopeFile, nil)
	file.AddRef(Ref{Name: "nonexistent"})

	ResolveAll(file)

	if file.Refs[0].Resolved != nil {
		t.Error("nonexistent should remain unresolved")
	}
}

func TestResolveCrossFile(t *testing.T) {
	g := NewGraph()

	// File a.go defines Foo
	a := NewScope(ScopeFile, nil)
	a.AddDef(Definition{Name: "Foo", Kind: DefFunction, Loc: Location{File: "a.go"}})
	g.AddFileScope("a.go", a)

	// File b.go references Foo (same package)
	b := NewScope(ScopeFile, nil)
	b.AddRef(Ref{Name: "Foo"})
	g.AddFileScope("b.go", b)

	// Package scope aggregates both files
	pkg := NewScope(ScopePackage, nil)
	pkg.AddDef(Definition{Name: "Foo", Kind: DefFunction, Loc: Location{File: "a.go"}})
	g.AddPackageScope("main", pkg)
	b.Parent = pkg

	ResolveAll(b)

	if b.Refs[0].Resolved == nil {
		t.Fatal("Foo should resolve from package scope")
	}
}

func TestResolveDottedAccess(t *testing.T) {
	g := NewGraph()

	// fmt package has Println
	fmtScope := NewScope(ScopePackage, nil)
	fmtScope.AddDef(Definition{Name: "Println", Kind: DefFunction})
	g.AddPackageScope("fmt", fmtScope)

	// main.go imports fmt, calls fmt.Println
	file := NewScope(ScopeFile, nil)
	fmtDef := Definition{Name: "fmt", Kind: DefImport, ImportPath: "fmt"}
	fmtDef.Scope = fmtScope // import points to package scope
	file.AddDef(fmtDef)
	file.AddRef(Ref{Name: "fmt", Member: "Println"})
	g.AddFileScope("main.go", file)

	ResolveAll(file)

	if file.Refs[0].Resolved == nil {
		t.Fatal("fmt.Println should resolve")
	}
	if file.Refs[0].Resolved.Name != "Println" {
		t.Errorf("expected Println, got %q", file.Refs[0].Resolved.Name)
	}
}
