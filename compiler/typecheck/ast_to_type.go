package typecheck

import (
	"strings"

	"github.com/hilthontt/luascript/compiler/ast"
)

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
		// In-scope generic parameter (e.g. the `T` inside `type Box<T> = ...`
		// or a generic function body) — becomes an opaque type variable.
		if c.isTypeParam(t.Name) {
			if len(t.TypeArgs) > 0 {
				c.errf(n.Line(), "not-generic",
					"type parameter %q does not take type arguments", t.Name)
			}
			return newTypeParam(t.Name)
		}
		// Generic alias reference: `Box<number>` → substitute into template.
		if scheme, ok := c.env.genericAliases[t.Name]; ok {
			return c.instantiateAlias(n.Line(), t, scheme)
		}
		if len(t.TypeArgs) > 0 {
			c.errf(n.Line(), "not-generic",
				"type %q is not generic and does not take type arguments", t.Name)
			// Fall through and resolve the base name for a best-effort type.
		}
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

// instantiateAlias resolves a generic alias reference `Name<A1, A2>` by
// substituting the supplied type arguments into the scheme's template.
// Arity mismatches are reported; missing args are filled with `any` and
// extras ignored so the checker keeps making progress. The result carries
// a display AliasName like `Box<number>`.
func (c *checker) instantiateAlias(line int, t *ast.TypeName, scheme *GenericScheme) *Type {
	if len(t.TypeArgs) != len(scheme.Params) {
		c.errf(line, "type-arity",
			"generic type %q expects %d type argument(s), got %d",
			t.Name, len(scheme.Params), len(t.TypeArgs))
	}
	subst := make(map[string]*Type, len(scheme.Params))
	argStrs := make([]string, len(scheme.Params))
	for i, p := range scheme.Params {
		if i < len(t.TypeArgs) {
			at := c.resolveAST(t.TypeArgs[i])
			subst[p] = at
			argStrs[i] = at.String()
		} else {
			subst[p] = anyT
			argStrs[i] = anyT.String()
		}
	}
	if scheme.Template == nil {
		return anyT
	}
	inst := substituteType(scheme.Template, subst)
	named := *inst
	named.AliasName = t.Name + "<" + strings.Join(argStrs, ", ") + ">"
	return &named
}
