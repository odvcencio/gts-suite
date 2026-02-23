package scope

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gts-suite/internal/model"
)

type Options struct {
	FilePath string
	Line     int
}

type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Detail   string `json:"detail,omitempty"`
	DeclLine int    `json:"decl_line"`
}

type Report struct {
	File    string        `json:"file"`
	Line    int           `json:"line"`
	Package string        `json:"package"`
	Focus   *model.Symbol `json:"focus,omitempty"`
	Symbols []Symbol      `json:"symbols,omitempty"`
}

func Build(idx *model.Index, opts Options) (Report, error) {
	if idx == nil {
		return Report{}, fmt.Errorf("index is nil")
	}
	if strings.TrimSpace(opts.FilePath) == "" {
		return Report{}, fmt.Errorf("file path is required")
	}
	if opts.Line <= 0 {
		opts.Line = 1
	}

	relPath, absPath, err := resolvePaths(idx.Root, opts.FilePath)
	if err != nil {
		return Report{}, err
	}
	if strings.ToLower(filepath.Ext(absPath)) != ".go" {
		return Report{}, fmt.Errorf("gtsscope currently supports Go files only")
	}

	fileSummary, err := findFileSummary(idx, relPath)
	if err != nil {
		return Report{}, err
	}

	source, err := os.ReadFile(absPath)
	if err != nil {
		return Report{}, err
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absPath, source, parser.SkipObjectResolution)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		File:    fileSummary.Path,
		Line:    opts.Line,
		Package: parsed.Name.Name,
	}

	focus := findFocusSymbol(fileSummary.Symbols, opts.Line)
	if focus != nil {
		focusCopy := *focus
		report.Focus = &focusCopy
	}

	collector := newSymbolCollector()
	addImports(collector, fset, parsed.Imports)
	addIndexedPackageSymbols(collector, idx, fileSummary.Path)
	addPackageDecls(collector, fset, parsed.Decls, opts.Line)

	if fn := findContainingFunction(fset, parsed, opts.Line); fn != nil {
		addFunctionScope(collector, fset, fn, opts.Line)
	}

	report.Symbols = collector.symbols()
	return report, nil
}

func resolvePaths(root, inputPath string) (string, string, error) {
	cleaned := filepath.Clean(inputPath)
	candidate := cleaned
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}

	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}

	rel, relErr := filepath.Rel(rootAbs, absolute)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		rel = cleaned
	}

	return filepath.ToSlash(rel), absolute, nil
}

func findFileSummary(idx *model.Index, relPath string) (model.FileSummary, error) {
	normalized := filepath.ToSlash(filepath.Clean(relPath))
	for _, file := range idx.Files {
		if filepath.ToSlash(filepath.Clean(file.Path)) == normalized {
			return file, nil
		}
	}
	return model.FileSummary{}, fmt.Errorf("file %q not found in index", relPath)
}

func findFocusSymbol(symbols []model.Symbol, line int) *model.Symbol {
	for i := range symbols {
		symbol := symbols[i]
		if line >= symbol.StartLine && line <= symbol.EndLine {
			return &symbols[i]
		}
	}
	return nil
}

func addImports(collector *symbolCollector, fset *token.FileSet, imports []*ast.ImportSpec) {
	for _, imp := range imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := ""
		switch {
		case imp.Name == nil:
			name = importBase(path)
		case imp.Name.Name == "_":
			continue
		default:
			name = imp.Name.Name
		}

		kind := "import"
		if name == "." {
			kind = "import_dot"
		}
		collector.add(name, kind, path, lineAt(fset, imp.Pos()))
	}
}

func addIndexedPackageSymbols(collector *symbolCollector, idx *model.Index, filePath string) {
	dir := filepath.ToSlash(filepath.Dir(filepath.Clean(filePath)))
	currentIsTest := strings.HasSuffix(filepath.ToSlash(filepath.Clean(filePath)), "_test.go")
	for _, file := range idx.Files {
		fileDir := filepath.ToSlash(filepath.Dir(filepath.Clean(file.Path)))
		if fileDir != dir {
			continue
		}
		if strings.HasSuffix(filepath.ToSlash(filepath.Clean(file.Path)), "_test.go") != currentIsTest {
			continue
		}
		for _, symbol := range file.Symbols {
			switch symbol.Kind {
			case "function_definition":
				collector.add(symbol.Name, "package_function", symbol.Signature, symbol.StartLine)
			case "type_definition":
				collector.add(symbol.Name, "package_type", symbol.Signature, symbol.StartLine)
			}
		}
	}
}

func addPackageDecls(collector *symbolCollector, fset *token.FileSet, decls []ast.Decl, line int) {
	for _, decl := range decls {
		if lineAt(fset, decl.Pos()) > line {
			break
		}
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			collector.add(d.Name.Name, "package_function", functionSignature(d), lineAt(fset, d.Pos()))
		case *ast.GenDecl:
			addGenDeclNames(collector, fset, d, "package")
		}
	}
}

