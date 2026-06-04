package formatter

import (
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/compiler/ast"
)

// emitter walks an ast.Program and produces a Doc. It also interleaves
// preserved trivia (comments, blank lines) by source line, since the parser
// drops them.
//
// Trivia interleaving works at block boundaries: before emitting a
// statement we flush every piece of trivia whose start line precedes the
// statement's start line. Inline trivia on the same line as a statement is
// out of scope for v1 — flagged in the package doc.
type emitter struct {
	trivia []Trivia
	ti     int // cursor into trivia
}

// Options controls formatter behavior. Empty zero value is "default style":
// 80-col target, 2-space indent. Kept tiny on purpose — once we ship more
// surface area we can grow it.
type Options struct {
	Width  int // target line width; 0 → 80
	Indent int // spaces per indent level; 0 → 2
}

func (o Options) width() int {
	if o.Width <= 0 {
		return 80
	}
	return o.Width
}

func (o Options) indent() int {
	if o.Indent <= 0 {
		return 2
	}
	return o.Indent
}

// flushRemainingTrivia drains any trivia past the last AST node. We drop
// trailing blank lines (the trailing newline is added separately) but keep
// any trailing comments.
func (e *emitter) flushRemainingTrivia() Doc {
	var parts []Doc
	first := true
	for e.ti < len(e.trivia) {
		t := e.trivia[e.ti]
		e.ti++
		if t.Kind == BlankLine {
			continue
		}
		if !first {
			parts = append(parts, hardLine())
		}
		parts = append(parts, e.triviaDoc(t))
		first = false
	}
	if len(parts) == 0 {
		return nilDoc()
	}
	return concat(append([]Doc{hardLine()}, parts...)...)
}

// triviaDoc renders one Trivia piece. BlankLine returns nilDoc — callers
// emit the actual hardLine separator.
func (e *emitter) triviaDoc(t Trivia) Doc {
	switch t.Kind {
	case LineComment:
		return text(normalizeLineComment(t.Text))
	case LongComment:
		return text(t.Text)
	default:
		return nilDoc()
	}
}

// block emits a nested block (function body, if-arm, loop body). The body
// is preceded by a hardLine and indented by one level so the caller can
// write `text("do"), e.block(body, opts), hardLine(), text("end")` and get
// the conventional layout.
func (e *emitter) block(b *ast.Block, opts Options) Doc {
	content := e.blockContent(b, opts)
	if _, ok := content.(docNil); ok {
		return nilDoc()
	}
	return nest(opts.indent(), concat(hardLine(), content))
}

// blockContent emits a block's lines without any surrounding indent or
// leading newline. Used for the chunk-level block (the outermost program)
// where indenting would be wrong, and as a helper from block().
//
// Each statement and each preserved comment becomes one "logical line".
// A run of one-or-more source blank lines collapses to a single empty
// logical line. The lines are joined with hardLine separators — a blank
// logical line therefore renders as a true blank line in the output.
func (e *emitter) blockContent(b *ast.Block, opts Options) Doc {
	if b == nil || b.IsEmpty() {
		return nilDoc()
	}
	var lines []Doc

	appendTriviaUntil := func(upto int) {
		prevBlank := false
		for e.ti < len(e.trivia) && e.trivia[e.ti].Line < upto {
			t := e.trivia[e.ti]
			e.ti++
			if t.Kind == BlankLine {
				// Drop blank lines at the very start of a block — those
				// were just visual padding under the opening keyword and
				// the formatter applies its own spacing.
				if prevBlank || len(lines) == 0 {
					continue
				}
				lines = append(lines, nilDoc())
				prevBlank = true
				continue
			}
			lines = append(lines, e.triviaDoc(t))
			prevBlank = false
		}
	}

	for _, stmt := range b.Statements {
		appendTriviaUntil(stmt.Line())
		lines = append(lines, e.statement(stmt, opts))
	}
	if b.Return != nil {
		appendTriviaUntil(b.Return.Line())
		lines = append(lines, e.returnStmt(b.Return, opts))
	}

	parts := make([]Doc, 0, len(lines)*2)
	for i, ln := range lines {
		if i > 0 {
			parts = append(parts, hardLine())
		}
		parts = append(parts, ln)
	}
	return concat(parts...)
}

