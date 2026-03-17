// Package lint evaluates structural linting rules and tree-sitter query patterns against a parsed index.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/odvcencio/gts-suite/pkg/complexity"
	"github.com/odvcencio/gts-suite/pkg/model"
	"github.com/odvcencio/gts-suite/pkg/xref"
)

var maxLinesRulePattern = regexp.MustCompile(`(?i)^\s*no\s+([a-z_]+)s?\s+longer\s+than\s+(\d+)\s+lines?\s*$`)
var noImportRulePattern = regexp.MustCompile(`(?i)^\s*no\s+import\s+(.+?)\s*$`)

type Rule struct {
	ID         string `json:"id"`
	Raw        string `json:"raw"`
	Type       string `json:"type"`
	Kind       string `json:"kind,omitempty"`
	KindLabel  string `json:"kind_label,omitempty"`
	MaxLines   int    `json:"max_lines,omitempty"`
	ImportPath string `json:"import_path,omitempty"`
}

type QueryPattern struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Query   string `json:"query"`
	Message string `json:"message,omitempty"`
}

type Violation struct {
	RuleID    string `json:"rule_id"`
	File      string `json:"file"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Span      int    `json:"span"`
	Message   string `json:"message"`
	Severity  string `json:"severity,omitempty"`
	Value     int    `json:"value,omitempty"`
}

// ThresholdRule expresses a simple metric > N threshold check.
type ThresholdRule struct {
	ID        string `json:"id"`
	Metric    string `json:"metric"`
	Threshold int    `json:"threshold"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
}

// DefaultRules is the built-in set of threshold-based lint rules.
var DefaultRules = []ThresholdRule{
	{ID: "complexity/cyclomatic", Metric: "cyclomatic", Threshold: 25, Severity: "warn", Message: "function too complex"},
	{ID: "complexity/cognitive", Metric: "cognitive", Threshold: 50, Severity: "warn", Message: "hard to reason about"},
	{ID: "complexity/lines", Metric: "lines", Threshold: 200, Severity: "warn", Message: "function too long"},
	{ID: "complexity/nesting", Metric: "nesting", Threshold: 5, Severity: "warn", Message: "deeply nested"},
	{ID: "complexity/params", Metric: "params", Threshold: 7, Severity: "warn", Message: "too many parameters"},
	{ID: "architecture/fan-in", Metric: "fan_in", Threshold: 30, Severity: "warn", Message: "chokepoint risk"},
	{ID: "architecture/fan-out", Metric: "fan_out", Threshold: 15, Severity: "warn", Message: "too many dependencies"},
}

// EvaluateThresholds checks every function in the index against the given threshold rules.
// It runs complexity analysis and xref graph building to gather per-function metrics,
// then compares each metric against each rule's threshold.
func EvaluateThresholds(idx *model.Index, rules []ThresholdRule) ([]Violation, error) {
	if idx == nil || len(rules) == 0 {
		return nil, nil
	}

	report, err := complexity.Analyze(idx, idx.Root, complexity.Options{})
	if err != nil {
		return nil, fmt.Errorf("complexity analysis: %w", err)
	}

	graph, err := xref.Build(idx)
	if err != nil {
		return nil, fmt.Errorf("xref build: %w", err)
	}
	complexity.EnrichWithXref(report, graph)

	violations := make([]Violation, 0, 32)
	for _, fn := range report.Functions {
		for _, rule := range rules {
			value, ok := thresholdMetricValue(fn, rule.Metric)
			if !ok {
				continue
			}
			if value <= rule.Threshold {
				continue
			}
			violations = append(violations, Violation{
				RuleID:    rule.ID,
				File:      fn.File,
				Kind:      fn.Kind,
				Name:      fn.Name,
				StartLine: fn.StartLine,
				EndLine:   fn.EndLine,
				Span:      fn.Lines,
				Message:   fmt.Sprintf("%s (%s=%d, threshold=%d)", rule.Message, rule.Metric, value, rule.Threshold),
				Severity:  rule.Severity,
				Value:     value,
			})
		}
	}

	sortViolations(violations)
	return violations, nil
}

// thresholdMetricValue extracts the named metric from function metrics.
func thresholdMetricValue(fn complexity.FunctionMetrics, metric string) (int, bool) {
	switch metric {
	case "cyclomatic":
		return fn.Cyclomatic, true
	case "cognitive":
		return fn.Cognitive, true
	case "lines":
		return fn.Lines, true
	case "nesting":
		return fn.MaxNesting, true
	case "params":
		return fn.Parameters, true
	case "fan_in":
		return fn.FanIn, true
	case "fan_out":
		return fn.FanOut, true
	default:
		return 0, false
	}
}

