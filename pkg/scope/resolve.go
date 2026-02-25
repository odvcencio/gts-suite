package scope

// ResolveAll walks the scope tree and resolves all references.
func ResolveAll(root *Scope) {
	resolveScope(root)
}

func resolveScope(s *Scope) {
	for i := range s.Refs {
		s.Refs[i].Resolved = resolveRef(&s.Refs[i], s)
	}
	for _, child := range s.Children {
		resolveScope(child)
	}
}

// resolveRef walks up the scope chain looking for a definition matching the reference.
func resolveRef(ref *Ref, from *Scope) *Definition {
	def := lookupName(ref.Name, from)
	if def == nil {
		return nil
	}
	// Simple name reference (no dotted access)
	if ref.Member == "" {
		return def
	}
	// Dotted access: resolve the member inside the definition's scope
	if def.Scope == nil {
		return nil
	}
	return lookupInScope(ref.Member, def.Scope)
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
