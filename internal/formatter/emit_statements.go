package formatter

// Doc emission for statements.

import (
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

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
	case *ast.MatchStatement:
		return e.matchStmt(s, opts)
	case *ast.ReturnStatement:
		// Normally a return lives in Block.Return and is emitted by block(),
		// but a match arm body is a single statement and may be a bare
		// `return`. Without this case it would fall through to stmt.String(),
		// whose fully-parenthesized rendering (`"a" .. b` -> `("a" .. b)`)
		// grows another paren layer on every format pass.
		return e.returnStmt(s, opts)
	case *ast.TryCatchStatement:
		return e.tryCatchStmt(s, opts)
	case *ast.ThrowStatement:
		return concat(text("throw "), e.expr(s.Value, opts))
	case *ast.BreakStatement:
		return text("break")
	case *ast.ContinueStatement:
		return text("continue")
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
	// `<T, U>` sits between the name and `(` in every function form, including
	// the anonymous one (parseFunctionBody reads it before the parameter list).
	tp := typeParamList(fe.TypeParams)
	var ps []Doc
	for _, p := range fe.Params {
		d := text(p.Name.Name)
		if p.Type != nil {
			d = concat(d, text(": "), e.typeNode(p.Type, opts))
		}
		if p.Default != nil {
			d = concat(d, text(" = "), e.expr(p.Default, opts))
		}
		ps = append(ps, d)
	}
	if fe.IsVararg {
		if fe.VarargType != nil {
			ps = append(ps, concat(text("...: "), e.typeNode(fe.VarargType, opts)))
		} else {
			ps = append(ps, text("..."))
		}
	}
	params := group(concat(
		tp,
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

func (e *emitter) tryCatchStmt(s *ast.TryCatchStatement, opts Options) Doc {
	catch := text("catch do")
	if s.CatchVar != nil {
		catch = concat(text("catch "), text(s.CatchVar.Name), text(" do"))
	}
	return concat(
		text("try"),
		e.block(s.Try, opts),
		hardLine(), catch,
		e.block(s.Catch, opts),
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
	return concat(text("type "), text(s.Name), typeParamList(s.TypeParams), text(" = "), e.typeNode(s.Target, opts))
}

// typeParamList renders a generic parameter list `<T, U>`, or nothing when the
// declaration has none. Every declaration form that accepts type parameters
// must route through this — omitting them turns a generic declaration into a
// non-generic one whose body references undefined type names.
func typeParamList(params []string) Doc {
	if len(params) == 0 {
		return nilDoc()
	}
	return text("<" + strings.Join(params, ", ") + ">")
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
		d := text(v.Name)
		// A tagged variant carries positional payload types — `Circle(number)`.
		// Dropping them silently demotes the enum to the integer-constant form
		// and breaks every constructor call.
		if len(v.Payload) > 0 {
			ps := make([]Doc, len(v.Payload))
			for i, p := range v.Payload {
				ps[i] = e.typeNode(p, opts)
			}
			d = concat(d, text("("), join(text(", "), ps...), text(")"))
		}
		lines = append(lines, concat(d, text(",")))
	}
	body := nest(opts.indent(), concat(hardLine(), join(hardLine(), lines...)))
	return concat(text("enum "), text(s.Name.Name), body, hardLine(), text("end"))
}

// matchStmt renders
//
//	match subject do
//	    <pattern> [if <guard>] -> <stmt>
//	    ...
//	end
//
// One arm per line, like enumStmt's variants. Arms are not a Block, so (also
// like enumStmt) comments written between them are not anchored here and get
// re-emitted by the enclosing block rather than staying put.
func (e *emitter) matchStmt(s *ast.MatchStatement, opts Options) Doc {
	head := concat(text("match "), e.expr(s.Subject, opts), text(" do"))
	if len(s.Arms) == 0 {
		return concat(head, hardLine(), text("end"))
	}
	var lines []Doc
	for i := range s.Arms {
		arm := &s.Arms[i]
		parts := []Doc{e.matchPattern(&arm.Pattern, opts)}
		if arm.Guard != nil {
			parts = append(parts, text(" if "), e.expr(arm.Guard, opts))
		}
		parts = append(parts, text(" -> "), e.statement(arm.Body, opts))
		// Arms are separated by a newline only, so an arm body ending in a call
		// would swallow the next arm's pattern when that pattern starts with a
		// token that continues a prefix expression (`f("x")` newline `"bye"`
		// parses as `f("x")("bye")`). A terminating `;` closes the body.
		if i+1 < len(s.Arms) && patternCanChain(&s.Arms[i+1].Pattern) {
			parts = append(parts, text(";"))
		}
		lines = append(lines, concat(parts...))
	}
	body := nest(opts.indent(), concat(hardLine(), join(hardLine(), lines...)))
	return concat(head, body, hardLine(), text("end"))
}

// patternCanChain reports whether p renders starting with a token that Lua
// would attach to a preceding prefix expression: a string literal or a table
// constructor (call sugar), or a parenthesized expression. Only value patterns
// can; `_`, `x: T`, and `Tag(...)` / `Tag{...}` all start with a name, which
// cannot continue an expression.
func patternCanChain(p *ast.MatchPattern) bool {
	if p.Kind != ast.MatchValue || len(p.Values) == 0 {
		return false
	}
	switch p.Values[0].(type) {
	case *ast.StringLiteral, *ast.TableConstructor, *ast.ParenExpression:
		return true
	}
	return false
}

func (e *emitter) matchPattern(p *ast.MatchPattern, opts Options) Doc {
	switch p.Kind {
	case ast.MatchValue:
		vs := make([]Doc, len(p.Values))
		for i, v := range p.Values {
			vs[i] = e.expr(v, opts)
		}
		return join(text(", "), vs...)
	case ast.MatchWildcard:
		return text("_")
	case ast.MatchTyped:
		name := p.Bind
		if name == "" {
			name = "_"
		}
		return concat(text(name), text(": "), e.typeNode(p.Type, opts))
	case ast.MatchDestructurePos:
		return concat(text(p.Tag), text("("), text(strings.Join(p.PosBinds, ", ")), text(")"))
	case ast.MatchDestructureNamed:
		fs := make([]Doc, len(p.NamedBinds))
		for i, nb := range p.NamedBinds {
			fs[i] = concat(text(nb.Field), text(" = "), text(nb.Bind))
		}
		return concat(text(p.Tag), text("{ "), join(text(", "), fs...), text(" }"))
	}
	return nilDoc()
}