// ParseThresholdOverride parses a string like "cyclomatic=35" and applies it
// to the given rules slice, modifying the matching rule's threshold in place.
func ParseThresholdOverride(override string, rules []ThresholdRule) error {
	parts := strings.SplitN(strings.TrimSpace(override), "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid threshold override %q: expected metric=value", override)
	}
	metric := strings.TrimSpace(parts[0])
	valueStr := strings.TrimSpace(parts[1])

	value, err := strconv.Atoi(valueStr)
	if err != nil || value < 0 {
		return fmt.Errorf("invalid threshold value in %q: must be a non-negative integer", override)
	}

	found := false
	for i := range rules {
		if rules[i].Metric == metric {
			rules[i].Threshold = value
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown metric %q in threshold override (valid: cyclomatic, cognitive, lines, nesting, params, fan_in, fan_out)", metric)
	}
	return nil
}

func ParseRule(raw string) (Rule, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Rule{}, fmt.Errorf("rule cannot be empty")
	}

	matches := maxLinesRulePattern.FindStringSubmatch(text)
	if matches != nil {
		kind, kindLabel, err := normalizeRuleKind(matches[1])
		if err != nil {
			return Rule{}, err
		}

		maxLines, err := strconv.Atoi(matches[2])
		if err != nil || maxLines <= 0 {
			return Rule{}, fmt.Errorf("invalid max line count in rule %q", raw)
		}

		return Rule{
			ID:        fmt.Sprintf("max-lines:%s:%d", kind, maxLines),
			Raw:       text,
			Type:      "max_lines",
			Kind:      kind,
			KindLabel: kindLabel,
			MaxLines:  maxLines,
		}, nil
	}

	matches = noImportRulePattern.FindStringSubmatch(text)
	if matches != nil {
		importPath := strings.TrimSpace(matches[1])
		importPath = strings.Trim(importPath, `"'`)
		if importPath == "" {
			return Rule{}, fmt.Errorf("import path cannot be empty in rule %q", raw)
		}
		return Rule{
			ID:         fmt.Sprintf("no-import:%s", importPath),
			Raw:        text,
			Type:       "no_import",
			ImportPath: importPath,
		}, nil
	}
	return Rule{}, fmt.Errorf("unsupported rule %q", raw)
}

func normalizeRuleKind(kind string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "function", "func", "function_definition":
		return "function_definition", "function", nil
	case "method", "method_definition":
		return "method_definition", "method", nil
	case "type", "type_definition":
		return "type_definition", "type", nil
	case "symbol", "any", "all", "*":
		return "*", "symbol", nil
	default:
		return "", "", fmt.Errorf("unsupported rule target %q", kind)
	}
}

func LoadQueryPattern(path string) (QueryPattern, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return QueryPattern{}, fmt.Errorf("pattern path cannot be empty")
	}

	source, err := os.ReadFile(cleaned)
	if err != nil {
		return QueryPattern{}, err
	}

	queryText := strings.TrimSpace(string(source))
	if queryText == "" {
		return QueryPattern{}, fmt.Errorf("pattern %q is empty", cleaned)
	}

	id := "query-pattern:" + filepath.ToSlash(filepath.Clean(cleaned))
	message := ""
	for _, line := range strings.Split(queryText, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, ";") {
			continue
		}
		meta := strings.TrimSpace(strings.TrimPrefix(trimmed, ";"))
		switch {
		case strings.HasPrefix(strings.ToLower(meta), "id:"):
			value := strings.TrimSpace(meta[len("id:"):])
			if value != "" {
				id = value
			}
		case strings.HasPrefix(strings.ToLower(meta), "message:"):
			value := strings.TrimSpace(meta[len("message:"):])
			if value != "" {
				message = value
			}
		}
	}

	return QueryPattern{
		ID:      id,
		Path:    filepath.ToSlash(filepath.Clean(cleaned)),
		Query:   queryText,
		Message: message,
	}, nil
}

func Evaluate(idx *model.Index, rules []Rule) []Violation {
	if idx == nil || len(rules) == 0 {
		return nil
	}

	violations := make([]Violation, 0, 16)
	for _, rule := range rules {
		switch rule.Type {
		case "max_lines":
			for _, file := range idx.Files {
				for _, symbol := range file.Symbols {
					span := symbolSpan(symbol)
					if rule.Kind != "*" && symbol.Kind != rule.Kind {
						continue
					}
					if span <= rule.MaxLines {
						continue
					}

					violations = append(violations, Violation{
						RuleID:    rule.ID,
						File:      symbol.File,
						Kind:      symbol.Kind,
						Name:      symbol.Name,
						StartLine: symbol.StartLine,
						EndLine:   symbol.EndLine,
						Span:      span,
						Message:   fmt.Sprintf("%s %q spans %d lines (max %d)", rule.KindLabel, symbol.Name, span, rule.MaxLines),
					})
				}
			}
		case "no_import":
			for _, file := range idx.Files {
				for _, imp := range file.Imports {
					if strings.TrimSpace(imp) != rule.ImportPath {
						continue
					}
					violations = append(violations, Violation{
						RuleID:  rule.ID,
						File:    file.Path,
						Kind:    "import",
						Name:    imp,
						Message: fmt.Sprintf("import %q is forbidden by rule", imp),
					})
				}
			}
		}
	}

	sortViolations(violations)

	return violations
}

