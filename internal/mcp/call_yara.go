package mcp

import (
	"github.com/odvcencio/gts-suite/pkg/yara"
)

func (s *Service) callYara(args map[string]any) (any, error) {
	target := s.stringArgOrDefault(args, "path", s.defaultRoot)
	cachePath := s.stringArgOrDefault(args, "cache", s.defaultCache)

	idx, err := s.loadOrBuild(cachePath, target)
	if err != nil {
		return nil, err
	}

	opts := yara.Options{
		RuleName:   s.stringArgOrDefault(args, "rule_name", "generated_rule"),
		MinStrings: intArg(args, "min_strings", 3),
		MaxStrings: intArg(args, "max_strings", 20),
	}

	rules, err := yara.Generate(idx, target, opts)
	if err != nil {
		return nil, err
	}
	if rules == nil {
		rules = []yara.GeneratedRule{}
	}

	return map[string]any{
		"count": len(rules),
		"rules": rules,
	}, nil
}
