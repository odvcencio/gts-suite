package scope

import (
	"embed"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

//go:embed rules/*.scm
var rulesFS embed.FS

// Rules holds a compiled scope query for a language.
type Rules struct {
	Language string
	Query    *gotreesitter.Query
}

// LoadRules loads and compiles the .scm scope rules for a language.
func LoadRules(langName string, lang *gotreesitter.Language) (*Rules, error) {
	data, err := rulesFS.ReadFile("rules/" + langName + ".scm")
	if err != nil {
		return nil, fmt.Errorf("scope rules not found for %s: %w", langName, err)
	}
	q, err := gotreesitter.NewQuery(string(data), lang)
	if err != nil {
		return nil, fmt.Errorf("compile scope rules for %s: %w", langName, err)
	}
	return &Rules{Language: langName, Query: q}, nil
}

// defKey identifies a definition by name and position, used to deduplicate
// overlapping query patterns that match the same AST node.
type defKey struct {
	name     string
	startRow uint32
	startCol uint32
}

// BuildFileScope constructs a scope tree from a parse tree using scope rules.
func BuildFileScope(
	tree *gotreesitter.Tree,
	lang *gotreesitter.Language,
	src []byte,
	rules *Rules,
	path string,
) *Scope {
	root := NewScope(ScopeFile, nil)
	seen := make(map[defKey]bool)

	cursor := rules.Query.Exec(tree.RootNode(), lang, src)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		processMatch(match, root, src, path, seen)
	}
	return root
}

// addDefDedup adds a definition to the scope, deduplicating by name and position.
func addDefDedup(fileScope *Scope, def Definition, seen map[defKey]bool) {
	sp := defKey{
		name:     def.Name,
		startRow: uint32(def.Loc.StartLine),
		startCol: uint32(def.Loc.StartCol),
	}
	if seen[sp] {
		return
	}
	seen[sp] = true
	fileScope.AddDef(def)
}

func processMatch(
	match gotreesitter.QueryMatch,
	fileScope *Scope,
	src []byte,
	path string,
	seen map[defKey]bool,
) {
	for _, cap := range match.Captures {
		name := cap.Name
		node := cap.Node
		text := node.Text(src)

		switch {
		case name == "def.function":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefFunction,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.method":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefMethod,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.variable" || name == "def.variable.notype":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefVariable,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.variable.type":
			// Attach type annotation to most recent variable def
			if len(fileScope.Defs) > 0 {
				last := &fileScope.Defs[len(fileScope.Defs)-1]
				if last.Kind == DefVariable {
					last.TypeAnnot = text
				}
			}

		case name == "def.constant":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefConstant,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.type":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefType,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.param":
			addDefDedup(fileScope, Definition{
				Name: text,
				Kind: DefParam,
				Loc:  nodeLocation(node, path),
			}, seen)

		case name == "def.import.path":
			addDefDedup(fileScope, Definition{
				Name:       importName(text),
				Kind:       DefImport,
				ImportPath: text,
				Loc:        nodeLocation(node, path),
			}, seen)

		case name == "def.import.alias":
			// Will be followed by def.import.aliased.path in same match

		case name == "def.import.aliased.path":
			// Find the alias capture in this same match
			alias := ""
			for _, c := range match.Captures {
				if c.Name == "def.import.alias" {
					alias = c.Node.Text(src)
					break
				}
			}
			addDefDedup(fileScope, Definition{
				Name:       alias,
				Kind:       DefImport,
				ImportPath: text,
				Loc:        nodeLocation(node, path),
			}, seen)

		case name == "ref.operand":
			fileScope.AddRef(Ref{
				Name: text,
				Loc:  nodeLocation(node, path),
			})

		case name == "ref.member":
			// Attach member to most recent ref
			if len(fileScope.Refs) > 0 {
				fileScope.Refs[len(fileScope.Refs)-1].Member = text
			}

		case name == "ref.call" || name == "ref":
			fileScope.AddRef(Ref{
				Name: text,
				Loc:  nodeLocation(node, path),
			})
		}
	}
}

func nodeLocation(node *gotreesitter.Node, path string) Location {
	sp := node.StartPoint()
	ep := node.EndPoint()
	return Location{
		File:      path,
		StartLine: int(sp.Row) + 1,
		EndLine:   int(ep.Row) + 1,
		StartCol:  int(sp.Column),
		EndCol:    int(ep.Column),
	}
}

// importName extracts the package name from an import path like "\"fmt\"".
func importName(path string) string {
	path = strings.Trim(path, "\"")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
