// Package treesitter node type classification maps for tree-sitter AST nodes.
// These maps categorize tree-sitter node types into semantic groups used by
// structural analysis tools (entity extraction, code navigation, etc.).

package treesitter

// ImportNodeTypes lists tree-sitter node types that represent import statements
// across supported languages.
var ImportNodeTypes = map[string]bool{
	"import_spec":               true, // Go
	"import_declaration":        true, // Java
	"import":                    true, // JavaScript
	"import_statement":          true, // Python / JS / TS
	"import_from_statement":     true, // Python
	"use_declaration":           true, // Rust / PHP
	"namespace_use_declaration": true, // PHP
	"using_directive":           true, // C#
	"import_header":             true, // Kotlin
	"preproc_include":           true, // C/C++
}

// DeclarationNodeTypes lists tree-sitter node types that represent top-level
// declarations (functions, types, classes, etc.) across supported languages.
var DeclarationNodeTypes = map[string]bool{
	"function_declaration":     true,
	"function_definition":      true,
	"function_item":            true,
	"method_declaration":       true,
	"method_definition":        true,
	"type_declaration":         true,
	"class_definition":         true,
	"class_declaration":        true,
	"struct_item":              true,
	"struct_declaration":       true,
	"enum_item":                true,
	"enum_declaration":         true,
	"trait_item":               true,
	"trait_declaration":        true,
	"impl_item":                true,
	"interface_declaration":    true,
	"const_declaration":        true,
	"const_item":               true,
	"var_declaration":          true,
	"property_declaration":     true,
	"decorated_definition":     true,
	"export_statement":         true,
	"lexical_declaration":      true,
	"type_spec":                true,
	"type_alias_declaration":   true,
	"short_var_declaration":    true,
	"module_definition":        true, // Ruby / Elixir
	"module_declaration":       true, // Swift / Java-like
	"namespace_definition":     true, // PHP
	"namespace_declaration":    true, // C#
	"protocol_declaration":     true, // Swift
	"record_declaration":       true, // C#
	"object_declaration":       true, // Kotlin / Scala
	"companion_object":         true, // Kotlin
	"singleton_method":         true, // Ruby
	"typealias_declaration":    true, // Swift
	"data_declaration":         true, // Haskell
	"type_signature":           true, // Haskell
	"value_declaration":        true, // Haskell / Scala
	"class_specifier":          true, // C++
	"function_definition_item": true, // Scala variant
}

// PreambleNodeTypes lists tree-sitter node types that represent file-level
// preamble (package clause, module declaration).
var PreambleNodeTypes = map[string]bool{
	"package_clause":        true,
	"package_declaration":   true,
	"module":                true,
	"module_declaration":    true,
	"namespace_declaration": true,
}

// CommentNodeTypes lists tree-sitter node types that represent comments.
var CommentNodeTypes = map[string]bool{
	"comment":               true,
	"block_comment":         true,
	"line_comment":          true,
	"documentation_comment": true,
}

// NameIdentifierTypes lists tree-sitter node types that represent name
// identifiers found as children of declarations.
var NameIdentifierTypes = map[string]bool{
	"identifier":           true,
	"type_identifier":      true,
	"field_identifier":     true,
	"package_identifier":   true,
	"property_identifier":  true,
	"constant_identifier":  true,
	"namespace_identifier": true,
	"simple_identifier":    true,
	"name":                 true,
}
