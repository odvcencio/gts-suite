package treesitter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"gts-suite/internal/model"
)

type Parser struct {
	entry  grammars.LangEntry
	lang   *gotreesitter.Language
	parser *gotreesitter.Parser
	tagger *gotreesitter.Tagger
}

func NewParser(entry grammars.LangEntry) (*Parser, error) {
	if strings.TrimSpace(entry.Name) == "" {
		return nil, fmt.Errorf("language entry name is required")
	}
	if entry.Language == nil {
		return nil, fmt.Errorf("language loader is required for %q", entry.Name)
	}
	if strings.TrimSpace(entry.TagsQuery) == "" {
		return nil, fmt.Errorf("tags query is required for %q", entry.Name)
	}

	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("language loader returned nil for %q", entry.Name)
	}

	tagger, err := gotreesitter.NewTagger(lang, entry.TagsQuery)
	if err != nil {
		return nil, fmt.Errorf("compile tags query for %q: %w", entry.Name, err)
	}

	return &Parser{
		entry:  entry,
		lang:   lang,
		parser: gotreesitter.NewParser(lang),
		tagger: tagger,
	}, nil
}

func (p *Parser) Language() string {
	return p.entry.Name
}

func (p *Parser) Parse(path string, src []byte) (model.FileSummary, error) {
	summary := model.FileSummary{
		Path:     path,
		Language: p.Language(),
	}
	if len(src) == 0 {
		return summary, nil
	}

	tree := p.parseTree(src)
	if tree == nil || tree.RootNode() == nil {
		return summary, nil
	}
	defer tree.Release()

	tags := p.tagger.TagTree(tree)
	summary.Imports = p.extractImports(tree, src)
	summary.Symbols = p.extractSymbols(src, tags)
	summary.References = p.extractReferences(tags)
	return summary, nil
}

func (p *Parser) parseTree(src []byte) *gotreesitter.Tree {
	if p.entry.TokenSourceFactory != nil {
		ts := p.entry.TokenSourceFactory(src, p.lang)
		if ts != nil {
			return p.parser.ParseWithTokenSource(src, ts)
		}
	}
	return p.parser.Parse(src)
}

func (p *Parser) extractImports(tree *gotreesitter.Tree, src []byte) []string {
	if p.entry.Name != "go" {
		return nil
	}

	imports := map[string]struct{}{}
	gotreesitter.Walk(tree.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node == nil || node.Type(p.lang) != "import_spec" {
			return gotreesitter.WalkContinue
		}
		path := importPathFromSpec(node, p.lang, src)
		if path != "" {
			imports[path] = struct{}{}
		}
		return gotreesitter.WalkContinue
	})

	if len(imports) == 0 {
		return nil
	}
	values := make([]string, 0, len(imports))
	for imp := range imports {
		values = append(values, imp)
	}
	sort.Strings(values)
	return values
}

func (p *Parser) extractSymbols(src []byte, tags []gotreesitter.Tag) []model.Symbol {
	if len(tags) == 0 {
		return nil
	}

	symbols := make([]model.Symbol, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		symbol, ok := symbolFromTag(src, tag)
		if !ok {
			continue
		}

		key := symbol.Kind + "|" + symbol.Name + "|" + strconv.Itoa(symbol.StartLine) + "|" + strconv.Itoa(symbol.EndLine)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		symbols = append(symbols, symbol)
	}

	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].StartLine == symbols[j].StartLine {
			if symbols[i].EndLine == symbols[j].EndLine {
				if symbols[i].Kind == symbols[j].Kind {
					return symbols[i].Name < symbols[j].Name
				}
				return symbols[i].Kind < symbols[j].Kind
			}
			return symbols[i].EndLine < symbols[j].EndLine
		}
		return symbols[i].StartLine < symbols[j].StartLine
	})
	return symbols
}

