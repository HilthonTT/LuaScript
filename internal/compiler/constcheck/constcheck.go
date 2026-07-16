// Package constcheck enforces Lua 5.4's local attributes at compile time:
// a local declared `<const>` or `<close>` may never be assigned after its
// declaration. The pass runs unconditionally in the compile pipeline (unlike
// the type checker it is not gated by `--!nocheck`), matching PUC Lua where
// assigning to a const local is a syntax-level error.
//
// Only the *assignment* rule is enforced here. The `<close>` attribute's
// runtime behaviour (calling `__close` at scope exit) is a documented
// language-level non-goal for now; `<close>` locals get the same
// no-reassignment protection as `<const>` and are otherwise inert.
package constcheck

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// Check walks prog and returns an error describing every assignment to a
// `<const>`/`<close>` local, or nil when the program is clean. Inner
// functions count: assigning to a captured const upvalue is also an error.
func Check(prog *ast.Program) error {
	if prog == nil || prog.Block == nil {
		return nil
	}
	c := &checker{}
	c.pushScope()
	c.block(prog.Block)
	c.popScope()
	if len(c.errors) == 0 {
		return nil
	}
	return &Errors{Messages: c.errors}
}

// Errors aggregates every attribute violation found in one Check pass.
type Errors struct {
	Messages []string
}

func (e *Errors) Error() string {
	return strings.Join(e.Messages, "\n")
}

// checker tracks lexical scopes. Every local is recorded (attributed or
// not) so an inner plain `local x` correctly shadows an outer `local x
// <const>`. The scope stack deliberately spans function boundaries: a name
// resolved in an enclosing function is an upvalue, and const upvalues are
// just as unassignable as const locals.
type checker struct {
	scopes []map[string]string // name → attrib ("", "const", or "close")
	errors []string
}

func (c *checker) pushScope() {
	c.scopes = append(c.scopes, map[string]string{})
}

func (c *checker) popScope() {
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *checker) define(name, attrib string) {
	c.scopes[len(c.scopes)-1][name] = attrib
}

// attribOf resolves name through the scope stack, innermost first. The
// boolean is false when the name is not a known local (i.e. a global, which
// is always assignable).
func (c *checker) attribOf(name string) (string, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if a, ok := c.scopes[i][name]; ok {
			return a, true
		}
	}
	return "", false
}

func (c *checker) checkTarget(name string, line int) {
	if a, ok := c.attribOf(name); ok && a != "" {
		c.errors = append(c.errors, fmt.Sprintf(
			"cannot assign to %s variable '%s' at line %d.", a, name, line))
	}
}

func (c *checker) block(b *ast.Block) {
	if b == nil {
		return
	}
	c.pushScope()
	for _, s := range b.Statements {
		c.stmt(s)
	}
	if b.Return != nil {
		c.stmt(b.Return)
	}
	c.popScope()
}

