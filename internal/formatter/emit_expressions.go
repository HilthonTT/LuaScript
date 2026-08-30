package formatter

// Doc emission for expressions.

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
		// Preserve the original literal (e.g. hex `0xFF`) when available.
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

// ifExpr renders the Luau-style conditional expression, breaking before
// `elseif`/`else` when the whole expression overflows the line.
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

// binary emits a left-flat / right-break chain. Repeated same-precedence
// operators get grouped so a long `a + b + c + d` breaks before each
// operator at the same indent rather than nesting.
// binary renders `a op b`, flattening a run of the SAME operator into a
// single group.
//
// Flattening is what makes the output stable. Emitted one node at a time,
// `a .. b .. c .. d` nests a group inside a group inside a group, each
// adding its own indent; when the outermost breaks, the inner ones make
// their own fit decisions and the result is a staircase. Re-formatting that
// staircase produces different decisions again — i.e. formatting was not
// idempotent, so its output was not a canonical form.
//
// With the chain flat, every operand breaks together at one indent level.
//
// Only children with the *same* operator are absorbed, and explicit
// parentheses survive parsing as ParenExpression nodes rather than
// BinaryExpression ones — so `a - (b - c)` is never flattened into
// `a - b - c`, which would change what it means.
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

// binaryChain collects the operands of a maximal run of `op`, left to right.
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

// callArgs renders `(a, b, c)` with break-on-overflow behavior. Single
// table-constructor or string args still use parens for v1 (Lua's
// paren-free call syntax is preserved by the parser as a one-element Args
// slice — we always render explicit parens for clarity).
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

// table renders `{}` either flat (`{ a, b }`) or broken with one field per
// line and a trailing comma. Empty tables stay as `{}`.
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
			// Record entry: Key is *Identifier or rendered as a name.
			var key Doc
			if id, ok := f.Key.(*ast.Identifier); ok {
				key = text(id.Name)
			} else {
				key = e.expr(f.Key, opts)
			}
			fields[i] = concat(key, text(" = "), e.expr(f.Value, opts))
		}
	}
	// Flat: `{ a, b, c }`. Broken: each field on its own line + trailing comma.
	flatSep := concat(text(","), line())
	body := join(flatSep, fields...)
	return group(concat(
		text("{"),
		nest(opts.indent(), concat(line(), body, trailingCommaIfBreak())),
		line(),
		text("}"),
	))
}

// trailingCommaIfBreak is a tiny custom Doc that emits "," only when the
// enclosing group is in break mode. Approximated for v1 by emitting an
// always-present softLine — we don't yet have a real `ifBreak` primitive.
// For now, omit the trailing comma; it can be added when ifBreak lands.
func trailingCommaIfBreak() Doc { return nilDoc() }