func addFunctionScope(collector *symbolCollector, fset *token.FileSet, fn *ast.FuncDecl, line int) {
	if fn.Recv != nil {
		addNamedFields(collector, fset, fn.Recv, "receiver")
	}
	if fn.Type != nil {
		addNamedFields(collector, fset, fn.Type.TypeParams, "type_param")
		addNamedFields(collector, fset, fn.Type.Params, "param")
		addNamedFields(collector, fset, fn.Type.Results, "result")
	}
	if fn.Body != nil {
		collectBlockScope(collector, fset, fn.Body, line)
	}
}

func findContainingFunction(fset *token.FileSet, file *ast.File, line int) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := lineAt(fset, fn.Body.Pos())
		end := lineAt(fset, fn.Body.End())
		if line >= start && line <= end {
			return fn
		}
	}
	return nil
}

func collectBlockScope(collector *symbolCollector, fset *token.FileSet, block *ast.BlockStmt, line int) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		start := lineAt(fset, stmt.Pos())
		if start > line {
			break
		}

		end := lineAt(fset, stmt.End())
		if line > end {
			addStmtDeclsVisibleAfter(collector, fset, stmt)
			continue
		}

		addStmtDeclsInside(collector, fset, stmt, line)
		return
	}
}

func addStmtDeclsVisibleAfter(collector *symbolCollector, fset *token.FileSet, stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			addGenDeclNames(collector, fset, decl, "local")
		}
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			addAssignNames(collector, fset, s, "local_var")
		}
	case *ast.LabeledStmt:
		addStmtDeclsVisibleAfter(collector, fset, s.Stmt)
	}
}

func addStmtDeclsInside(collector *symbolCollector, fset *token.FileSet, stmt ast.Stmt, line int) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		collectBlockScope(collector, fset, s, line)
	case *ast.DeclStmt:
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			addGenDeclNames(collector, fset, decl, "local")
		}
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			addAssignNames(collector, fset, s, "local_var")
		}
	case *ast.IfStmt:
		if s.Init != nil {
			addStmtDeclsVisibleAfter(collector, fset, s.Init)
		}
		if s.Body != nil {
			start, end := lineAt(fset, s.Body.Pos()), lineAt(fset, s.Body.End())
			if line >= start && line <= end {
				collectBlockScope(collector, fset, s.Body, line)
				return
			}
		}
		if s.Else != nil {
			start, end := lineAt(fset, s.Else.Pos()), lineAt(fset, s.Else.End())
			if line >= start && line <= end {
				addStmtDeclsInside(collector, fset, s.Else, line)
				return
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			addStmtDeclsVisibleAfter(collector, fset, s.Init)
		}
		if s.Body != nil {
			collectBlockScope(collector, fset, s.Body, line)
		}
	case *ast.RangeStmt:
		if s.Tok == token.DEFINE {
			addRangeNames(collector, fset, s, "local_var")
		}
		if s.Body != nil {
			collectBlockScope(collector, fset, s.Body, line)
		}
	case *ast.SwitchStmt:
		if s.Init != nil {
			addStmtDeclsVisibleAfter(collector, fset, s.Init)
		}
		collectCaseClauses(collector, fset, s.Body, line)
	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			addStmtDeclsVisibleAfter(collector, fset, s.Init)
		}
		collectCaseClauses(collector, fset, s.Body, line)
	case *ast.SelectStmt:
		collectCommClauses(collector, fset, s.Body, line)
	case *ast.LabeledStmt:
		addStmtDeclsInside(collector, fset, s.Stmt, line)
	}
}

func collectCaseClauses(collector *symbolCollector, fset *token.FileSet, block *ast.BlockStmt, line int) {
	if block == nil {
		return
	}
	for _, entry := range block.List {
		clause, ok := entry.(*ast.CaseClause)
		if !ok {
			continue
		}
		start, end := lineAt(fset, clause.Pos()), lineAt(fset, clause.End())
		if line < start || line > end {
			continue
		}
		collectStmtList(collector, fset, clause.Body, line)
		return
	}
}

func collectCommClauses(collector *symbolCollector, fset *token.FileSet, block *ast.BlockStmt, line int) {
	if block == nil {
		return
	}
	for _, entry := range block.List {
		clause, ok := entry.(*ast.CommClause)
		if !ok {
			continue
		}
		start, end := lineAt(fset, clause.Pos()), lineAt(fset, clause.End())
		if line < start || line > end {
			continue
		}
		if clause.Comm != nil {
			addStmtDeclsVisibleAfter(collector, fset, clause.Comm)
		}
		collectStmtList(collector, fset, clause.Body, line)
		return
	}
}

func collectStmtList(collector *symbolCollector, fset *token.FileSet, stmts []ast.Stmt, line int) {
	for _, stmt := range stmts {
		start := lineAt(fset, stmt.Pos())
		if start > line {
			break
		}
		end := lineAt(fset, stmt.End())
		if line > end {
			addStmtDeclsVisibleAfter(collector, fset, stmt)
			continue
		}
		addStmtDeclsInside(collector, fset, stmt, line)
		return
	}
}

