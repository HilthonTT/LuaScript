package analyze

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type lintPass struct{}

func (lintPass) Name() string {
	return "lint"
}

type localDecl struct {
	name string
	line int
	used bool
}

type scope struct {
	parent *scope
	locals map[string]*localDecl
}

func (lintPass) Run(prog *ast.Program, _ Options, rep *Report) {
	p := lintPass{}
	p.lintBlockWith(prog.Block, nil, nil, rep)
}

func (p lintPass) lintBlockWith(b *ast.Block, parent *scope, predefs []string, rep *Report) {
	if b == nil {
		return
	}
	sc := &scope{parent: parent, locals: map[string]*localDecl{}}
	for _, name := range predefs {
		sc.locals[name] = &localDecl{name: name, used: true}
	}
	p.lintStmts(b.Statements, sc, rep)
	if b.Return != nil {
		p.lintStmt(b.Return, sc, rep)
	}
	p.reportUnused(sc, rep)
}

func (p lintPass) lintBlock(b *ast.Block, parent *scope, rep *Report) {
	p.lintBlockWith(b, parent, nil, rep)
}

func (p lintPass) reportUnused(sc *scope, rep *Report) {
	for _, d := range sc.locals {
		if !d.used && !strings.HasPrefix(d.name, "_") {
			rep.add(Finding{
				Pass:     "lint",
				Rule:     "unused-local",
				Severity: SeverityWarning,
				Line:     d.line,
				Message:  fmt.Sprintf("local '%s' is never used", d.name),
			})
		}
	}
}

func (p lintPass) lintStmts(stmts []ast.Statement, sc *scope, rep *Report) {
	afterJump := false
	deadReported := false
	for _, s := range stmts {
		if afterJump && !deadReported {
			rep.add(Finding{
				Pass:     "lint",
				Rule:     "unreachable-code",
				Severity: SeverityWarning,
				Line:     s.Line(),
				Message:  "unreachable code after break/continue/goto",
			})
			deadReported = true
		}
		p.lintStmt(s, sc, rep)
		switch s.(type) {
		case *ast.BreakStatement, *ast.ContinueStatement, *ast.GotoStatement:
			afterJump = true
		}
	}
}

func (p lintPass) lintStmt(s ast.Statement, sc *scope, rep *Report) {
	switch n := s.(type) {
	case *ast.LocalStatement:
		for _, v := range n.Values {
			p.lintExpr(v, sc, rep)
		}
		for _, ln := range n.Names {
			p.defineLocal(sc, ln.Name, n.Line(), rep)
		}
	case *ast.LocalFunctionStatement:
		p.defineLocal(sc, n.Name, n.Line(), rep)
		p.lintFunc(n.Func, sc, rep)
	case *ast.FunctionDeclaration:
		if len(n.DottedFields) > 0 || n.MethodName != "" {
			markName(n.Name.Name, sc)
		}
		p.lintFunc(n.Func, sc, rep)
	case *ast.AssignStatement:
		for _, t := range n.Targets {
			if _, ok := t.(*ast.Identifier); ok {
				continue
			}
			p.lintExpr(t, sc, rep)
		}
		for _, v := range n.Values {
			p.lintExpr(v, sc, rep)
		}
	case *ast.IfStatement:
		for _, c := range n.Clauses {
			p.lintExpr(c.Condition, sc, rep)
			p.lintBlock(c.Body, sc, rep)
		}
		p.lintBlock(n.Else, sc, rep)
	case *ast.WhileStatement:
		p.lintExpr(n.Condition, sc, rep)
		p.lintBlock(n.Body, sc, rep)
	case *ast.RepeatStatement:
		rsc := &scope{parent: sc, locals: map[string]*localDecl{}}
		if n.Body != nil {
			p.lintStmts(n.Body.Statements, rsc, rep)
			if n.Body.Return != nil {
				p.lintStmt(n.Body.Return, rsc, rep)
			}
		}
		p.lintExpr(n.Condition, rsc, rep)
		p.reportUnused(rsc, rep)
	case *ast.NumericForStatement:
		p.lintExpr(n.Start, sc, rep)
		p.lintExpr(n.Limit, sc, rep)
		if n.Step != nil {
			p.lintExpr(n.Step, sc, rep)
		}
		p.lintBlockWith(n.Body, sc, []string{n.Name}, rep)
	case *ast.GenericForStatement:
		for _, e := range n.Exprs {
			p.lintExpr(e, sc, rep)
		}
		p.lintBlockWith(n.Body, sc, n.Names, rep)
	case *ast.DoStatement:
		p.lintBlock(n.Body, sc, rep)
	case *ast.ReturnStatement:
		for _, v := range n.Values {
			p.lintExpr(v, sc, rep)
		}
	case *ast.ExpressionStatement:
		p.lintExpr(n.Expression, sc, rep)
	}
}

func (p lintPass) lintFunc(fe *ast.FunctionExpression, parent *scope, rep *Report) {
	if fe == nil {
		return
	}
	params := make([]string, 0, len(fe.Params))
	for _, pr := range fe.Params {
		if pr.Default != nil {
			p.lintExpr(pr.Default, parent, rep)
		}
		params = append(params, pr.Name.Name)
	}
	p.lintBlockWith(fe.Body, parent, params, rep)
}

func (p lintPass) lintExpr(e ast.Expression, sc *scope, rep *Report) {
	if e == nil {
		return
	}
	w := &walker{stopAtFunc: true}
	w.onExpr = func(x ast.Expression) {
		switch n := x.(type) {
		case *ast.Identifier:
			markName(n.Name, sc)
		case *ast.FunctionExpression:
			p.lintFunc(n, sc, rep)
		}
	}
	w.walkExpr(e)
}

func (p lintPass) defineLocal(sc *scope, name string, line int, rep *Report) {
	if !strings.HasPrefix(name, "_") {
		for s := sc.parent; s != nil; s = s.parent {
			if _, ok := s.locals[name]; ok {
				rep.add(Finding{
					Pass:     "lint",
					Rule:     "shadowing",
					Severity: SeverityInfo,
					Line:     line,
					Message:  fmt.Sprintf("local '%s' shadows an outer declaration", name),
				})
				break
			}
		}
	}
	sc.locals[name] = &localDecl{name: name, line: line}
}

func markName(name string, sc *scope) {
	for s := sc; s != nil; s = s.parent {
		if d, ok := s.locals[name]; ok {
			d.used = true
			return
		}
	}
}
