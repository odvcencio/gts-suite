// Package generated detects machine-generated source files using filename
// patterns and header markers from a built-in registry of code generators.
package generated

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/odvcencio/gts-suite/pkg/model"
)

const maxScanLines = 20

// ConfigEntry represents a user-defined generated file pattern from .gtsgenerated.
type ConfigEntry struct {
	Generator string
	Pattern   string
}

// Detector identifies generated files using config overrides, filename patterns,
// and header marker regexps.
type Detector struct {
	configs         []ConfigEntry
	compiledMarkers []compiledSignature
}

type compiledSignature struct {
	generator string
	patterns  []string
	markers   []*regexp.Regexp
}

// NewDetector creates a Detector with the given user config entries (may be nil)
// and the built-in registry.
func NewDetector(configs []ConfigEntry) *Detector {
	d := &Detector{configs: configs}
	for _, sig := range builtinRegistry {
		cs := compiledSignature{
			generator: sig.Generator,
			patterns:  sig.Patterns,
		}
		for _, m := range sig.Markers {
			re, err := regexp.Compile(m)
			if err != nil {
				continue
			}
			cs.markers = append(cs.markers, re)
		}
		d.compiledMarkers = append(d.compiledMarkers, cs)
	}
	return d
}

// Detect checks whether the file at relPath (slash-separated, relative to repo root)
// is generated. Source bytes are needed for header marker scanning; pass nil to
// skip marker detection (filename patterns still work).
// Returns nil if the file is not generated.
func (d *Detector) Detect(relPath string, source []byte) *model.GeneratedInfo {
	if d == nil {
		return nil
	}

	// Phase 1: user config patterns (highest priority).
	for _, cfg := range d.configs {
		if matchGlob(cfg.Pattern, relPath) {
			return &model.GeneratedInfo{
				Generator: cfg.Generator,
				Reason:    "config",
			}
		}
	}

	// Phase 2: built-in filename patterns.
	for _, cs := range d.compiledMarkers {
		for _, pat := range cs.patterns {
			base := filepath.Base(relPath)
			if matched, _ := filepath.Match(pat, base); matched {
				return &model.GeneratedInfo{
					Generator: cs.generator,
					Reason:    "filename",
				}
			}
			// Also try full path match for patterns with slashes.
			if strings.Contains(pat, "/") {
				if matched, _ := filepath.Match(pat, relPath); matched {
					return &model.GeneratedInfo{
						Generator: cs.generator,
						Reason:    "filename",
					}
				}
			}
		}
	}

	// Phase 3: header markers (scan first N lines of source).
	if len(source) == 0 {
		return nil
	}

	header := extractHeader(source, maxScanLines)
	for _, cs := range d.compiledMarkers {
		for _, re := range cs.markers {
			if loc := re.Find(header); loc != nil {
				return &model.GeneratedInfo{
					Generator: cs.generator,
					Reason:    "marker",
					Marker:    strings.TrimSpace(string(loc)),
				}
			}
		}
	}

	return nil
}

func extractHeader(source []byte, maxLines int) []byte {
	end := 0
	lines := 0
	for i, b := range source {
		if b == '\n' {
			lines++
			if lines >= maxLines {
				end = i
				break
			}
		}
	}
	if end == 0 {
		return source
	}
	return source[:end]
}

// matchGlob matches a pattern against a slash-separated path, supporting **
// for recursive directory matching. Falls back to filepath.Match for patterns
// without **.
func matchGlob(pattern, path string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}
	parts := strings.Split(pattern, "**")
	if len(parts) == 2 {
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")
		if prefix != "" && !strings.HasPrefix(path, prefix+"/") && path != prefix {
			return false
		}
		if suffix == "" {
			return true
		}
		remaining := path
		if prefix != "" {
			remaining = strings.TrimPrefix(path, prefix+"/")
		}
		segments := strings.Split(remaining, "/")
		for i := range segments {
			tail := strings.Join(segments[i:], "/")
			if matched, _ := filepath.Match(suffix, tail); matched {
				return true
			}
		}
	}
	return false
}
