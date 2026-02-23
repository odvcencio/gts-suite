package model

import "time"

type Symbol struct {
	File      string `json:"file"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	Receiver  string `json:"receiver,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type FileSummary struct {
	Path            string   `json:"path"`
	Language        string   `json:"language"`
	SizeBytes       int64    `json:"size_bytes,omitempty"`
	ModTimeUnixNano int64    `json:"mod_time_unix_nano,omitempty"`
	Imports         []string `json:"imports,omitempty"`
	Symbols         []Symbol `json:"symbols,omitempty"`
}

type ParseError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type Index struct {
	Version     string        `json:"version"`
	Root        string        `json:"root"`
	GeneratedAt time.Time     `json:"generated_at"`
	Files       []FileSummary `json:"files"`
	Errors      []ParseError  `json:"errors,omitempty"`
}

func (idx *Index) FileCount() int {
	if idx == nil {
		return 0
	}
	return len(idx.Files)
}

func (idx *Index) SymbolCount() int {
	if idx == nil {
		return 0
	}

	total := 0
	for _, file := range idx.Files {
		total += len(file.Symbols)
	}
	return total
}
