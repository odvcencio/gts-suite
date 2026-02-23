package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"gts-suite/internal/model"
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Language() string {
	return "go"
}

func (p *Parser) Parse(path string, src []byte) (model.FileSummary, error) {
	summary, err := p.parseWithTreeSitter(path, src)
	if err == nil {
		return summary, nil
	}
	return p.parseWithGoAST(path, src)
}

func (p *Parser) parseWithTreeSitter(path string, src []byte) (model.FileSummary, error) {
	entry := grammars.DetectLanguage(path)
	if entry == nil || entry.Name != p.Language() {
		return model.FileSummary{}, fmt.Errorf("unsupported go source: %s", path)
	}

	bound, err := grammars.ParseFile(path, src)
	if err != nil {
		return model.FileSummary{}, err
	}
	defer bound.Release()

	root := bound.RootNode()
	if root == nil {
		return model.FileSummary{}, fmt.Errorf("gotreesitter returned nil root")
	}
	rootType := bound.NodeType(root)
	// Some parse failures currently return an empty root type with no children.
	if rootType == "" || (len(bytes.TrimSpace(src)) > 0 && root.ChildCount() == 0) {
		return model.FileSummary{}, fmt.Errorf("gotreesitter parse did not produce a valid go root")
	}

	summary := model.FileSummary{
		Path:     path,
		Language: p.Language(),
	}

	imports := map[string]struct{}{}
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		switch bound.NodeType(node) {
		case "import_spec":
			path := importPathFromSpec(bound, node)
			if path != "" {
				imports[path] = struct{}{}
			}
		case "function_declaration":
			symbol, ok := functionSymbolFromNode(bound, node)
			if ok {
				summary.Symbols = append(summary.Symbols, symbol)
			}
		case "method_declaration":
			symbol, ok := methodSymbolFromNode(bound, node)
			if ok {
				summary.Symbols = append(summary.Symbols, symbol)
			}
		case "type_spec", "type_alias":
			symbol, ok := typeSymbolFromNode(bound, node)
			if ok {
				summary.Symbols = append(summary.Symbols, symbol)
			}
		}
		return gotreesitter.WalkContinue
	})

	if len(imports) > 0 {
		summary.Imports = make([]string, 0, len(imports))
		for imp := range imports {
			summary.Imports = append(summary.Imports, imp)
		}
		sort.Strings(summary.Imports)
	}

	sort.Slice(summary.Symbols, func(i, j int) bool {
		if summary.Symbols[i].StartLine == summary.Symbols[j].StartLine {
			return summary.Symbols[i].Name < summary.Symbols[j].Name
		}
		return summary.Symbols[i].StartLine < summary.Symbols[j].StartLine
	})

	return summary, nil
}

func (p *Parser) parseWithGoAST(path string, src []byte) (model.FileSummary, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return model.FileSummary{}, err
	}

	summary := model.FileSummary{
		Path:     path,
		Language: p.Language(),
	}

	for _, imp := range file.Imports {
		summary.Imports = append(summary.Imports, strings.Trim(imp.Path.Value, `"`))
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			kind := "function_definition"
			receiver := ""
			if n.Recv != nil && len(n.Recv.List) > 0 {
				kind = "method_definition"
				receiver = fieldString(n.Recv.List[0], true)
			}

			start, end := lineSpan(fset, n)
			summary.Symbols = append(summary.Symbols, model.Symbol{
				Kind:      kind,
				Name:      n.Name.Name,
				Signature: functionSignature(n),
				Receiver:  receiver,
				StartLine: start,
				EndLine:   end,
			})
			return false
		case *ast.GenDecl:
			if n.Tok != token.TYPE {
				return true
			}

			for _, spec := range n.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				start, end := lineSpan(fset, typeSpec)
				summary.Symbols = append(summary.Symbols, model.Symbol{
					Kind:      "type_definition",
					Name:      typeSpec.Name.Name,
					Signature: typeSignature(typeSpec),
					StartLine: start,
					EndLine:   end,
				})
			}
			return false
		default:
			return true
		}
	})

	sort.Strings(summary.Imports)
	sort.Slice(summary.Symbols, func(i, j int) bool {
		if summary.Symbols[i].StartLine == summary.Symbols[j].StartLine {
			return summary.Symbols[i].Name < summary.Symbols[j].Name
		}
		return summary.Symbols[i].StartLine < summary.Symbols[j].StartLine
	})

	return summary, nil
}

