package scope

import "strings"

// ResolveAll walks the scope tree and resolves all references
// using only scope-chain lookup (no cross-file type resolution).
func ResolveAll(root *Scope) {
	resolveScope(root, nil)
}

// ResolveAllGraph walks the scope tree and resolves all references,
// using the graph for cross-file type-directed resolution.
func ResolveAllGraph(root *Scope, graph *Graph) {
	resolveScope(root, graph)
}

func resolveScope(s *Scope, graph *Graph) {
	for i := range s.Refs {
		s.Refs[i].Resolved = resolveRef(&s.Refs[i], s, graph)
	}
	for _, child := range s.Children {
		resolveScope(child, graph)
	}
}

// resolveRef walks up the scope chain looking for a definition matching the reference.
func resolveRef(ref *Ref, from *Scope, graph *Graph) *Definition {
	def := lookupName(ref.Name, from)
	if def == nil {
		return nil
	}
	// Simple name reference (no dotted access)
	if ref.Member == "" {
		return def
	}
	// Direct scope lookup (existing behavior)
	if def.Scope != nil {
		if m := lookupInScope(ref.Member, def.Scope); m != nil {
			return m
		}
	}
	// Type-directed lookup: resolve via TypeAnnot
	if def.TypeAnnot != "" && graph != nil {
		typeDef := lookupType(def.TypeAnnot, from, graph)
		if typeDef != nil && typeDef.Scope != nil {
			if m := lookupInScope(ref.Member, typeDef.Scope); m != nil {
				return m
			}
		}
	}
	// Inheritance chain
	for _, base := range def.BaseClasses {
		if graph == nil {
			break
		}
		baseDef := lookupType(base, from, graph)
		if baseDef != nil && baseDef.Scope != nil {
			if m := lookupInScope(ref.Member, baseDef.Scope); m != nil {
				return m
			}
		}
	}
	return nil
}

// lookupName searches for a name starting at the given scope, walking up to parents.
func lookupName(name string, s *Scope) *Definition {
	for cur := s; cur != nil; cur = cur.Parent {
		if d := lookupInScope(name, cur); d != nil {
			return d
		}
	}
	return nil
}

// lookupInScope searches for a name in a single scope (no parent traversal).
func lookupInScope(name string, s *Scope) *Definition {
	for i := range s.Defs {
		if s.Defs[i].Name == name {
			return &s.Defs[i]
		}
	}
	return nil
}

// lookupType resolves a type name to its definition using the scope chain
// and the graph's package scopes.
func lookupType(typeName string, from *Scope, graph *Graph) *Definition {
	// Strip pointer/slice prefixes
	clean := strings.TrimLeft(typeName, "*[]")

	// Check if qualified (pkg.Type)
	if parts := strings.SplitN(clean, ".", 2); len(parts) == 2 {
		if graph != nil {
			if pkgScope := graph.PackageScope(parts[0]); pkgScope != nil {
				return lookupInScope(parts[1], pkgScope)
			}
		}
	}

	// Unqualified: walk scope chain
	return lookupName(clean, from)
}
