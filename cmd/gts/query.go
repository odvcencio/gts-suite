package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/spf13/cobra"
)

func newQueryCmd() *cobra.Command {
	var cachePath string
	var noCache bool
	var jsonOutput bool
	var countOnly bool
	var limit int
	var captures []string

	cmd := &cobra.Command{
		Use:     "query <pattern> [path]",
		Aliases: []string{"gtsquery"},
		Short:   "Run raw tree-sitter S-expression queries across files",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			queryText := strings.TrimSpace(args[0])
			if queryText == "" {
				return errors.New("query pattern cannot be empty")
			}

			target := "."
			if len(args) == 2 {
				target = args[1]
			}
			idx, err := loadOrBuild(cachePath, target, noCache)
			if err != nil {
				return err
			}

			captureFilter := map[string]bool{}
			for _, name := range captures {
				captureFilter[strings.TrimSpace(name)] = true
			}

			entriesByLanguage := map[string]grammars.LangEntry{}
			for _, entry := range grammars.AllLanguages() {
				if strings.TrimSpace(entry.Name) == "" || entry.Language == nil {
					continue
				}
				entriesByLanguage[entry.Name] = entry
			}

			queryByLanguage := map[string]*gotreesitter.Query{}
			queryErrorByLanguage := map[string]string{}
			langByName := map[string]*gotreesitter.Language{}
			parserByLanguage := map[string]*gotreesitter.Parser{}

			truncated := false
			results := make([]queryCaptureMatch, 0, idx.SymbolCount())
		fileLoop:
			for _, file := range idx.Files {
				entry, ok := entriesByLanguage[file.Language]
				if !ok {
					continue
				}
				if _, failed := queryErrorByLanguage[file.Language]; failed {
					continue
				}

				lang, ok := langByName[file.Language]
				if !ok {
					lang = entry.Language()
					if lang == nil {
						queryErrorByLanguage[file.Language] = "language loader returned nil"
						continue
					}
					langByName[file.Language] = lang
				}

				queryForLanguage, ok := queryByLanguage[file.Language]
				if !ok {
					compiled, compileErr := gotreesitter.NewQuery(queryText, lang)
					if compileErr != nil {
						queryErrorByLanguage[file.Language] = compileErr.Error()
						continue
					}
					queryByLanguage[file.Language] = compiled
					queryForLanguage = compiled
				}

				sourcePath := filepath.Join(idx.Root, filepath.FromSlash(file.Path))
				source, readErr := os.ReadFile(sourcePath)
				if readErr != nil {
					return readErr
				}

				parser, ok := parserByLanguage[file.Language]
				if !ok {
					parser = gotreesitter.NewParser(lang)
					parserByLanguage[file.Language] = parser
				}

				var tree *gotreesitter.Tree
				var parseErr error
				if entry.TokenSourceFactory != nil {
					tokenSource := entry.TokenSourceFactory(source, lang)
					if tokenSource != nil {
						tree, parseErr = parser.ParseWithTokenSource(source, tokenSource)
					}
				}
				if tree == nil && parseErr == nil {
					tree, parseErr = parser.Parse(source)
				}
				if parseErr != nil {
					continue
				}
				if tree == nil || tree.RootNode() == nil {
					continue
				}

				matches := queryForLanguage.Execute(tree)
				for _, match := range matches {
					for _, capture := range match.Captures {
						if len(captureFilter) > 0 && !captureFilter[capture.Name] {
							continue
						}
						node := capture.Node
						if node == nil {
							continue
						}
						startLine := int(node.StartPoint().Row) + 1
						endLine := int(node.EndPoint().Row) + 1
						if endLine < startLine {
							endLine = startLine
						}
						startColumn := int(node.StartPoint().Column) + 1
						endColumn := int(node.EndPoint().Column) + 1
						if endColumn < startColumn {
							endColumn = startColumn
						}
						results = append(results, queryCaptureMatch{
							File:        file.Path,
							Language:    file.Language,
							Pattern:     match.PatternIndex,
							Capture:     capture.Name,
							NodeType:    node.Type(lang),
							Text:        compactNodeText(node.Text(source)),
							StartLine:   startLine,
							EndLine:     endLine,
							StartColumn: startColumn,
							EndColumn:   endColumn,
						})
						if limit > 0 && len(results) >= limit {
							truncated = true
							tree.Release()
							break fileLoop
						}
					}
				}
				tree.Release()
			}

			sort.Slice(results, func(i, j int) bool {
				if results[i].File == results[j].File {
					if results[i].StartLine == results[j].StartLine {
						if results[i].StartColumn == results[j].StartColumn {
							return results[i].Capture < results[j].Capture
						}
						return results[i].StartColumn < results[j].StartColumn
					}
					return results[i].StartLine < results[j].StartLine
				}
				return results[i].File < results[j].File
			})

			languageErrors := make([]queryLanguageError, 0, len(queryErrorByLanguage))
			for language, value := range queryErrorByLanguage {
				languageErrors = append(languageErrors, queryLanguageError{
					Language: language,
					Error:    value,
				})
			}
			sort.Slice(languageErrors, func(i, j int) bool {
				return languageErrors[i].Language < languageErrors[j].Language
			})

			if jsonOutput {
				if countOnly {
					return emitJSON(struct {
						Count          int                  `json:"count"`
						Truncated      bool                 `json:"truncated,omitempty"`
						LanguageErrors []queryLanguageError `json:"language_errors,omitempty"`
					}{
						Count:          len(results),
						Truncated:      truncated,
						LanguageErrors: languageErrors,
					})
				}
				if truncated {
					return emitJSON(struct {
						Matches        []queryCaptureMatch  `json:"matches,omitempty"`
						Count          int                  `json:"count"`
						Truncated      bool                 `json:"truncated"`
						LanguageErrors []queryLanguageError `json:"language_errors,omitempty"`
					}{
						Matches:        results,
						Count:          len(results),
						Truncated:      true,
						LanguageErrors: languageErrors,
					})
				}
				return emitJSON(struct {
					Matches        []queryCaptureMatch  `json:"matches,omitempty"`
					Count          int                  `json:"count"`
					LanguageErrors []queryLanguageError `json:"language_errors,omitempty"`
				}{
					Matches:        results,
					Count:          len(results),
					LanguageErrors: languageErrors,
				})
			}

			for _, item := range languageErrors {
				fmt.Fprintf(os.Stderr, "query: skip language=%s err=%s\n", item.Language, item.Error)
			}

			if countOnly {
				fmt.Println(len(results))
				if truncated {
					fmt.Printf("truncated: limit=%d\n", limit)
				}
				return nil
			}

			for _, match := range results {
				fmt.Printf(
					"%s:%d:%d capture=%s type=%s text=%q\n",
					match.File,
					match.StartLine,
					match.StartColumn,
					match.Capture,
					match.NodeType,
					match.Text,
				)
			}
			if truncated {
				fmt.Fprintf(os.Stderr, "warning: results truncated at limit=%d, use --limit 0 for all\n", limit)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cachePath, "cache", "", "load index from cache instead of parsing")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "skip auto-discovery of cached index")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&countOnly, "count", false, "print the number of captures")
	cmd.Flags().IntVar(&limit, "limit", 1000, "maximum number of results (0 for unlimited)")
	cmd.Flags().StringArrayVar(&captures, "capture", nil, "capture name filter (repeatable)")
	return cmd
}

func runQuery(args []string) error {
	cmd := newQueryCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}
