package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

// This file holds the generics machinery: scoping type parameters as
// gradual type variables, instantiating generic aliases/structs on
// application (`Box<number>`), and best-effort call-site inference for
// generic functions (`identity(5)` → number).

// pushTypeParams registers each name as a fresh gradual type variable in the
// alias table, shadowing any existing alias, and returns a restore function
// that puts the previous bindings back. Used around a generic function's
// signature resolution and body walk so `T` resolves to a KindTypeParam.
func (c *checker) pushTypeParams(names []string) func() {
	if len(names) == 0 {
		return func() {}
	}
	type prev struct {
		t   *Type
		had bool
	}
	saved := make(map[string]prev, len(names))
	for _, n := range names {
		t, had := c.env.aliases[n]
		saved[n] = prev{t: t, had: had}
		c.env.aliases[n] = &Type{Kind: KindTypeParam, AliasName: n}
	}
	return func() {
		for n, p := range saved {
			if p.had {
				c.env.aliases[n] = p.t
			} else {
				delete(c.env.aliases, n)
			}
		}
	}
}

// resolveTypeApplication resolves a `Name<Args>` type. It instantiates the
// named generic template by binding its parameters to the (resolved)
// arguments and resolving the template body under those bindings. Unknown
// names, or arity mismatches, degrade to `any` with a diagnostic rather than
// crashing — keeping the checker gradual.
// maxInstantiationDepth bounds nested generic expansion. Legitimate nesting
// (`Box<Box<Box<number>>>`) stays tiny; anything deeper is a recursive
// template that would otherwise expand until the Go stack overflows.
const maxInstantiationDepth = 64

func (c *checker) resolveTypeApplication(app *ast.TypeApplication) *Type {
	if c.instDepth >= maxInstantiationDepth {
		c.errf(app.Line(), "recursive-generic",
			"generic type %q expands recursively — self-referential generic types are not supported", app.Name)
		return anyT
	}
	c.instDepth++
	defer func() {
		c.instDepth--
	}()

	g, ok := c.env.generics[app.Name]
	if !ok {
		// Not a known generic. If it names a plain (non-generic) alias the
		// user likely over-applied it; otherwise it's unknown.
		if _, isAlias := c.env.aliases[app.Name]; isAlias {
			c.errf(app.Line(), "not-generic",
				"type %q is not generic but was given type arguments", app.Name)
		} else {
			c.errf(app.Line(), "unknown-type", "unknown type %q", app.Name)
		}
		return anyT
	}
	if len(app.Args) != len(g.params) {
		c.errf(app.Line(), "generic-arity",
			"generic type %q expects %d type argument(s), got %d",
			app.Name, len(g.params), len(app.Args))
		return anyT
	}

	// Resolve each argument, then bind params → args and resolve the body.
	args := make([]*Type, len(app.Args))
	for i, a := range app.Args {
		args[i] = c.resolveAST(a)
	}
	restore := c.bindParamTypes(g.params, args)
	defer restore()

	t := c.resolveAST(g.target)
	// Tag the instantiation with a readable name (`Box<number>`).
	if t != nil && t.AliasName == "" {
		withName := *t
		withName.AliasName = app.String()
		return &withName
	}
	return t
}

// bindParamTypes temporarily binds concrete types to parameter names in the
// alias table (used while resolving a generic template body). Returns a
// restore function.
func (c *checker) bindParamTypes(params []string, args []*Type) func() {
	type prev struct {
		t   *Type
		had bool
	}
	saved := make(map[string]prev, len(params))
	for i, n := range params {
		t, had := c.env.aliases[n]
		saved[n] = prev{t: t, had: had}
		// Give the bound argument a display name so a later resolveAST of the
		// parameter reference (`T`) doesn't re-tag it with the parameter name
		// — the concrete argument's own spelling (`number`, `Point`) is what
		// should appear in diagnostics.
		bound := *args[i]
		if bound.AliasName == "" {
			bound.AliasName = args[i].String()
		}
		c.env.aliases[n] = &bound
	}
	return func() {
		for n, p := range saved {
			if p.had {
				c.env.aliases[n] = p.t
			} else {
				delete(c.env.aliases, n)
			}
		}
	}
}