// statement dispatches on AST type. Every case returns a Doc that fits on
// one line in flat mode (no embedded hardLine at the top level) so that
// surrounding contexts can group it.
func (e *emitter) statement(stmt ast.Statement, opts Options) Doc {
	switch s := stmt.(type) {
	case *ast.LocalStatement:
		return e.localStmt(s, opts)
	case *ast.LocalFunctionStatement:
		return e.localFuncStmt(s, opts)
	case *ast.FunctionDeclaration:
		return e.funcDecl(s, opts)
	case *ast.AssignStatement:
		return e.assignStmt(s, opts)
	case *ast.IfStatement:
		return e.ifStmt(s, opts)
	case *ast.WhileStatement:
		return e.whileStmt(s, opts)
	case *ast.RepeatStatement:
		return e.repeatStmt(s, opts)
	case *ast.NumericForStatement:
		return e.numericFor(s, opts)
	case *ast.GenericForStatement:
		return e.genericFor(s, opts)
	case *ast.DoStatement:
		return e.doStmt(s, opts)
	case *ast.BreakStatement:
		return text("break")
	case *ast.GotoStatement:
		return concat(text("goto "), text(s.Label))
	case *ast.LabelStatement:
		return concat(text("::"), text(s.Name), text("::"))
	case *ast.ExpressionStatement:
		if s.Expression == nil {
			return nilDoc()
		}
		return e.expr(s.Expression, opts)
	case *ast.TypeAliasStatement:
		return e.typeAlias(s, opts)
	case *ast.EnumStatement:
		return e.enumStmt(s, opts)
	}
	// Unknown statement: fall back to its own String() rendering. The Doc
	// renderer will print it verbatim. This is a safety net; every AST node
	// known at v1 has a case above.
	return text(stmt.String())
}

func (e *emitter) localStmt(s *ast.LocalStatement, opts Options) Doc {
	var nameParts []Doc
	for _, n := range s.Names {
		p := text(n.Name)
		if n.Type != nil {
			p = concat(p, text(": "), e.typeNode(n.Type, opts))
		}
		if n.Attrib != "" {
			p = concat(p, text(" <"), text(n.Attrib), text(">"))
		}
		nameParts = append(nameParts, p)
	}
	head := concat(text("local "), join(text(", "), nameParts...))
	if len(s.Values) == 0 {
		return head
	}
	return e.assignTail(head, s.Values, opts)
}

func (e *emitter) localFuncStmt(s *ast.LocalFunctionStatement, opts Options) Doc {
	return concat(
		text("local function "),
		text(s.Name),
		e.funcSig(s.Func, opts),
		e.funcBody(s.Func, opts),
	)
}

func (e *emitter) funcDecl(s *ast.FunctionDeclaration, opts Options) Doc {
	var head strings.Builder
	head.WriteString("function ")
	head.WriteString(s.Name.Name)
	for _, f := range s.DottedFields {
		head.WriteByte('.')
		head.WriteString(f)
	}
	if s.MethodName != "" {
		head.WriteByte(':')
		head.WriteString(s.MethodName)
	}
	return concat(
		text(head.String()),
		e.funcSig(s.Func, opts),
		e.funcBody(s.Func, opts),
	)
}

