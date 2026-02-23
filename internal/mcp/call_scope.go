package mcp

import (
	gtsscope "gts-suite/internal/scope"
)

func (s *Service) callScope(args map[string]any) (any, error) {
	filePath, err := requiredStringArg(args, "file")
	if err != nil {
		return nil, err
	}

	rootPath := s.stringArgOrDefault(args, "root", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)
	line := intArg(args, "line", 1)

	idx, err := s.loadOrBuild(cachePath, rootPath)
	if err != nil {
		return nil, err
	}

	report, err := gtsscope.Build(idx, gtsscope.Options{
		FilePath: filePath,
		Line:     line,
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}
