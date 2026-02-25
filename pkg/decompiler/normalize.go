package decompiler

import (
	"bytes"
	"regexp"
)

var (
	ghidraWarningBlock = regexp.MustCompile(`/\*\s*WARNING:.*?\*/`)
	ghidraWarningLine  = regexp.MustCompile(`(?m)^\s*//\s*WARNING:.*$`)
	// Ordered longest-first so "undefined4" is replaced before "undefined".
	undefinedTypes = [][2]string{
		{"undefined1", "uint8_t"},
		{"undefined2", "uint16_t"},
		{"undefined4", "uint32_t"},
		{"undefined8", "uint64_t"},
		{"undefined", "void*"},
	}
)

// NormalizeGhidra strips Ghidra-specific annotations from decompiled C output.
func NormalizeGhidra(src []byte) []byte {
	// Strip /* WARNING: ... */ block comments
	result := ghidraWarningBlock.ReplaceAll(src, nil)
	// Strip // WARNING: lines
	result = ghidraWarningLine.ReplaceAll(result, nil)
	// Replace undefined types (longest-first to avoid partial matches)
	for _, pair := range undefinedTypes {
		result = bytes.ReplaceAll(result, []byte(pair[0]), []byte(pair[1]))
	}
	return result
}

// NormalizeIDA strips IDA-specific annotations from decompiled C output.
func NormalizeIDA(src []byte) []byte {
	// IDA uses __int64, __int32, etc.
	result := bytes.ReplaceAll(src, []byte("__int64"), []byte("int64_t"))
	result = bytes.ReplaceAll(result, []byte("__int32"), []byte("int32_t"))
	result = bytes.ReplaceAll(result, []byte("__int16"), []byte("int16_t"))
	result = bytes.ReplaceAll(result, []byte("__int8"), []byte("int8_t"))
	result = bytes.ReplaceAll(result, []byte("_BYTE"), []byte("uint8_t"))
	result = bytes.ReplaceAll(result, []byte("_WORD"), []byte("uint16_t"))
	result = bytes.ReplaceAll(result, []byte("_DWORD"), []byte("uint32_t"))
	result = bytes.ReplaceAll(result, []byte("_QWORD"), []byte("uint64_t"))
	return result
}
