package yara

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/odvcencio/gts-suite/pkg/model"
)

// Options controls YARA rule generation.
type Options struct {
	RuleName   string
	MinStrings int // default 3
	MaxStrings int // default 20
}

// GeneratedRule represents a generated YARA rule.
type GeneratedRule struct {
	Name      string   `json:"name"`
	Strings   []string `json:"strings"`
	APIs      []string `json:"apis"`
	Condition string   `json:"condition"`
	YARAText  string   `json:"yara_text"`
}

var stringLiteral = regexp.MustCompile(`"([^"\\]|\\.){2,}"`)

// Generate produces YARA rules from an index.
func Generate(idx *model.Index, root string, opts Options) ([]GeneratedRule, error) {
	if opts.MinStrings <= 0 {
		opts.MinStrings = 3
	}
	if opts.MaxStrings <= 0 {
		opts.MaxStrings = 20
	}
	if opts.RuleName == "" {
		opts.RuleName = "generated_rule"
	}

	// Collect all string literals and API calls across all files
	stringCounts := make(map[string]int) // string -> number of functions containing it
	apiSet := make(map[string]bool)
	totalFunctions := 0

	for _, f := range idx.Files {
		for _, sym := range f.Symbols {
			if sym.Kind != "function_definition" && sym.Kind != "method_definition" {
				continue
			}
			totalFunctions++

			body, err := readFunctionBody(root, f.Path, sym.StartLine, sym.EndLine)
			if err != nil {
				continue
			}

			// Extract string literals
			matches := stringLiteral.FindAllString(body, -1)
			seen := make(map[string]bool)
			for _, m := range matches {
				// Strip quotes
				inner := m[1 : len(m)-1]
				if !seen[inner] {
					seen[inner] = true
					stringCounts[inner]++
				}
			}
		}

		// Collect API references
		for _, ref := range f.References {
			if ref.Kind == "reference.call" {
				apiSet[ref.Name] = true
			}
		}
	}

	if totalFunctions == 0 {
		return nil, nil
	}

	// Score strings by uniqueness (fewer functions = more unique = higher score)
	type scoredString struct {
		Value string
		Score float64
	}
	var scored []scoredString
	for s, count := range stringCounts {
		if len(s) < 4 { // skip very short strings
			continue
		}
		score := 1.0 - float64(count)/float64(totalFunctions)
		if score < 0 {
			score = 0
		}
		scored = append(scored, scoredString{Value: s, Score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Take top strings up to MaxStrings
	var selectedStrings []string
	for i, s := range scored {
		if i >= opts.MaxStrings {
			break
		}
		selectedStrings = append(selectedStrings, s.Value)
	}

	var apis []string
	for api := range apiSet {
		apis = append(apis, api)
	}
	sort.Strings(apis)

	if len(selectedStrings) < opts.MinStrings && len(apis) == 0 {
		return nil, nil
	}

	// Generate YARA text
	rule := buildYARARule(opts.RuleName, selectedStrings, apis, opts.MinStrings)
	return []GeneratedRule{rule}, nil
}

func buildYARARule(name string, strs []string, apis []string, minStrings int) GeneratedRule {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("rule %s {\n", name))
	sb.WriteString("  strings:\n")

	allStrings := make([]string, 0, len(strs)+len(apis))

	for i, s := range strs {
		varName := fmt.Sprintf("$s%d", i)
		sb.WriteString(fmt.Sprintf("    %s = \"%s\"\n", varName, escapeYARA(s)))
		allStrings = append(allStrings, s)
	}

	for i, api := range apis {
		varName := fmt.Sprintf("$api%d", i)
		sb.WriteString(fmt.Sprintf("    %s = \"%s\"\n", varName, api))
		allStrings = append(allStrings, api)
	}

	totalVars := len(strs) + len(apis)
	condCount := minStrings
	if condCount > totalVars {
		condCount = totalVars
	}

	condition := fmt.Sprintf("%d of them", condCount)
	sb.WriteString("  condition:\n")
	sb.WriteString(fmt.Sprintf("    %s\n", condition))
	sb.WriteString("}\n")

	return GeneratedRule{
		Name:      name,
		Strings:   allStrings,
		APIs:      apis,
		Condition: condition,
		YARAText:  sb.String(),
	}
}

func escapeYARA(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func readFunctionBody(root, file string, startLine, endLine int) (string, error) {
	path := file
	if root != "" {
		path = root + "/" + file
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.Join(lines[startLine-1:endLine], "\n"), nil
}
