package typecheck

import "github.com/hilthontt/luascript/compiler/ast"

// resolveAST converts an ast.TypeNode (parsed type syntax) into the
// checker's internal Type. Aliases are looked up against the env's alias
// table; unknown names produce KindNever (which surfaces as an
// "unknown type" error at the use site without crashing the walker).
//
// Called from two places:
//   - the alias-resolution pre-pass (env.aliases is empty on first call;
//     forward references thus resolve to KindNever and are fixed up by a
//     second pass once all aliases are registered).
//   - the per-statement walker, which reads annotation TypeNodes and
//     converts them with all aliases already populated.
func (c *checker) resolveAST(n ast.TypeNode) *Type {
	if n == nil {
		return anyT
	}
	switch t := n.(type) {
	case *ast.TypePrimitive:
		if p, ok := primitiveByName[t.Name]; ok {
			return p
		}
		// `nil` parsed as a "primitive" via parseTypeAtom's special case;
		// also covered by primitiveByName. Anything else is a parser bug.
		return anyT
	case *ast.TypeName:
		// Alias reference — look up. Annotate with the alias name so
		// diagnostics show the user's spelling rather than the expansion.
		resolved := c.env.alias(t.Name)
		if resolved == neverT {
			c.errf(n.Line(), "unknown-type", "unknown type %q", t.Name)
		}
		// Don't overwrite an existing AliasName (we want the outermost
		// alias name to stick when nested aliases are involved).
		if resolved.AliasName == "" {
			withName := *resolved
			withName.AliasName = t.Name
			return &withName
		}
		return resolved
	case *ast.TypeOptional:
		return Optional(c.resolveAST(t.Inner))
	case *ast.TypeUnion:
		members := make([]*Type, len(t.Members))
		for i, m := range t.Members {
			members[i] = c.resolveAST(m)
		}
		return NewUnion(members...)
	case *ast.TypeFunction:
		params := make([]*Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = c.resolveAST(p)
		}
		returns := make([]*Type, len(t.Returns))
		for i, r := range t.Returns {
			returns[i] = c.resolveAST(r)
		}
		var va *Type
		if t.IsVararg && t.VarargType != nil {
			va = c.resolveAST(t.VarargType)
		}
		return NewFunction(params, returns, t.IsVararg, va)
	case *ast.TypeTable:
		fields := make([]TableField, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = TableField{Key: f.Key, Type: c.resolveAST(f.Value)}
		}
		var idx *Indexer
		if t.Indexer != nil {
			idx = &Indexer{
				Key:   c.resolveAST(t.Indexer.Key),
				Value: c.resolveAST(t.Indexer.Value),
			}
		}
		return NewTable(fields, idx)
	}
	return anyT
}
