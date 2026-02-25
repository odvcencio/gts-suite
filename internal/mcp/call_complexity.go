package mcp

import (
	"github.com/odvcencio/gts-suite/pkg/complexity"
	"github.com/odvcencio/gts-suite/pkg/xref"
)

func (s *Service) callComplexity(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	minCyclomatic := intArg(args, "min_cyclomatic", 0)
	sortField := s.stringArgOrDefault(args, "sort", "cyclomatic")
	top := intArg(args, "top", 0)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	opts := complexity.Options{
		MinCyclomatic: minCyclomatic,
		Sort:          sortField,
		Top:           top,
	}

	report, err := complexity.Analyze(idx, idx.Root, opts)
	if err != nil {
		return nil, err
	}

	graph, err := xref.Build(idx)
	if err == nil {
		complexity.EnrichWithXref(report, graph)
	}

	return report, nil
}
