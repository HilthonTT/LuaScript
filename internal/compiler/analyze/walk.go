package analyze

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// walker is a generic recursive AST traversal. The.lsc AST has no shared
// visitor, so the analyzer provides its own. onStmt / onExpr (either may be
// nil) are invoked on every statement / expression node, parents before
// children. When stopAtFunc is set, the walk does not descend into nested
// FunctionExpression bodies — used by the complexity pass, which scores each
// function independently.
type walker struct {
	onStmt     func(ast.Statement)
	onExpr     func(ast.Expression)
	stopAtFunc bool
}

func (w *walker) walkBlock(b *ast.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Statements {
		w.walkStmt(s)
	}
	if b.Return != nil {
		w.walkStmt(b.Return)
	}
}

func (w *walker) walkStmt(s ast.Statement) {
	if s == nil {
		return
	}
	if w.onStmt != nil {
		w.onStmt(s)
	}
	switch n := s.(type) {
	case *ast.AssignStatement:
		w.walkExprs(n.Targets)
		w.walkExprs(n.Values)
	case *ast.LocalStatement:
		w.walkExprs(n.Values)
	case *ast.LocalFunctionStatement:
		w.walkExpr(n.Func)
	case *ast.FunctionDeclaration:
		w.walkExpr(n.Func)
	case *ast.IfStatement:
		for _, c := range n.Clauses {
			w.walkExpr(c.Condition)
			w.walkBlock(c.Body)
		}
		w.walkBlock(n.Else)
	case *ast.WhileStatement:
		w.walkExpr(n.Condition)
		w.walkBlock(n.Body)
	case *ast.RepeatStatement:
		w.walkBlock(n.Body)
		w.walkExpr(n.Condition)
	case *ast.NumericForStatement:
		w.walkExpr(n.Start)
		w.walkExpr(n.Limit)
		w.walkExpr(n.Step)
		w.walkBlock(n.Body)
	case *ast.GenericForStatement:
		w.walkExprs(n.Exprs)
		w.walkBlock(n.Body)
	case *ast.DoStatement:
		w.walkBlock(n.Body)
	case *ast.ReturnStatement:
		w.walkExprs(n.Values)
	case *ast.ExpressionStatement:
		w.walkExpr(n.Expression)
	}
	// BreakStatement, GotoStatement, LabelStatement, TypeAliasStatement,
	// *ast.Block carry no further nodes the analyzer inspects.
}

func (w *walker) walkExprs(es []ast.Expression) {
	for _, e := range es {
		w.walkExpr(e)
	}
}

func (w *walker) walkExpr(e ast.Expression) {
	if e == nil {
		return
	}
	if w.onExpr != nil {
		w.onExpr(e)
	}
	switch n := e.(type) {
	case *ast.BinaryExpression:
		w.walkExpr(n.Left)
		w.walkExpr(n.Right)
	case *ast.UnaryExpression:
		w.walkExpr(n.Operand)
	case *ast.ParenExpression:
		w.walkExpr(n.Inner)
	case *ast.CallExpression:
		w.walkExpr(n.Func)
		w.walkExprs(n.Args)
	case *ast.MethodCallExpression:
		w.walkExpr(n.Object)
		w.walkExprs(n.Args)
	case *ast.IndexExpression:
		w.walkExpr(n.Object)
		w.walkExpr(n.Index)
	case *ast.TableConstructor:
		for _, f := range n.Fields {
			if f.Key != nil {
				w.walkExpr(f.Key)
			}
			w.walkExpr(f.Value)
		}
	case *ast.TypeAssertionExpression:
		w.walkExpr(n.Expr)
	case *ast.IfExpression:
		for _, c := range n.Clauses {
			w.walkExpr(c.Condition)
			w.walkExpr(c.Value)
		}
		w.walkExpr(n.Else)
	case *ast.FunctionExpression:
		for _, p := range n.Params {
			if p.Default != nil {
				w.walkExpr(p.Default)
			}
		}
		if !w.stopAtFunc {
			w.walkBlock(n.Body)
		}
	}
	// Literals, Identifier, VarargExpression: leaf nodes.
}

// fnInfo names a function unit for the complexity pass.
type fnInfo struct {
	name string
	line int
	body *ast.Block
}

// eachFunction returns the main chunk followed by every FunctionExpression in
// the program, in source order. Names are taken from the enclosing
// declaration when there is one; anonymous functions get "function@<line>".
func eachFunction(prog *ast.Program) []fnInfo {
	fns := []fnInfo{{name: "main chunk", line: 1, body: prog.Block}}
	named := map[*ast.FunctionExpression]string{}

	w := &walker{}
	w.onStmt = func(s ast.Statement) {
		switch n := s.(type) {
		case *ast.FunctionDeclaration:
			if n.Func != nil {
				named[n.Func] = funcDeclName(n)
			}
		case *ast.LocalFunctionStatement:
			if n.Func != nil {
				named[n.Func] = n.Name
			}
		}
	}
	w.onExpr = func(e ast.Expression) {
		if fe, ok := e.(*ast.FunctionExpression); ok {
			name := named[fe]
			if name == "" {
				name = fmt.Sprintf("function@%d", fe.Line())
			}
			fns = append(fns, fnInfo{name: name, line: fe.Line(), body: fe.Body})
		}
	}
	w.walkBlock(prog.Block)
	return fns
}

func funcDeclName(fd *ast.FunctionDeclaration) string {
	var name strings.Builder
	name.WriteString(fd.Name.Name)
	for _, f := range fd.DottedFields {
		name.WriteString(".")
		name.WriteString(f)
	}
	if fd.MethodName != "" {
		name.WriteString(":")
		name.WriteString(fd.MethodName)
	}
	return name.String()
}