func EvaluatePatterns(idx *model.Index, patterns []QueryPattern) ([]Violation, error) {
	if idx == nil || len(patterns) == 0 {
		return nil, nil
	}

	entriesByLanguage := map[string]grammars.LangEntry{}
	for _, entry := range grammars.AllLanguages() {
		if strings.TrimSpace(entry.Name) == "" || entry.Language == nil {
			continue
		}
		entriesByLanguage[entry.Name] = entry
	}

	langByName := map[string]*gotreesitter.Language{}
	parserByLanguage := map[string]*gotreesitter.Parser{}
	queryByPatternLanguage := map[string]*gotreesitter.Query{}
	queryCompileErr := map[string]bool{}

	violations := make([]Violation, 0, 32)
	for _, file := range idx.Files {
		entry, ok := entriesByLanguage[file.Language]
		if !ok {
			continue
		}

		lang, ok := langByName[file.Language]
		if !ok {
			lang = entry.Language()
			if lang == nil {
				continue
			}
			langByName[file.Language] = lang
		}

		sourcePath := filepath.Join(idx.Root, filepath.FromSlash(file.Path))
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, err
		}

		parser, ok := parserByLanguage[file.Language]
		if !ok {
			parser = gotreesitter.NewParser(lang)
			parserByLanguage[file.Language] = parser
		}

		var tree *gotreesitter.Tree
		var parseErr error
		if entry.TokenSourceFactory != nil {
			tokenSource := entry.TokenSourceFactory(source, lang)
			if tokenSource != nil {
				tree, parseErr = parser.ParseWithTokenSource(source, tokenSource)
			}
		}
		if tree == nil && parseErr == nil {
			tree, parseErr = parser.Parse(source)
		}
		if parseErr != nil {
			continue
		}
		if tree == nil || tree.RootNode() == nil {
			continue
		}

		for _, pattern := range patterns {
			key := pattern.ID + "\x00" + file.Language
			if queryCompileErr[key] {
				continue
			}

			compiled := queryByPatternLanguage[key]
			if compiled == nil {
				query, err := gotreesitter.NewQuery(pattern.Query, lang)
				if err != nil {
					queryCompileErr[key] = true
					continue
				}
				queryByPatternLanguage[key] = query
				compiled = query
			}

			matches := compiled.Execute(tree)
			for _, match := range matches {
				captureName, node := pickViolationCapture(match.Captures)
				if node == nil {
					continue
				}

				startLine := int(node.StartPoint().Row) + 1
				endLine := int(node.EndPoint().Row) + 1
				if endLine < startLine {
					endLine = startLine
				}
				span := endLine - startLine + 1
				if span < 1 {
					span = 1
				}

				message := pattern.Message
				if strings.TrimSpace(message) == "" {
					message = fmt.Sprintf("query pattern %q matched", pattern.Path)
				}
				if captureName != "" {
					message = message + " (@" + captureName + ")"
				}

				violations = append(violations, Violation{
					RuleID:    pattern.ID,
					File:      file.Path,
					Kind:      "query_pattern",
					Name:      compactPatternText(node.Text(source)),
					StartLine: startLine,
					EndLine:   endLine,
					Span:      span,
					Message:   message,
				})
			}
		}

		tree.Release()
	}

	sortViolations(violations)
	return violations, nil
}

func symbolSpan(symbol model.Symbol) int {
	if symbol.StartLine <= 0 || symbol.EndLine < symbol.StartLine {
		return 0
	}
	return symbol.EndLine - symbol.StartLine + 1
}

func pickViolationCapture(captures []gotreesitter.QueryCapture) (string, *gotreesitter.Node) {
	if len(captures) == 0 {
		return "", nil
	}
	for _, capture := range captures {
		if capture.Node == nil {
			continue
		}
		if capture.Name == "violation" {
			return capture.Name, capture.Node
		}
	}
	for _, capture := range captures {
		if capture.Node == nil {
			continue
		}
		return capture.Name, capture.Node
	}
	return "", nil
}

func compactPatternText(text string) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxLen = 120
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File == violations[j].File {
			if violations[i].StartLine == violations[j].StartLine {
				if violations[i].RuleID == violations[j].RuleID {
					return violations[i].Name < violations[j].Name
				}
				return violations[i].RuleID < violations[j].RuleID
			}
			return violations[i].StartLine < violations[j].StartLine
		}
		return violations[i].File < violations[j].File
	})
}
