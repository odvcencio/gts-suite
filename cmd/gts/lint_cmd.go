package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/odvcencio/gts-suite/internal/lint"
)

func newLintCmd() *cobra.Command {
	var cachePath string
	var failOnViolations bool
	var jsonOutput bool
	var rawRules []string
	var rawPatterns []string

	cmd := &cobra.Command{
		Use:     "lint [path]",
		Aliases: []string{"gtslint"},
		Short:   "Run structural lint rules against indexed symbols",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(rawRules) == 0 && len(rawPatterns) == 0 {
				return errors.New("at least one --rule or --pattern is required")
			}

			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			rules := make([]lint.Rule, 0, len(rawRules))
			for _, rawRule := range rawRules {
				rule, err := lint.ParseRule(rawRule)
				if err != nil {
					return fmt.Errorf("parse rule %q: %w", rawRule, err)
				}
				rules = append(rules, rule)
			}
			patterns := make([]lint.QueryPattern, 0, len(rawPatterns))
			for _, rawPattern := range rawPatterns {
				pattern, err := lint.LoadQueryPattern(rawPattern)
				if err != nil {
					return fmt.Errorf("load pattern %q: %w", rawPattern, err)
				}
				patterns = append(patterns, pattern)
			}

			idx, err := loadOrBuild(cachePath, target)
			if err != nil {
				return err
			}

			violations := lint.Evaluate(idx, rules)
			patternViolations, err := lint.EvaluatePatterns(idx, patterns)
			if err != nil {
				return err
			}
			violations = append(violations, patternViolations...)
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

			if jsonOutput {
				return emitJSON(struct {
					Rules      []lint.Rule         `json:"rules,omitempty"`
					Patterns   []lint.QueryPattern `json:"patterns,omitempty"`
					Violations []lint.Violation    `json:"violations,omitempty"`
					Count      int                 `json:"count"`
				}{
					Rules:      rules,
					Patterns:   patterns,
					Violations: violations,
					Count:      len(violations),
				})
			}

			for _, violation := range violations {
				if violation.StartLine <= 0 {
					fmt.Printf(
						"%s %s %s rule=%s %s\n",
						violation.File,
						violation.Kind,
						violation.Name,
						violation.RuleID,
						violation.Message,
					)
					continue
				}
				fmt.Printf(
					"%s:%d:%d %s %s rule=%s %s\n",
					violation.File,
					violation.StartLine,
					violation.EndLine,
					violation.Kind,
					violation.Name,
					violation.RuleID,
					violation.Message,
				)
			}
			fmt.Printf("lint: rules=%d patterns=%d violations=%d\n", len(rules), len(patterns), len(violations))
			if len(idx.Errors) > 0 {
				fmt.Printf("lint: parse errors=%d (ignored)\n", len(idx.Errors))
			}

			if len(violations) > 0 && failOnViolations {
				return exitCodeError{
					code: 3,
					err:  fmt.Errorf("%d lint violations", len(violations)),
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().BoolVar(&failOnViolations, "fail-on-violations", true, "exit non-zero when violations are found")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	cmd.Flags().StringArrayVar(&rawRules, "rule", nil, "lint rule expression (repeatable)")
	cmd.Flags().StringArrayVar(&rawPatterns, "pattern", nil, "tree-sitter query pattern file (.scm) (repeatable)")
	return cmd
}

func runLint(args []string) error {
	cmd := newLintCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