// instantiateCall performs best-effort inference for a call to a generic
// function. It unifies each declared parameter type against the corresponding
// argument type to bind the function's type variables, then substitutes those
// bindings into the declared return types. Unbound variables fall back to
// `any` (gradual). Returns the substituted return types.
func (c *checker) instantiateCall(fn *FunctionShape, args []*Type) []*Type {
	subst := map[string]*Type{}
	for _, name := range fn.TypeParams {
		subst[name] = nil // unbound
	}
	n := min(len(args), len(fn.Params))
	for i := range n {
		unify(fn.Params[i], args[i], subst)
	}
	// Any variable still unbound becomes `any`.
	for name, t := range subst {
		if t == nil {
			subst[name] = anyT
		}
	}
	out := make([]*Type, len(fn.Returns))
	for i, r := range fn.Returns {
		out[i] = substitute(r, subst)
	}
	return out
}

// unify matches a declared type (which may mention type variables) against a
// concrete argument type, recording variable → concrete bindings in subst.
// It is deliberately shallow and best-effort: it binds a variable the first
// time it sees one and otherwise recurses into matching structure. Conflicts
// are ignored (first binding wins) — inference here informs precision, it
// does not gate correctness (assignability already ran gradually).
func unify(declared, actual *Type, subst map[string]*Type) {
	if declared == nil || actual == nil {
		return
	}
	if declared.Kind == KindTypeParam {
		if _, tracked := subst[declared.AliasName]; tracked {
			if subst[declared.AliasName] == nil {
				subst[declared.AliasName] = actual
			}
		}
		return
	}
	if actual.Kind == KindAny {
		return
	}
	switch declared.Kind {
	case KindFunction:
		if actual.Kind == KindFunction && declared.Fn != nil && actual.Fn != nil {
			m := min(len(declared.Fn.Params), len(actual.Fn.Params))
			for i := range m {
				unify(declared.Fn.Params[i], actual.Fn.Params[i], subst)
			}
			r := min(len(declared.Fn.Returns), len(actual.Fn.Returns))
			for i := range r {
				unify(declared.Fn.Returns[i], actual.Fn.Returns[i], subst)
			}
		}
	case KindTable:
		if actual.Kind == KindTable && declared.Table != nil && actual.Table != nil {
			actualFields := map[string]*Type{}
			for _, f := range actual.Table.Fields {
				actualFields[f.Key] = f.Type
			}
			for _, f := range declared.Table.Fields {
				if af, ok := actualFields[f.Key]; ok {
					unify(f.Type, af, subst)
				}
			}
			if declared.Table.Indexer != nil && actual.Table.Indexer != nil {
				unify(declared.Table.Indexer.Value, actual.Table.Indexer.Value, subst)
			}
		}
	}
}

// substitute replaces every type variable in t with its binding from subst,
// returning a new Type. Types with no variables are returned as-is.
func substitute(t *Type, subst map[string]*Type) *Type {
	if t == nil {
		return t
	}
	switch t.Kind {
	case KindTypeParam:
		if b, ok := subst[t.AliasName]; ok && b != nil {
			return b
		}
		return t
	case KindUnion:
		members := make([]*Type, len(t.Union))
		for i, m := range t.Union {
			members[i] = substitute(m, subst)
		}
		return NewUnion(members...)
	case KindFunction:
		if t.Fn == nil {
			return t
		}
		params := substituteAll(t.Fn.Params, subst)
		returns := substituteAll(t.Fn.Returns, subst)
		va := t.Fn.VarargType
		if va != nil {
			va = substitute(va, subst)
		}
		return &Type{Kind: KindFunction, AliasName: t.AliasName, Fn: &FunctionShape{
			Params: params, Returns: returns, IsVararg: t.Fn.IsVararg,
			VarargType: va, TypeParams: t.Fn.TypeParams, Struct: t.Fn.Struct,
		}}
	case KindTable:
		if t.Table == nil {
			return t
		}
		fields := make([]TableField, len(t.Table.Fields))
		for i, f := range t.Table.Fields {
			fields[i] = TableField{Key: f.Key, Type: substitute(f.Type, subst)}
		}
		var idx *Indexer
		if t.Table.Indexer != nil {
			idx = &Indexer{
				Key:   substitute(t.Table.Indexer.Key, subst),
				Value: substitute(t.Table.Indexer.Value, subst),
			}
		}
		nt := NewTable(fields, idx)
		nt.AliasName = t.AliasName
		return nt
	}
	return t
}

func substituteAll(ts []*Type, subst map[string]*Type) []*Type {
	out := make([]*Type, len(ts))
	for i, t := range ts {
		out[i] = substitute(t, subst)
	}
	return out
}
