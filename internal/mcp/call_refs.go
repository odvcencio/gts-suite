package mcp

import (
	"fmt"
	"regexp"
	"sort"
)

func (s *Service) callRefs(args map[string]any) (any, error) {
	name, err := requiredStringArg(args, "name")
	if err != nil {
		return nil, err
	}
	regexMode := boolArg(args, "regex", false)
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	matchReference := func(candidate string) bool { return candidate == name }
	if regexMode {
		compiled, compileErr := regexp.Compile(name)
		if compileErr != nil {
			return nil, fmt.Errorf("compile regex: %w", compileErr)
		}
		matchReference = compiled.MatchString
	}

	type referenceMatch struct {
		File        string `json:"file"`
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		StartColumn int    `json:"start_column"`
		EndColumn   int    `json:"end_column"`
	}
	matches := make([]referenceMatch, 0, idx.ReferenceCount())
	for _, file := range idx.Files {
		for _, reference := range file.References {
			if !matchReference(reference.Name) {
				continue
			}
			matches = append(matches, referenceMatch{
				File:        file.Path,
				Kind:        reference.Kind,
				Name:        reference.Name,
				StartLine:   reference.StartLine,
				EndLine:     reference.EndLine,
				StartColumn: reference.StartColumn,
				EndColumn:   reference.EndColumn,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].File == matches[j].File {
			if matches[i].StartLine == matches[j].StartLine {
				if matches[i].StartColumn == matches[j].StartColumn {
					return matches[i].Name < matches[j].Name
				}
				return matches[i].StartColumn < matches[j].StartColumn
			}
			return matches[i].StartLine < matches[j].StartLine
		}
		return matches[i].File < matches[j].File
	})

	return map[string]any{
		"matches": matches,
		"count":   len(matches),
	}, nil
}
