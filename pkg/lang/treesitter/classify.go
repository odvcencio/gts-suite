// Package treesitter node type classification maps for tree-sitter AST nodes.
// These maps categorize tree-sitter node types into semantic groups used by
// structural analysis tools (entity extraction, code navigation, etc.).

package treesitter

// ImportNodeTypes lists tree-sitter node types that represent import statements
// across supported languages.
var ImportNodeTypes = map[string]bool{
	"import_declaration":    true,
	"import_statement":      true,
	"import_from_statement": true,
	"use_declaration":       true,
	"preproc_include":       true,
}

// DeclarationNodeTypes lists tree-sitter node types that represent top-level
// declarations (functions, types, classes, etc.) across supported languages.
var DeclarationNodeTypes = map[string]bool{
	"function_declaration":  true,
	"function_definition":   true,
	"function_item":         true,
	"method_declaration":    true,
	"type_declaration":      true,
	"class_definition":      true,
	"class_declaration":     true,
	"struct_item":           true,
	"enum_item":             true,
	"trait_item":            true,
	"impl_item":             true,
	"interface_declaration": true,
	"const_declaration":     true,
	"var_declaration":       true,
	"decorated_definition":  true,
	"export_statement":      true,
	"lexical_declaration":   true,
	"type_spec":             true,
	"short_var_declaration": true,
}

// PreambleNodeTypes lists tree-sitter node types that represent file-level
// preamble (package clause, module declaration).
var PreambleNodeTypes = map[string]bool{
	"package_clause":      true,
	"package_declaration": true,
	"module":              true,
}

// CommentNodeTypes lists tree-sitter node types that represent comments.
var CommentNodeTypes = map[string]bool{
	"comment":       true,
	"block_comment": true,
	"line_comment":  true,
}

// NameIdentifierTypes lists tree-sitter node types that represent name
// identifiers found as children of declarations.
var NameIdentifierTypes = map[string]bool{
	"identifier":          true,
	"type_identifier":     true,
	"field_identifier":    true,
	"package_identifier":  true,
	"property_identifier": true,
}