func addNamedFields(collector *symbolCollector, fset *token.FileSet, fields *ast.FieldList, kind string) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			continue
		}
		detail := exprString(field.Type)
		for _, name := range field.Names {
			collector.add(name.Name, kind, detail, lineAt(fset, name.Pos()))
		}
	}
}

func addGenDeclNames(collector *symbolCollector, fset *token.FileSet, decl *ast.GenDecl, scope string) {
	if decl == nil {
		return
	}
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			kind := scope + "_type"
			collector.add(s.Name.Name, kind, typeSignature(s), lineAt(fset, s.Name.Pos()))
		case *ast.ValueSpec:
			kind := scope + "_value"
			switch decl.Tok {
			case token.VAR:
				kind = scope + "_var"
			case token.CONST:
				kind = scope + "_const"
			}
			detail := exprString(s.Type)
			for _, name := range s.Names {
				collector.add(name.Name, kind, detail, lineAt(fset, name.Pos()))
			}
		}
	}
}

func addAssignNames(collector *symbolCollector, fset *token.FileSet, stmt *ast.AssignStmt, kind string) {
	for _, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		collector.add(ident.Name, kind, "", lineAt(fset, ident.Pos()))
	}
}

func addRangeNames(collector *symbolCollector, fset *token.FileSet, stmt *ast.RangeStmt, kind string) {
	if stmt.Key != nil {
		if ident, ok := stmt.Key.(*ast.Ident); ok {
			collector.add(ident.Name, kind, "", lineAt(fset, ident.Pos()))
		}
	}
	if stmt.Value != nil {
		if ident, ok := stmt.Value.(*ast.Ident); ok {
			collector.add(ident.Name, kind, "", lineAt(fset, ident.Pos()))
		}
	}
}

func lineAt(fset *token.FileSet, pos token.Pos) int {
	if !pos.IsValid() {
		return 0
	}
	return fset.Position(pos).Line
}

func importBase(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

func functionSignature(fn *ast.FuncDecl) string {
	var builder strings.Builder
	builder.WriteString("func ")

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		builder.WriteString("(")
		builder.WriteString(fieldString(fn.Recv.List[0], true))
		builder.WriteString(") ")
	}

	builder.WriteString(fn.Name.Name)
	builder.WriteString("(")
	builder.WriteString(fieldListString(fn.Type.Params, true))
	builder.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		results := fieldListString(fn.Type.Results, true)
		if len(fn.Type.Results.List) == 1 && len(fn.Type.Results.List[0].Names) == 0 {
			builder.WriteString(" ")
			builder.WriteString(results)
		} else {
			builder.WriteString(" (")
			builder.WriteString(results)
			builder.WriteString(")")
		}
	}

	return builder.String()
}

func typeSignature(typeSpec *ast.TypeSpec) string {
	var builder strings.Builder
	builder.WriteString("type ")
	builder.WriteString(typeSpec.Name.Name)
	if typeSpec.Type != nil {
		builder.WriteString(" ")
		builder.WriteString(exprString(typeSpec.Type))
	}
	return builder.String()
}

func fieldListString(list *ast.FieldList, includeNames bool) string {
	if list == nil || len(list.List) == 0 {
		return ""
	}

	parts := make([]string, 0, len(list.List))
	for _, field := range list.List {
		parts = append(parts, fieldString(field, includeNames))
	}
	return strings.Join(parts, ", ")
}

func fieldString(field *ast.Field, includeNames bool) string {
	typeText := exprString(field.Type)
	if !includeNames || len(field.Names) == 0 {
		return typeText
	}

	names := make([]string, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	return strings.Join(names, ", ") + " " + typeText
}

func exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buffer bytes.Buffer
	_ = printer.Fprint(&buffer, token.NewFileSet(), expr)
	return buffer.String()
}

type symbolCollector struct {
	items  []Symbol
	byName map[string]int
}

func newSymbolCollector() *symbolCollector {
	return &symbolCollector{
		items:  make([]Symbol, 0, 32),
		byName: make(map[string]int),
	}
}

func (c *symbolCollector) add(name, kind, detail string, line int) {
	name = strings.TrimSpace(name)
	if name == "" || name == "_" {
		return
	}
	symbol := Symbol{
		Name:     name,
		Kind:     kind,
		Detail:   strings.TrimSpace(detail),
		DeclLine: line,
	}
	if idx, ok := c.byName[name]; ok {
		c.items[idx] = symbol
		return
	}
	c.byName[name] = len(c.items)
	c.items = append(c.items, symbol)
}

func (c *symbolCollector) symbols() []Symbol {
	out := make([]Symbol, 0, len(c.items))
	for _, symbol := range c.items {
		if strings.TrimSpace(symbol.Name) == "" {
			continue
		}
		out = append(out, symbol)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeclLine == out[j].DeclLine {
			if out[i].Name == out[j].Name {
				return out[i].Kind < out[j].Kind
			}
			return out[i].Name < out[j].Name
		}
		return out[i].DeclLine < out[j].DeclLine
	})
	return out
}