func importPathFromSpec(bound *gotreesitter.BoundTree, node *gotreesitter.Node) string {
	if node == nil {
		return ""
	}

	for i := node.ChildCount() - 1; i >= 0; i-- {
		child := node.Child(i)
		typeName := bound.NodeType(child)
		if typeName != "interpreted_string_literal" && typeName != "raw_string_literal" {
			continue
		}
		text := strings.TrimSpace(bound.NodeText(child))
		text = strings.Trim(text, "\"`")
		if text != "" {
			return text
		}
	}
	return ""
}

func functionSymbolFromNode(bound *gotreesitter.BoundTree, node *gotreesitter.Node) (model.Symbol, bool) {
	name := ""
	typeParams := ""
	params := ""
	result := ""
	seenName := false

	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		typeName := bound.NodeType(child)
		if !child.IsNamed() {
			continue
		}

		switch {
		case (typeName == "identifier" || typeName == "field_identifier") && name == "":
			name = normalizeInline(bound.NodeText(child))
			seenName = true
		case typeName == "type_parameter_list" && seenName && typeParams == "":
			typeParams = normalizeInline(bound.NodeText(child))
		case typeName == "parameter_list" && seenName && params == "":
			params = normalizeInline(bound.NodeText(child))
		case typeName != "block" && seenName && params != "" && result == "":
			result = normalizeInline(bound.NodeText(child))
		}
	}

	if name == "" {
		return model.Symbol{}, false
	}

	start, end := lineSpanNode(node)
	signature := "func " + name
	if typeParams != "" {
		signature += typeParams
	}
	if params == "" {
		signature += "()"
	} else {
		signature += params
	}
	if result != "" {
		signature += " " + result
	}

	return model.Symbol{
		Kind:      "function_definition",
		Name:      name,
		Signature: signature,
		StartLine: start,
		EndLine:   end,
	}, true
}

func methodSymbolFromNode(bound *gotreesitter.BoundTree, node *gotreesitter.Node) (model.Symbol, bool) {
	receiver := ""
	name := ""
	typeParams := ""
	params := ""
	result := ""
	seenName := false

	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		typeName := bound.NodeType(child)
		if !child.IsNamed() {
			continue
		}

		switch {
		case typeName == "parameter_list" && !seenName && receiver == "":
			receiver = normalizeInline(bound.NodeText(child))
		case (typeName == "field_identifier" || typeName == "identifier") && name == "":
			name = normalizeInline(bound.NodeText(child))
			seenName = true
		case typeName == "type_parameter_list" && seenName && typeParams == "":
			typeParams = normalizeInline(bound.NodeText(child))
		case typeName == "parameter_list" && seenName && params == "":
			params = normalizeInline(bound.NodeText(child))
		case typeName != "block" && seenName && params != "" && result == "":
			result = normalizeInline(bound.NodeText(child))
		}
	}

	if name == "" {
		return model.Symbol{}, false
	}

	start, end := lineSpanNode(node)
	signature := "func "
	if receiver != "" {
		signature += receiver + " "
	}
	signature += name
	if typeParams != "" {
		signature += typeParams
	}
	if params == "" {
		signature += "()"
	} else {
		signature += params
	}
	if result != "" {
		signature += " " + result
	}

	return model.Symbol{
		Kind:      "method_definition",
		Name:      name,
		Signature: signature,
		Receiver:  receiver,
		StartLine: start,
		EndLine:   end,
	}, true
}

func typeSymbolFromNode(bound *gotreesitter.BoundTree, node *gotreesitter.Node) (model.Symbol, bool) {
	name := ""
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if !child.IsNamed() {
			continue
		}
		typeName := bound.NodeType(child)
		if typeName == "type_identifier" || typeName == "identifier" {
			name = normalizeInline(bound.NodeText(child))
			break
		}
	}
	if name == "" {
		return model.Symbol{}, false
	}

	start, end := lineSpanNode(node)
	signature := "type " + normalizeInline(bound.NodeText(node))
	return model.Symbol{
		Kind:      "type_definition",
		Name:      name,
		Signature: signature,
		StartLine: start,
		EndLine:   end,
	}, true
}

func lineSpanNode(node *gotreesitter.Node) (int, int) {
	if node == nil {
		return 0, 0
	}
	start := int(node.StartPoint().Row) + 1
	end := int(node.EndPoint().Row) + 1
	if end < start {
		end = start
	}
	return start, end
}

func normalizeInline(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func lineSpan(fset *token.FileSet, node ast.Node) (int, int) {
	if node == nil {
		return 0, 0
	}
	start := fset.Position(node.Pos()).Line
	end := fset.Position(node.End()).Line
	return start, end
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