// funcSig is "(params): returns" — the part of a function from `(` to the
// end of the return-type annotation. The leading `function` / `function
// name` is the caller's responsibility (because anonymous and declaration
// forms differ).
func (e *emitter) funcSig(fe *ast.FunctionExpression, opts Options) Doc {
	var ps []Doc
	for _, p := range fe.Params {
		if p.Type != nil {
			ps = append(ps, concat(text(p.Name.Name), text(": "), e.typeNode(p.Type, opts)))
		} else {
			ps = append(ps, text(p.Name.Name))
		}
	}
	if fe.IsVararg {
		if fe.VarargType != nil {
			ps = append(ps, concat(text("...: "), e.typeNode(fe.VarargType, opts)))
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
	if len(fe.ReturnTypes) == 0 {
		return params
	}
	var ret Doc
	if len(fe.ReturnTypes) == 1 {
		ret = e.typeNode(fe.ReturnTypes[0], opts)
	} else {
		rs := make([]Doc, len(fe.ReturnTypes))
		for i, r := range fe.ReturnTypes {
			rs[i] = e.typeNode(r, opts)
		}
		ret = concat(text("("), join(text(", "), rs...), text(")"))
	}
	return concat(params, text(": "), ret)
}

// funcBody is "<body> end" with the body inset by one indent level.
func (e *emitter) funcBody(fe *ast.FunctionExpression, opts Options) Doc {
	return concat(e.block(fe.Body, opts), hardLine(), text("end"))
}

func (e *emitter) assignStmt(s *ast.AssignStatement, opts Options) Doc {
	targets := make([]Doc, len(s.Targets))
	for i, t := range s.Targets {
		targets[i] = e.expr(t, opts)
	}
	head := join(text(", "), targets...)
	return e.assignTail(head, s.Values, opts)
}

// assignTail emits ` = <values>` where values may break onto multiple
// lines. The head doc is the LHS plus any name/attribute decorations.
func (e *emitter) assignTail(head Doc, values []ast.Expression, opts Options) Doc {
	vs := make([]Doc, len(values))
	for i, v := range values {
		vs[i] = e.expr(v, opts)
	}
	rhs := group(nest(opts.indent(), concat(line(), join(concat(text(","), line()), vs...))))
	return group(concat(head, text(" ="), rhs))
}

func (e *emitter) ifStmt(s *ast.IfStatement, opts Options) Doc {
	var parts []Doc
	for i, c := range s.Clauses {
		kw := "if "
		if i > 0 {
			kw = "elseif "
		}
		parts = append(parts,
			text(kw), e.expr(c.Condition, opts), text(" then"),
			e.block(c.Body, opts),
			hardLine(),
		)
	}
	if s.Else != nil {
		parts = append(parts, text("else"), e.block(s.Else, opts), hardLine())
	}
	parts = append(parts, text("end"))
	return concat(parts...)
}

func (e *emitter) whileStmt(s *ast.WhileStatement, opts Options) Doc {
	return concat(
		text("while "), e.expr(s.Condition, opts), text(" do"),
		e.block(s.Body, opts),
		hardLine(), text("end"),
	)
}

func (e *emitter) repeatStmt(s *ast.RepeatStatement, opts Options) Doc {
	return concat(
		text("repeat"),
		e.block(s.Body, opts),
		hardLine(),
		text("until "), e.expr(s.Condition, opts),
	)
}

func (e *emitter) numericFor(s *ast.NumericForStatement, opts Options) Doc {
	head := concat(text("for "), text(s.Name), text(" = "),
		e.expr(s.Start, opts), text(", "),
		e.expr(s.Limit, opts),
	)
	if s.Step != nil {
		head = concat(head, text(", "), e.expr(s.Step, opts))
	}
	return concat(
		head, text(" do"),
		e.block(s.Body, opts),
		hardLine(), text("end"),
	)
}

func (e *emitter) genericFor(s *ast.GenericForStatement, opts Options) Doc {
	exprs := make([]Doc, len(s.Exprs))
	for i, x := range s.Exprs {
		exprs[i] = e.expr(x, opts)
	}
	return concat(
		text("for "), text(strings.Join(s.Names, ", ")),
		text(" in "), join(text(", "), exprs...),
		text(" do"),
		e.block(s.Body, opts),
		hardLine(), text("end"),
	)
}

func (e *emitter) doStmt(s *ast.DoStatement, opts Options) Doc {
	return concat(
		text("do"),
		e.block(s.Body, opts),
		hardLine(), text("end"),
	)
}

func (e *emitter) returnStmt(s *ast.ReturnStatement, opts Options) Doc {
	if len(s.Values) == 0 {
		return text("return")
	}
	vs := make([]Doc, len(s.Values))
	for i, v := range s.Values {
		vs[i] = e.expr(v, opts)
	}
	return concat(text("return "), join(text(", "), vs...))
}

func (e *emitter) typeAlias(s *ast.TypeAliasStatement, opts Options) Doc {
	return concat(text("type "), text(s.Name), text(" = "), e.typeNode(s.Target, opts))
}

// enumStmt renders
//
//	enum Name
//	    V1,
//	    V2,
//	    ...
//	end
//
// Variants are always laid out one-per-line with a trailing comma. The
// hand-written single-line form `enum Color RED, GREEN, BLUE end` is
// supported by the parser but we always normalize to the block form on
// output — it scales better as enums grow and keeps diffs minimal when
// variants are added or removed.
func (e *emitter) enumStmt(s *ast.EnumStatement, opts Options) Doc {
	if s.Name == nil {
		return text(s.String())
	}
	if len(s.Variants) == 0 {
		// Parser usually rejects this, but if we receive a partial AST
		// (e.g. mid-edit through an IDE integration) we still emit
		// something syntactically reasonable.
		return concat(text("enum "), text(s.Name.Name), hardLine(), text("end"))
	}
	var lines []Doc
	for _, v := range s.Variants {
		lines = append(lines, concat(text(v.Name), text(",")))
	}
	body := nest(opts.indent(), concat(hardLine(), join(hardLine(), lines...)))
	return concat(text("enum "), text(s.Name.Name), body, hardLine(), text("end"))
}

// --- expressions -------------------------------------------------------

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
	}
	return text(x.String())
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
func (e *emitter) binary(b *ast.BinaryExpression, opts Options) Doc {
	return group(concat(
		e.expr(b.Left, opts),
		nest(opts.indent(), concat(line(), text(b.Op+" "), e.expr(b.Right, opts))),
	))
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

// --- type nodes --------------------------------------------------------

func (e *emitter) typeNode(t ast.TypeNode, opts Options) Doc {
	switch v := t.(type) {
	case *ast.TypePrimitive:
		return text(v.Name)
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
