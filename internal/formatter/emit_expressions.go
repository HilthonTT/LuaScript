package formatter

import (
	"strconv"

	"github.com/hilthontt/luascript/v3/internal/compiler/ast"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser/precedence"
	"github.com/hilthontt/luascript/v3/internal/compiler/token"
)

var binaryOpTokens = map[string]token.Type{
	"+": token.Plus, "-": token.Minus, "*": token.Asterisk, "/": token.Slash,
	"//": token.FloorDiv, "%": token.Percent, "^": token.Caret, "..": token.Concat,
	"==": token.Eq, "~=": token.NotEq, "<": token.LT, "<=": token.LTE,
	">": token.GT, ">=": token.GTE, "&": token.Ampersand, "|": token.Pipe,
	"~": token.Tilde, "<<": token.LShift, ">>": token.RShift,
	"and": token.And, "or": token.Or, "??": token.Coalesce,
}

func binaryPrec(op string) (prec int, rightAssoc bool) {
	t := binaryOpTokens[op]
	return precedence.LookupTable[t], precedence.IsRightAssoc(t)
}

// The parser keeps source parentheses as ParenExpression, but its desugarings
// (string interpolation, compound assignment) build bare BinaryExpressions.
// Printing those without parentheses would re-parse with a different shape,
// so the helpers below parenthesize by precedence.

func (e *emitter) parenthesized(x ast.Expression, opts Options) Doc {
	return concat(text("("), e.expr(x, opts), text(")"))
}

// needsPrefixParens reports whether x, as the base of a call, index or method
// call, must be wrapped in parentheses to re-parse as that base. The parser
// accepts literals and table constructors bare, so those are left alone.
func needsPrefixParens(x ast.Expression) bool {
	switch x.(type) {
	case *ast.BinaryExpression, *ast.UnaryExpression, *ast.IfExpression,
		*ast.TypeAssertionExpression:
		return true
	}
	return false
}

// prefixExpr emits x as the base of a call, index or method call.
func (e *emitter) prefixExpr(x ast.Expression, opts Options) Doc {
	if needsPrefixParens(x) {
		return e.parenthesized(x, opts)
	}
	return e.expr(x, opts)
}

// operand emits x as an operand of a binary operator with precedence prec;
// left reports which side it sits on.
func (e *emitter) operand(x ast.Expression, prec int, rightAssoc, left bool, opts Options) Doc {
	switch v := x.(type) {
	case *ast.BinaryExpression:
		cp, _ := binaryPrec(v.Op)
		if cp < prec || (cp == prec && left == rightAssoc) {
			return e.parenthesized(x, opts)
		}
	case *ast.UnaryExpression:
		if left && prec >= precedence.Pow {
			return e.parenthesized(x, opts)
		}
	}
	return e.expr(x, opts)
}

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
		return text(quoteLua(v.Value))
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
	operand := e.expr(u.Operand, opts)
	if b, ok := u.Operand.(*ast.BinaryExpression); ok && b.Op != "^" {
		operand = e.parenthesized(u.Operand, opts)
	}
	if u.Op == "not" {
		return concat(text("not "), operand)
	}
	// "- -x" must keep its space: "--x" is a comment.
	if inner, ok := u.Operand.(*ast.UnaryExpression); ok && u.Op == "-" && inner.Op == "-" {
		return concat(text("- "), operand)
	}
	return concat(text(u.Op), operand)
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

// binaryChain flattens a run of the same operator along its associative side
// (left for most operators, right for .. and ^) into a list of operands.
func (e *emitter) binaryChain(x ast.Expression, op string, opts Options) []Doc {
	prec, rightAssoc := binaryPrec(op)
	var docs []Doc
	if rightAssoc {
		for {
			b, ok := x.(*ast.BinaryExpression)
			if !ok || b.Op != op {
				return append(docs, e.operand(x, prec, true, false, opts))
			}
			if l, ok := b.Left.(*ast.BinaryExpression); ok && op == ".." && l.Op == ".." {
				// Interpolation builds ((a .. b) .. c); string concatenation
				// is associative, so print it as the flat a .. b .. c.
				docs = append(docs, e.binaryChain(l, op, opts)...)
			} else {
				docs = append(docs, e.operand(b.Left, prec, true, true, opts))
			}
			x = b.Right
		}
	}
	var rights []Doc
	for {
		b, ok := x.(*ast.BinaryExpression)
		if !ok || b.Op != op {
			docs = append(docs, e.operand(x, prec, false, true, opts))
			break
		}
		rights = append(rights, e.operand(b.Right, prec, false, false, opts))
		x = b.Left
	}
	for i := len(rights) - 1; i >= 0; i-- {
		docs = append(docs, rights[i])
	}
	return docs
}

func (e *emitter) index(ix *ast.IndexExpression, opts Options) Doc {
	q := ""
	if ix.Optional {
		q = "?"
	}
	if ix.IsDot {
		if s, ok := ix.Index.(*ast.StringLiteral); ok {
			return concat(e.prefixExpr(ix.Object, opts), text(q+"."), text(s.Value))
		}
	}
	return concat(e.prefixExpr(ix.Object, opts), text(q+"["), e.expr(ix.Index, opts), text("]"))
}

func (e *emitter) call(c *ast.CallExpression, opts Options) Doc {
	args := e.callArgs(c.Args, opts)
	return concat(e.prefixExpr(c.Func, opts), args)
}

func (e *emitter) methodCall(m *ast.MethodCallExpression, opts Options) Doc {
	args := e.callArgs(m.Args, opts)
	sep := ":"
	if m.Optional {
		sep = "?:"
	}
	return concat(e.prefixExpr(m.Object, opts), text(sep), text(m.Method), args)
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
		case f.IsSpread:
			fields[i] = concat(text("..."), e.expr(f.Value, opts))
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
