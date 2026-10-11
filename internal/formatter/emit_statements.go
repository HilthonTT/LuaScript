package formatter

import (
	"strings"

	"github.com/hilthontt/luascript/v3/internal/compiler/ast"
)

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
	case *ast.LocalDestructureStatement:
		return e.localDestructure(s, opts)
	case *ast.ImplStatement:
		return e.implStmt(s, opts)
	case *ast.StructStatement:
		return e.structStmt(s, opts)
	}
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

func (e *emitter) funcSig(fe *ast.FunctionExpression, opts Options) Doc {
	tp := e.typeParamList(fe.TypeParams, opts)
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
	if s.IsInterface {
		return concat(text("interface "), text(s.Name), e.typeParamList(s.TypeParams, opts),
			text(" "), e.typeNode(s.Target, opts))
	}
	return concat(text("type "), text(s.Name), e.typeParamList(s.TypeParams, opts),
		text(" = "), e.typeNode(s.Target, opts))
}

func (e *emitter) typeParamList(params []ast.TypeParam, opts Options) Doc {
	if len(params) == 0 {
		return nilDoc()
	}
	parts := make([]Doc, len(params))
	for i, p := range params {
		parts[i] = text(p.Name)
		if p.Constraint != nil {
			parts[i] = concat(parts[i], text(": "), e.typeNode(p.Constraint, opts))
		}
	}
	return concat(text("<"), join(text(", "), parts...), text(">"))
}

func (e *emitter) localDestructure(s *ast.LocalDestructureStatement, opts Options) Doc {
	open, closer := "{", "}"
	if s.IsArray {
		open, closer = "[", "]"
	}
	parts := make([]Doc, len(s.Binds))
	for i, b := range s.Binds {
		var d Doc
		switch {
		case b.Rest:
			d = concat(text("..."), text(b.Bind))
		case b.Key != "" && b.Key != b.Bind:
			d = concat(text(b.Key), text(" = "), text(b.Bind))
		default:
			d = text(b.Bind)
		}
		if b.Type != nil {
			d = concat(d, text(": "), e.typeNode(b.Type, opts))
		}
		if b.Default != nil {
			d = concat(d, text(" or "), e.expr(b.Default, opts))
		}
		parts[i] = d
	}
	head := concat(text("local "), text(open), text(" "),
		join(text(", "), parts...), text(" "), text(closer))
	return e.assignTail(head, []ast.Expression{s.Value}, opts)
}

func (e *emitter) implStmt(s *ast.ImplStatement, opts Options) Doc {
	if s.Target == nil {
		return text(s.String())
	}
	var body []Doc
	for i, m := range s.Members {
		if i > 0 {
			body = append(body, hardLine())
		}
		body = append(body,
			hardLine(),
			text("function "), text(m.Name), e.funcSig(m.Func, opts),
			e.funcBody(m.Func, opts))
	}
	return concat(
		text("impl "), text(s.Target.Name),
		nest(opts.indent(), concat(body...)),
		hardLine(), text("end"))
}

func (e *emitter) structStmt(s *ast.StructStatement, opts Options) Doc {
	if s.Name == nil {
		return text(s.String())
	}
	fields := make([]Doc, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = concat(text(f.Name), text(": "), e.typeNode(f.Type, opts))
	}
	return concat(
		text("struct "), text(s.Name.Name), e.typeParamList(s.TypeParams, opts), text(" {"),
		nest(opts.indent(), concat(hardLine(), join(concat(text(","), hardLine()), fields...), text(","))),
		hardLine(), text("}"))
}

func (e *emitter) enumStmt(s *ast.EnumStatement, opts Options) Doc {
	if s.Name == nil {
		return text(s.String())
	}
	if len(s.Variants) == 0 {
		return concat(text("enum "), text(s.Name.Name), hardLine(), text("end"))
	}
	var lines []Doc
	for _, v := range s.Variants {
		d := text(v.Name)
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
		if i+1 < len(s.Arms) && patternCanChain(&s.Arms[i+1].Pattern) {
			parts = append(parts, text(";"))
		}
		lines = append(lines, concat(parts...))
	}
	body := nest(opts.indent(), concat(hardLine(), join(hardLine(), lines...)))
	return concat(head, body, hardLine(), text("end"))
}

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
