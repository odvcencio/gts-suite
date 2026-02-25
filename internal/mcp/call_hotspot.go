package mcp

import (
	"github.com/odvcencio/gts-suite/pkg/hotspot"
)

func (s *Service) callHotspot(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	since := s.stringArgOrDefault(args, "since", "90d")
	top := intArg(args, "top", 20)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	opts := hotspot.Options{
		Root:  target,
		Since: since,
		Top:   top,
	}

	report, err := hotspot.Analyze(idx, opts)
	if err != nil {
		return nil, err
	}

	return report, nil
}
