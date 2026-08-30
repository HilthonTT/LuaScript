package formatter

// Doc emission for type annotations.

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (e *emitter) typeNode(t ast.TypeNode, opts Options) Doc {
	switch v := t.(type) {
	case *ast.TypePrimitive:
		return text(v.Name)
	case *ast.TypeLiteral:
		// Raw is the source spelling, so a singleton round-trips exactly as
		// written (`0x10` stays `0x10`).
		return text(v.Raw)
	case *ast.TypeName:
		return text(v.Name)
	case *ast.TypeOptional:
		return concat(e.typeNode(v.Inner, opts), text("?"))
	case *ast.TypeUnion:
		ms := make([]Doc, len(v.Members))
		for i, m := range v.Members {
			ms[i] = e.typeNode(m, opts)
		}
		return group(join(concat(line(), text("| ")), ms...))
	case *ast.TypeFunction:
		return e.typeFunc(v, opts)
	case *ast.TypeTable:
		return e.typeTable(v, opts)
	}
	return text(t.String())
}

func (e *emitter) typeFunc(t *ast.TypeFunction, opts Options) Doc {
	var ps []Doc
	for i, p := range t.Params {
		if i < len(t.ParamNames) && t.ParamNames[i] != "" {
			ps = append(ps, concat(text(t.ParamNames[i]), text(": "), e.typeNode(p, opts)))
		} else {
			ps = append(ps, e.typeNode(p, opts))
		}
	}
	if t.IsVararg {
		if t.VarargType != nil {
			ps = append(ps, concat(text("...: "), e.typeNode(t.VarargType, opts)))
		} else {
			ps = append(ps, text("..."))
		}
	}
	params := group(concat(
		text("("),
		nest(opts.indent(), concat(softLine(), join(concat(text(","), line()), ps...))),
		softLine(),
		text(")"),
	))
	var ret Doc
	switch len(t.Returns) {
	case 0:
		ret = text("()")
	case 1:
		ret = e.typeNode(t.Returns[0], opts)
	default:
		rs := make([]Doc, len(t.Returns))
		for i, r := range t.Returns {
			rs[i] = e.typeNode(r, opts)
		}
		ret = concat(text("("), join(text(", "), rs...), text(")"))
	}
	return concat(params, text(" -> "), ret)
}

func (e *emitter) typeTable(t *ast.TypeTable, opts Options) Doc {
	if t.Indexer == nil && len(t.Fields) == 0 {
		return text("{}")
	}
	var parts []Doc
	if t.Indexer != nil {
		parts = append(parts, concat(
			text("["), e.typeNode(t.Indexer.Key, opts), text("]: "),
			e.typeNode(t.Indexer.Value, opts),
		))
	}
	for _, f := range t.Fields {
		parts = append(parts, concat(text(f.Key), text(": "), e.typeNode(f.Value, opts)))
	}
	return group(concat(
		text("{"),
		nest(opts.indent(), concat(line(), join(concat(text(","), line()), parts...))),
		line(),
		text("}"),
	))
}
