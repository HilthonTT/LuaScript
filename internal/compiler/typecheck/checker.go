package typecheck

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type Options struct {
	Strict bool
}

func Check(prog *ast.Program, opts Options) []TypeError {
	c := &checker{
		env:          newEnv(),
		opts:         opts,
		taggedEnums:  map[string][]string{},
		classicEnums: map[string][]string{},
	}
	c.installGlobals()
	if prog == nil || prog.Block == nil {
		return nil
	}
	c.preResolveAliases(prog.Block.Statements)
	c.scanMutations(prog.Block)
	c.walkBlock(prog.Block)
	sortByLine(c.errors)
	return c.errors
}

type checker struct {
	env  *env
	opts Options

	errors []TypeError

	returnsStack [][]*Type

	instDepth int

	builtins map[string]*Type

	taggedEnums  map[string][]string
	classicEnums map[string][]string

	assignedSomewhere map[string]bool
	upvalMutated      map[string]bool
}

func (c *checker) builtinInScope(name string) bool {
	t, ok := c.env.lookup(name)
	return ok && t == c.builtins[name]
}

func (c *checker) invalidateCallRefinements() {
	if len(c.upvalMutated) == 0 {
		return
	}
	for name := range c.upvalMutated {
		if !c.env.visiblyRefined(name) {
			continue
		}
		if declared, ok := c.env.lookupDeclared(name); ok {
			c.env.widenRefined(name, declared)
		}
	}
}

func (c *checker) widenLoopAssigned(b *ast.Block) {
	if b == nil {
		return
	}
	assigned := map[string]bool{}
	upval := map[string]bool{}
	scanBlockMutations(b, assigned, upval, false, nil)
	for name := range assigned {
		if !c.env.visiblyRefined(name) {
			continue
		}
		if declared, ok := c.env.lookupDeclared(name); ok {
			c.env.widenRefined(name, declared)
		}
	}
}

func (c *checker) expandRHS(values []ast.Expression, n int) []*Type {
	out := make([]*Type, n)
	for i := range n {
		out[i] = nilT
	}
	if len(values) == 0 {
		return out
	}
	m := len(values)
	for i := 0; i < m-1 && i < n; i++ {
		out[i] = c.typeOfExpression(values[i])
	}
	last := values[m-1]
	if m-1 < n {
		out[m-1] = c.typeOfExpression(last)
	} else {
		c.walkExpressionDiscard(last)
	}
	switch last.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression, *ast.VarargExpression:
		for i := m; i < n; i++ {
			out[i] = anyT
		}
	}
	return out
}

func (c *checker) installGlobals() {
	g := stdlibGlobals()
	for name, t := range g {
		c.env.define(name, t)
	}
	c.builtins = map[string]*Type{
		"assert": g["assert"],
		"error":  g["error"],
		"type":   g["type"],
		"typeof": g["typeof"],
	}
}
