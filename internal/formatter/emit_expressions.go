package formatter

import (
	"strconv"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func (e *emitter) expr(x ast.Expression, opts Options) Doc {
	switch v := x.(type) {
	case *ast.NilLiteral:
		return text("nil")
	case *ast.BooleanLiteral:
		if v.Value {
			return text("true")
		}
		return text("false")
	case *ast.IntegerLiteral:
		if v.Token.Literal != "" {
			return text(v.Token.Literal)
		}
		return text(strconv.FormatInt(v.Value, 10))
	case *ast.FloatLiteral:
		if v.Token.Literal != "" {
			return text(v.Token.Literal)
		}
		return text(strconv.FormatFloat(v.Value, 'g', -1, 64))
	case *ast.StringLiteral:
		if v.IsLong {
			return text("[[" + v.Value + "]]")
		}
		return text(strconv.Quote(v.Value))
	case *ast.VarargExpression:
		return text("...")
	case *ast.Identifier:
		return text(v.Name)
	case *ast.UnaryExpression:
		return e.unary(v, opts)
	case *ast.BinaryExpression:
		return e.binary(v, opts)
	case *ast.ParenExpression:
		return concat(text("("), e.expr(v.Inner, opts), text(")"))
	case *ast.IndexExpression:
		return e.index(v, opts)
	case *ast.CallExpression:
		return e.call(v, opts)
	case *ast.MethodCallExpression:
		return e.methodCall(v, opts)
	case *ast.TableConstructor:
		return e.table(v, opts)
	case *ast.FunctionExpression:
		return concat(text("function"), e.funcSig(v, opts), e.funcBody(v, opts))
	case *ast.TypeAssertionExpression:
		return concat(e.expr(v.Expr, opts), text(" :: "), e.typeNode(v.Type, opts))
	case *ast.IfExpression:
		return e.ifExpr(v, opts)
	}
	return text(x.String())
}

func (e *emitter) ifExpr(ie *ast.IfExpression, opts Options) Doc {
	var parts []Doc
	for i, c := range ie.Clauses {
		kw := "if "
		if i > 0 {
			parts = append(parts, line(), text("elseif "))
			kw = ""
		}
		if kw != "" {
			parts = append(parts, text(kw))
		}
		parts = append(parts, e.expr(c.Condition, opts), text(" then "), e.expr(c.Value, opts))
	}
	parts = append(parts, line(), text("else "), e.expr(ie.Else, opts))
	return group(concat(parts[0], nest(opts.indent(), concat(parts[1:]...))))
}

func (e *emitter) unary(u *ast.UnaryExpression, opts Options) Doc {
	if u.Op == "not" {
		return concat(text("not "), e.expr(u.Operand, opts))
	}
	return concat(text(u.Op), e.expr(u.Operand, opts))
}

func (e *emitter) binary(b *ast.BinaryExpression, opts Options) Doc {
	operands := e.binaryChain(b, b.Op, opts)
	rest := make([]Doc, 0, len(operands)-1)
	for _, operand := range operands[1:] {
		rest = append(rest, line(), text(b.Op+" "), operand)
	}
	return group(concat(
		operands[0],
		nest(opts.indent(), concat(rest...)),
	))
}

func (e *emitter) binaryChain(x ast.Expression, op string, opts Options) []Doc {
	if b, ok := x.(*ast.BinaryExpression); ok && b.Op == op {
		return append(
			e.binaryChain(b.Left, op, opts),
			e.binaryChain(b.Right, op, opts)...,
		)
	}
	return []Doc{e.expr(x, opts)}
}

func (e *emitter) index(ix *ast.IndexExpression, opts Options) Doc {
	if ix.IsDot {
		if s, ok := ix.Index.(*ast.StringLiteral); ok {
			return concat(e.expr(ix.Object, opts), text("."), text(s.Value))
		}
	}
	return concat(e.expr(ix.Object, opts), text("["), e.expr(ix.Index, opts), text("]"))
}

func (e *emitter) call(c *ast.CallExpression, opts Options) Doc {
	args := e.callArgs(c.Args, opts)
	return concat(e.expr(c.Func, opts), args)
}

func (e *emitter) methodCall(m *ast.MethodCallExpression, opts Options) Doc {
	args := e.callArgs(m.Args, opts)
	return concat(e.expr(m.Object, opts), text(":"), text(m.Method), args)
}

func (e *emitter) callArgs(args []ast.Expression, opts Options) Doc {
	if len(args) == 0 {
		return text("()")
	}
	ds := make([]Doc, len(args))
	for i, a := range args {
		ds[i] = e.expr(a, opts)
	}
	return group(concat(
		text("("),
		nest(opts.indent(), concat(softLine(), join(concat(text(","), line()), ds...))),
		softLine(),
		text(")"),
	))
}

func (e *emitter) table(t *ast.TableConstructor, opts Options) Doc {
	if len(t.Fields) == 0 {
		return text("{}")
	}
	fields := make([]Doc, len(t.Fields))
	for i, f := range t.Fields {
		switch {
		case f.Key == nil:
			fields[i] = e.expr(f.Value, opts)
		case f.IsBracketed:
			fields[i] = concat(text("["), e.expr(f.Key, opts), text("] = "), e.expr(f.Value, opts))
		default:
			var key Doc
			if id, ok := f.Key.(*ast.Identifier); ok {
				key = text(id.Name)
			} else {
				key = e.expr(f.Key, opts)
			}
			fields[i] = concat(key, text(" = "), e.expr(f.Value, opts))
		}
	}
	flatSep := concat(text(","), line())
	body := join(flatSep, fields...)
	return group(concat(
		text("{"),
		nest(opts.indent(), concat(line(), body, trailingCommaIfBreak())),
		line(),
		text("}"),
	))
}

func trailingCommaIfBreak() Doc { return nilDoc() }
