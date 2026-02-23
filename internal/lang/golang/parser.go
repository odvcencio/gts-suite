package golang

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"

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
