// Package index builds and caches structural indexes by walking source trees and parsing files with registered language parsers.
package index

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/odvcencio/gts-suite/pkg/ignore"
	"github.com/odvcencio/gts-suite/pkg/lang"
	"github.com/odvcencio/gts-suite/pkg/lang/treesitter"
	"github.com/odvcencio/gts-suite/pkg/model"
)

const schemaVersion = "0.1.0"

type Builder struct {
	parsers map[string]lang.Parser
	ignore  *ignore.Matcher
}

type BuildStats struct {
	CandidateFiles int `json:"candidate_files"`
	ParsedFiles    int `json:"parsed_files"`
	ReusedFiles    int `json:"reused_files"`
}

func NewBuilder() *Builder {
	builder := &Builder{
		parsers: make(map[string]lang.Parser),
	}
	builder.registerTreesitterParsers()
	return builder
}

func (b *Builder) registerTreesitterParsers() {
	entries := append([]grammars.LangEntry(nil), grammars.AllLanguages()...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	for _, entry := range entries {
		if strings.TrimSpace(entry.TagsQuery) == "" {
			continue
		}

		parser, err := treesitter.NewParser(entry)
		if err != nil {
			continue
		}

		for _, ext := range entry.Extensions {
			normalized := normalizeExtension(ext)
			if normalized == "" {
				continue
			}
			if _, exists := b.parsers[normalized]; exists {
				continue
			}
			b.parsers[normalized] = parser
		}
	}
}

// SetIgnore configures a .gtsignore-style matcher to skip paths during indexing.
func (b *Builder) SetIgnore(m *ignore.Matcher) {
	b.ignore = m
}

// Ignore returns the current ignore matcher, or nil if none is set.
func (b *Builder) Ignore() *ignore.Matcher {
	return b.ignore
}

func (b *Builder) Register(extension string, parser lang.Parser) {
	if parser == nil {
		return
	}
	normalized := normalizeExtension(extension)
	if normalized == "" {
		return
	}
	b.parsers[normalized] = parser
}

func normalizeExtension(extension string) string {
	normalized := strings.ToLower(strings.TrimSpace(extension))
	if normalized == "" {
		return ""
	}
	if normalized[0] != '.' {
		normalized = "." + normalized
	}
	return normalized
}

func (b *Builder) BuildPath(path string) (*model.Index, error) {
	idx, _, err := b.BuildPathIncremental(path, nil)
	return idx, err
}

func (b *Builder) BuildPathIncremental(path string, previous *model.Index) (*model.Index, BuildStats, error) {
	stats := BuildStats{}

	if strings.TrimSpace(path) == "" {
		path = "."
	}

	target, err := filepath.Abs(path)
	if err != nil {
		return nil, stats, err
	}
	target = filepath.Clean(target)

	info, err := os.Stat(target)
	if err != nil {
		return nil, stats, err
	}

	root := target
	candidates := make([]sourceCandidate, 0, 128)
	if info.IsDir() {
		candidates, err = b.collectCandidates(target)
		if err != nil {
			return nil, stats, err
		}
	} else {
		root = filepath.Dir(target)
		if parser, ok := b.parserForPath(target); ok {
			candidates = append(candidates, sourceCandidate{
				Path:            target,
				Parser:          parser,
				SizeBytes:       info.Size(),
				ModTimeUnixNano: info.ModTime().UnixNano(),
			})
		}
	}
	root = filepath.Clean(root)

	index := &model.Index{
		Version:     schemaVersion,
		Root:        root,
		GeneratedAt: time.Now().UTC(),
	}

	previousByPath := previousFilesByPath(previous, root)

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Path < candidates[j].Path
	})
	tasks := make([]parseTask, 0, len(candidates))
	orderedFiles := make([]model.FileSummary, 0, len(candidates))
	orderedValid := make([]bool, 0, len(candidates))
	for _, candidate := range candidates {
		stats.CandidateFiles++

		relPath, relErr := filepath.Rel(root, candidate.Path)
		if relErr != nil {
			relPath = candidate.Path
		}
		relPath = filepath.ToSlash(relPath)

		if previousFile, ok := previousByPath[relPath]; ok && canReuseSummary(previousFile, candidate, candidate.Parser.Language()) {
			reused := previousFile
			reused.Path = relPath
			reused.Language = candidate.Parser.Language()
			reused.SizeBytes = candidate.SizeBytes
			reused.ModTimeUnixNano = candidate.ModTimeUnixNano
			for i := range reused.Symbols {
				reused.Symbols[i].File = relPath
			}
			for i := range reused.References {
				reused.References[i].File = relPath
			}
			orderedFiles = append(orderedFiles, reused)
			orderedValid = append(orderedValid, true)
			stats.ReusedFiles++
			continue
		}

		position := len(orderedFiles)
		orderedFiles = append(orderedFiles, model.FileSummary{})
		orderedValid = append(orderedValid, false)
		tasks = append(tasks, parseTask{
			Position:        position,
			FilePath:        candidate.Path,
			RelPath:         relPath,
			Parser:          candidate.Parser,
			SizeBytes:       candidate.SizeBytes,
			ModTimeUnixNano: candidate.ModTimeUnixNano,
		})
	}

	results := b.parseFiles(tasks)
	index.Files = make([]model.FileSummary, 0, len(orderedFiles))
	for _, result := range results {
		if result.Err != nil {
			index.Errors = append(index.Errors, model.ParseError{
				Path:  result.RelPath,
				Error: result.Err.Error(),
			})
			continue
		}
		orderedFiles[result.Position] = result.Summary
		orderedValid[result.Position] = true
		stats.ParsedFiles++
	}

	for i, valid := range orderedValid {
		if !valid {
			continue
		}
		index.Files = append(index.Files, orderedFiles[i])
	}

	return index, stats, nil
}

