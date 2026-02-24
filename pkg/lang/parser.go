// Package lang defines the Parser interface for language-specific source file parsing.
package lang

import "github.com/odvcencio/gts-suite/pkg/model"

type Parser interface {
	Language() string
	Parse(path string, src []byte) (model.FileSummary, error)
}
