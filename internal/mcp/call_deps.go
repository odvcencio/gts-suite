package mcp

import (
	"github.com/odvcencio/gts-suite/internal/deps"
)

func (s *Service) callDeps(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	report, err := deps.Build(idx, deps.Options{
		Mode:         s.stringArgOrDefault(args, "by", "package"),
		Top:          intArg(args, "top", 10),
		Focus:        stringArg(args, "focus"),
		Depth:        intArg(args, "depth", 1),
		Reverse:      boolArg(args, "reverse", false),
		IncludeEdges: boolArg(args, "edges", false),
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}
