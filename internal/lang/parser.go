// Package lang defines the Parser interface for language-specific source file parsing.
package lang

import "gts-suite/internal/model"

type Parser interface {
	Language() string
	Parse(path string, src []byte) (model.FileSummary, error)
}