func (c *checker) stmt(s ast.Statement) {
	switch n := s.(type) {
	case *ast.LocalStatement:
		// Initializers are evaluated before the new names enter scope.
		c.exprs(n.Values)
		for _, ln := range n.Names {
			c.define(ln.Name, ln.Attrib)
		}
	case *ast.LocalFunctionStatement:
		c.define(n.Name, "")
		c.function(n.Func)
	case *ast.FunctionDeclaration:
		// Plain `function name() end` assigns to `name`; the dotted/method
		// forms only write a table field, which attributes don't guard.
		if len(n.DottedFields) == 0 && n.MethodName == "" {
			c.checkTarget(n.Name.Name, n.Line())
		}
		c.function(n.Func)
	case *ast.AssignStatement:
		c.exprs(n.Values)
		for _, t := range n.Targets {
			switch tgt := t.(type) {
			case *ast.Identifier:
				c.checkTarget(tgt.Name, n.Line())
			case *ast.IndexExpression:
				// Writing a field of a const table is legal (the binding is
				// const, not the value) — but the sub-expressions may contain
				// function literals worth walking.
				c.expr(tgt.Object)
				c.expr(tgt.Index)
			}
		}
	case *ast.IfStatement:
		for _, cl := range n.Clauses {
			c.expr(cl.Condition)
			c.block(cl.Body)
		}
		c.block(n.Else)
	case *ast.WhileStatement:
		c.expr(n.Condition)
		c.block(n.Body)
	case *ast.RepeatStatement:
		// The `until` condition sees the body's locals.
		c.pushScope()
		if n.Body != nil {
			for _, st := range n.Body.Statements {
				c.stmt(st)
			}
			if n.Body.Return != nil {
				c.stmt(n.Body.Return)
			}
		}
		c.expr(n.Condition)
		c.popScope()
	case *ast.NumericForStatement:
		c.expr(n.Start)
		c.expr(n.Limit)
		c.expr(n.Step)
		c.pushScope()
		c.define(n.Name, "")
		c.block(n.Body)
		c.popScope()
	case *ast.GenericForStatement:
		c.exprs(n.Exprs)
		c.pushScope()
		for _, name := range n.Names {
			c.define(name, "")
		}
		c.block(n.Body)
		c.popScope()
	case *ast.DoStatement:
		c.block(n.Body)
	case *ast.ReturnStatement:
		c.exprs(n.Values)
	case *ast.ExpressionStatement:
		c.expr(n.Expression)
	case *ast.DeferStatement:
		c.expr(n.Call)
	case *ast.TryCatchStatement:
		c.block(n.Try)
		// The catch binding is an ordinary assignable local scoped to the
		// handler, so it needs a scope of its own around the handler's block.
		c.pushScope()
		if n.CatchVar != nil {
			c.define(n.CatchVar.Name, "")
		}
		c.block(n.Catch)
		c.popScope()
	case *ast.ThrowStatement:
		c.expr(n.Value)
	case *ast.EnumStatement:
		if n.Name != nil {
			c.define(n.Name.Name, "")
		}
	case *ast.StructStatement:
		if n.Name != nil {
			c.define(n.Name.Name, "")
		}
	}
	// Break/Continue/Goto/Label/TypeAlias: no bindings, no assignments.
}

// function walks a function literal: parameters are fresh (assignable)
// locals in a new scope; defaults are evaluated in that scope too.
func (c *checker) function(fe *ast.FunctionExpression) {
	if fe == nil {
		return
	}
	c.pushScope()
	for _, p := range fe.Params {
		c.define(p.Name.Name, "")
		c.expr(p.Default)
	}
	c.block(fe.Body)
	c.popScope()
}

func (c *checker) exprs(es []ast.Expression) {
	for _, e := range es {
		c.expr(e)
	}
}

// expr descends into sub-expressions looking for nested function literals
// (whose bodies may assign to captured const locals).
func (c *checker) expr(e ast.Expression) {
	switch n := e.(type) {
	case *ast.BinaryExpression:
		c.expr(n.Left)
		c.expr(n.Right)
	case *ast.UnaryExpression:
		c.expr(n.Operand)
	case *ast.ParenExpression:
		c.expr(n.Inner)
	case *ast.CallExpression:
		c.expr(n.Func)
		c.exprs(n.Args)
	case *ast.MethodCallExpression:
		c.expr(n.Object)
		c.exprs(n.Args)
	case *ast.IndexExpression:
		c.expr(n.Object)
		c.expr(n.Index)
	case *ast.TableConstructor:
		for _, f := range n.Fields {
			c.expr(f.Key)
			c.expr(f.Value)
		}
	case *ast.TypeAssertionExpression:
		c.expr(n.Expr)
	case *ast.IfExpression:
		for _, cl := range n.Clauses {
			c.expr(cl.Condition)
			c.expr(cl.Value)
		}
		c.expr(n.Else)
	case *ast.FunctionExpression:
		c.function(n)
	}
	// Literals, Identifier, Vararg: leaves. Reading a const is always fine.
}
