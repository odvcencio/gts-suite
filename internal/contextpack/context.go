package contextpack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gts-suite/internal/model"
)

type Options struct {
	FilePath    string
	Line        int
	TokenBudget int
}

type Report struct {
	File            string         `json:"file"`
	Line            int            `json:"line"`
	TokenBudget     int            `json:"token_budget"`
	EstimatedTokens int            `json:"estimated_tokens"`
	Focus           *model.Symbol  `json:"focus,omitempty"`
	Imports         []string       `json:"imports,omitempty"`
	SnippetStart    int            `json:"snippet_start"`
	SnippetEnd      int            `json:"snippet_end"`
	Snippet         string         `json:"snippet"`
	Related         []model.Symbol `json:"related,omitempty"`
	Truncated       bool           `json:"truncated"`
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
	if opts.TokenBudget <= 0 {
		opts.TokenBudget = 800
	}

	relPath, absPath, err := resolvePaths(idx.Root, opts.FilePath)
	if err != nil {
		return Report{}, err
	}

	fileSummary, err := findFileSummary(idx, relPath)
	if err != nil {
		return Report{}, err
	}

	source, err := os.ReadFile(absPath)
	if err != nil {
		return Report{}, err
	}
	lines := splitLines(string(source))
	if len(lines) == 0 {
		lines = []string{""}
	}

	report := Report{
		File:        fileSummary.Path,
		Line:        opts.Line,
		TokenBudget: opts.TokenBudget,
		Imports:     append([]string(nil), fileSummary.Imports...),
	}

	focus := findFocusSymbol(fileSummary.Symbols, opts.Line)
	if focus != nil {
		focusCopy := *focus
		report.Focus = &focusCopy
	}

	start, end := initialSnippetBounds(report.Focus, opts.Line, len(lines))
	snippet := renderSnippet(lines, start, end)

	baseTokens := estimateTokens(renderMetadata(report))
	snippetTokens := estimateTokens(snippet)
	for start < end && baseTokens+snippetTokens > opts.TokenBudget {
		start, end = shrinkWindow(start, end, opts.Line)
		snippet = renderSnippet(lines, start, end)
		snippetTokens = estimateTokens(snippet)
		report.Truncated = true
	}

	report.SnippetStart = start
	report.SnippetEnd = end
	report.Snippet = snippet

	remaining := opts.TokenBudget - (baseTokens + snippetTokens)
	report.Related = pickRelatedSymbols(fileSummary.Symbols, report.Focus, remaining)

	report.EstimatedTokens = estimateTokens(renderMetadata(report) + snippet + renderRelated(report.Related))
	if report.EstimatedTokens > opts.TokenBudget {
		report.Truncated = true
	}

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

func splitLines(src string) []string {
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
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

func initialSnippetBounds(focus *model.Symbol, line, totalLines int) (int, int) {
	if totalLines <= 0 {
		return 1, 1
	}

	if focus != nil && focus.StartLine > 0 && focus.EndLine >= focus.StartLine {
		return clampLine(focus.StartLine, totalLines), clampLine(focus.EndLine, totalLines)
	}

	return clampLine(line-8, totalLines), clampLine(line+8, totalLines)
}

func clampLine(line, totalLines int) int {
	if line < 1 {
		return 1
	}
	if line > totalLines {
		return totalLines
	}
	return line
}

func renderSnippet(lines []string, start, end int) string {
	if len(lines) == 0 {
		return ""
	}
	start = clampLine(start, len(lines))
	end = clampLine(end, len(lines))
	if end < start {
		end = start
	}

	width := len(fmt.Sprintf("%d", end))
	var builder strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&builder, "%*d | %s\n", width, i, lines[i-1])
	}
	return builder.String()
}

func shrinkWindow(start, end, focusLine int) (int, int) {
	if start >= end {
		return start, end
	}

	if focusLine <= start {
		return start, end - 1
	}
	if focusLine >= end {
		return start + 1, end
	}

	if (focusLine - start) >= (end - focusLine) {
		return start + 1, end
	}
	return start, end - 1
}

func pickRelatedSymbols(symbols []model.Symbol, focus *model.Symbol, budget int) []model.Symbol {
	if budget <= 0 {
		return nil
	}

	related := make([]model.Symbol, 0, 8)
	for _, symbol := range symbols {
		if focus != nil && symbol.Name == focus.Name && symbol.Kind == focus.Kind && symbol.StartLine == focus.StartLine {
			continue
		}
		if symbol.Kind != "type_definition" {
			continue
		}
		related = append(related, symbol)
	}

	sort.Slice(related, func(i, j int) bool {
		if related[i].StartLine == related[j].StartLine {
			return related[i].Name < related[j].Name
		}
		return related[i].StartLine < related[j].StartLine
	})

	trimmed := make([]model.Symbol, 0, len(related))
	used := 0
	for _, symbol := range related {
		cost := estimateTokens(symbol.Signature) + estimateTokens(symbol.Name) + 4
		if used+cost > budget {
			break
		}
		trimmed = append(trimmed, symbol)
		used += cost
	}
	return trimmed
}

func estimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return (len(trimmed) + 3) / 4
}

func renderMetadata(report Report) string {
	var builder strings.Builder
	builder.WriteString(report.File)
	builder.WriteString("\n")
	builder.WriteString(strings.Join(report.Imports, ","))
	builder.WriteString("\n")
	if report.Focus != nil {
		builder.WriteString(report.Focus.Signature)
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderRelated(symbols []model.Symbol) string {
	if len(symbols) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, symbol := range symbols {
		builder.WriteString(symbol.Signature)
		builder.WriteByte('\n')
	}
	return builder.String()
}