type parseTask struct {
	Position        int
	FilePath        string
	RelPath         string
	Parser          lang.Parser
	SizeBytes       int64
	ModTimeUnixNano int64
}

type parseResult struct {
	Position int
	RelPath  string
	Summary  model.FileSummary
	Err      error
}

type sourceCandidate struct {
	Path            string
	Parser          lang.Parser
	SizeBytes       int64
	ModTimeUnixNano int64
}

func (b *Builder) parseFiles(tasks []parseTask) []parseResult {
	if len(tasks) == 0 {
		return nil
	}

	results := make([]parseResult, len(tasks))
	workers := indexWorkerCount(len(tasks))

	taskCh := make(chan int, len(tasks))
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for idx := range taskCh {
				task := tasks[idx]
				result := parseResult{
					Position: task.Position,
					RelPath:  task.RelPath,
				}

				source, readErr := os.ReadFile(task.FilePath)
				if readErr != nil {
					result.Err = readErr
					results[idx] = result
					continue
				}

				summary, parseErr := task.Parser.Parse(task.FilePath, source)
				if parseErr != nil {
					result.Err = parseErr
					results[idx] = result
					continue
				}

				summary.Path = task.RelPath
				summary.SizeBytes = task.SizeBytes
				summary.ModTimeUnixNano = task.ModTimeUnixNano
				for i := range summary.Symbols {
					summary.Symbols[i].File = task.RelPath
				}
				for i := range summary.References {
					summary.References[i].File = task.RelPath
				}
				result.Summary = summary
				results[idx] = result
			}
		}()
	}

	for i := range tasks {
		taskCh <- i
	}
	close(taskCh)
	wg.Wait()
	return results
}

func indexWorkerCount(taskCount int) int {
	if taskCount <= 0 {
		return 0
	}

	if raw := strings.TrimSpace(os.Getenv("GTS_INDEX_WORKERS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > taskCount {
				return taskCount
			}
			return parsed
		}
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > taskCount {
		workers = taskCount
	}
	return workers
}

func previousFilesByPath(previous *model.Index, root string) map[string]model.FileSummary {
	reused := map[string]model.FileSummary{}
	if previous == nil {
		return reused
	}

	previousRoot := filepath.Clean(previous.Root)
	if previousRoot != root {
		return reused
	}

	for _, file := range previous.Files {
		reused[file.Path] = file
	}
	return reused
}

func canReuseSummary(summary model.FileSummary, candidate sourceCandidate, language string) bool {
	if summary.Language != language {
		return false
	}
	if summary.SizeBytes != candidate.SizeBytes {
		return false
	}
	if summary.ModTimeUnixNano != candidate.ModTimeUnixNano {
		return false
	}
	return true
}

func (b *Builder) parserForPath(path string) (lang.Parser, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	parser, ok := b.parsers[ext]
	return parser, ok
}

func (b *Builder) ParserForPath(path string) (lang.Parser, bool) {
	return b.parserForPath(path)
}

func (b *Builder) collectCandidates(root string) ([]sourceCandidate, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("root path is empty")
	}

	files := make([]sourceCandidate, 0, 128)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == ".hg" || name == ".svn" || name == "node_modules" || name == "vendor" {
				if path != root {
					return filepath.SkipDir
				}
			}
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			if path != root && b.ignore != nil {
				relPath, relErr := filepath.Rel(root, path)
				if relErr == nil && b.ignore.Match(filepath.ToSlash(relPath), true) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if b.ignore != nil {
			relPath, relErr := filepath.Rel(root, path)
			if relErr == nil && b.ignore.Match(filepath.ToSlash(relPath), false) {
				return nil
			}
		}

		parser, ok := b.parserForPath(path)
		if !ok {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, sourceCandidate{
			Path:            path,
			Parser:          parser,
			SizeBytes:       info.Size(),
			ModTimeUnixNano: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
