package analyze

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type complexityPass struct{}

func (complexityPass) Name() string {
	return "complexity"
}

func (complexityPass) Run(prog *ast.Program, opts Options, rep *Report) {
	fns := eachFunction(prog)
	rep.Metrics.Functions = len(fns) - 1

	for _, fn := range fns {
		c := complexityOf(fn.body)
		rep.Metrics.TotalComplexity += c
		if c > rep.Metrics.MaxComplexity {
			rep.Metrics.MaxComplexity = c
		}
		if c > opts.MaxComplexity {
			rep.add(Finding{
				Pass:     "complexity",
				Rule:     "high-complexity",
				Severity: SeverityWarning,
				Line:     fn.line,
				Message: fmt.Sprintf("%s has cyclomatic complexity %d (max %d)",
					fn.name, c, opts.MaxComplexity),
			})
		}
	}
}

func complexityOf(body *ast.Block) int {
	c := 1
	w := &walker{stopAtFunc: true}
	w.onStmt = func(s ast.Statement) {
		switch n := s.(type) {
		case *ast.IfStatement:
			c += len(n.Clauses)
		case *ast.WhileStatement, *ast.RepeatStatement,
			*ast.NumericForStatement, *ast.GenericForStatement:
			c++
		}
	}
	w.onExpr = func(e ast.Expression) {
		if be, ok := e.(*ast.BinaryExpression); ok && (be.Op == "and" || be.Op == "or") {
			c++
		}
	}
	w.walkBlock(body)
	return c
}