func (p *Parser) extractReferences(tags []gotreesitter.Tag) []model.Reference {
	if len(tags) == 0 {
		return nil
	}

	references := make([]model.Reference, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		reference, ok := referenceFromTag(tag)
		if !ok {
			continue
		}

		key := reference.Kind + "|" + reference.Name + "|" + strconv.Itoa(reference.StartLine) + "|" + strconv.Itoa(reference.StartColumn)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		references = append(references, reference)
	}

	sort.Slice(references, func(i, j int) bool {
		if references[i].StartLine == references[j].StartLine {
			if references[i].StartColumn == references[j].StartColumn {
				if references[i].Kind == references[j].Kind {
					return references[i].Name < references[j].Name
				}
				return references[i].Kind < references[j].Kind
			}
			return references[i].StartColumn < references[j].StartColumn
		}
		return references[i].StartLine < references[j].StartLine
	})
	return references
}

func symbolFromTag(src []byte, tag gotreesitter.Tag) (model.Symbol, bool) {
	kind, ok := mapTagKind(tag.Kind)
	if !ok {
		return model.Symbol{}, false
	}

	name := strings.TrimSpace(tag.Name)
	if name == "" {
		return model.Symbol{}, false
	}

	start := int(tag.Range.StartPoint.Row) + 1
	end := int(tag.Range.EndPoint.Row) + 1
	if end < start {
		end = start
	}

	signature := summarizeSignature(rawRangeText(src, tag.Range))
	receiver := ""
	if kind == "method_definition" {
		receiver = inferGoReceiver(signature)
	}

	return model.Symbol{
		Kind:      kind,
		Name:      name,
		Signature: signature,
		Receiver:  receiver,
		StartLine: start,
		EndLine:   end,
	}, true
}

func referenceFromTag(tag gotreesitter.Tag) (model.Reference, bool) {
	if !strings.HasPrefix(tag.Kind, "reference.") {
		return model.Reference{}, false
	}

	name := strings.TrimSpace(tag.Name)
	if name == "" {
		return model.Reference{}, false
	}

	startLine := int(tag.NameRange.StartPoint.Row) + 1
	endLine := int(tag.NameRange.EndPoint.Row) + 1
	if endLine < startLine {
		endLine = startLine
	}
	startCol := int(tag.NameRange.StartPoint.Column) + 1
	endCol := int(tag.NameRange.EndPoint.Column) + 1
	if endCol < startCol {
		endCol = startCol
	}

	return model.Reference{
		Kind:        tag.Kind,
		Name:        name,
		StartLine:   startLine,
		EndLine:     endLine,
		StartColumn: startCol,
		EndColumn:   endCol,
	}, true
}

func mapTagKind(tagKind string) (string, bool) {
	if !strings.HasPrefix(tagKind, "definition.") {
		return "", false
	}

	switch strings.TrimPrefix(tagKind, "definition.") {
	case "function", "constructor":
		return "function_definition", true
	case "method":
		return "method_definition", true
	default:
		return "type_definition", true
	}
}

func rawRangeText(src []byte, rng gotreesitter.Range) string {
	if len(src) == 0 {
		return ""
	}

	start := int(rng.StartByte)
	end := int(rng.EndByte)
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(src) {
		start = len(src)
	}
	if end > len(src) {
		end = len(src)
	}
	return string(src[start:end])
}

func summarizeSignature(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}

	if idx := strings.Index(trimmed, "\n"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if idx := strings.Index(trimmed, "{"); idx > 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	trimmed = strings.TrimSuffix(trimmed, ";")
	return strings.Join(strings.Fields(trimmed), " ")
}

func inferGoReceiver(signature string) string {
	const prefix = "func ("
	if !strings.HasPrefix(signature, prefix) {
		return ""
	}
	rest := signature[len(prefix):]
	closing := strings.Index(rest, ")")
	if closing <= 0 {
		return ""
	}
	return strings.TrimSpace(rest[:closing])
}

func importPathFromSpec(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	if node == nil {
		return ""
	}

	for i := node.ChildCount() - 1; i >= 0; i-- {
		child := node.Child(i)
		typeName := child.Type(lang)
		if typeName != "interpreted_string_literal" && typeName != "raw_string_literal" {
			continue
		}
		text := strings.TrimSpace(child.Text(src))
		text = strings.Trim(text, "\"`")
		if text != "" {
			return text
		}
	}
	return ""
}
