package mcp

import (
	"fmt"

	"github.com/odvcencio/gts-suite/pkg/structdiff"
)

func (s *Service) callDiff(args map[string]any) (any, error) {
	beforePath := stringArg(args, "before_path")
	beforeCache := stringArg(args, "before_cache")
	afterPath := stringArg(args, "after_path")
	afterCache := stringArg(args, "after_cache")

	if beforePath == "" && beforeCache == "" {
		return nil, fmt.Errorf("missing before source: set before_path or before_cache")
	}
	if afterPath == "" && afterCache == "" {
		return nil, fmt.Errorf("missing after source: set after_path or after_cache")
	}

	beforeIndex, err := s.loadIndexFromSource(beforePath, beforeCache)
	if err != nil {
		return nil, fmt.Errorf("load before source: %w", err)
	}
	afterIndex, err := s.loadIndexFromSource(afterPath, afterCache)
	if err != nil {
		return nil, fmt.Errorf("load after source: %w", err)
	}

	report := structdiff.Compare(beforeIndex, afterIndex)
	return report, nil
}
