package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) resolveAST(n ast.TypeNode) *Type {
	if n == nil {
		return anyT
	}
	switch t := n.(type) {
	case *ast.TypePrimitive:
		if p, ok := primitiveByName[t.Name]; ok {
			return p
		}
		return anyT
	case *ast.TypeLiteral:
		switch t.Kind {
		case ast.LiteralString:
			return NewStringLiteral(t.Str, t.Raw)
		case ast.LiteralNumber:
			return NewNumberLiteral(t.Num, t.Raw)
		case ast.LiteralBoolean:
			return NewBooleanLiteral(t.Bool, t.Raw)
		}
		return anyT
	case *ast.TypeName:
		resolved := c.env.alias(t.Name)
		if resolved == neverT {
			c.errf(n.Line(), "unknown-type", "unknown type %q", t.Name)
		}
		if resolved.AliasName == "" {
			withName := *resolved
			withName.AliasName = t.Name
			return &withName
		}
		return resolved
	case *ast.TypeApplication:
		return c.resolveTypeApplication(t)
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
