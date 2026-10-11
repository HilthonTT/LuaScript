package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) pushTypeParams(params []ast.TypeParam) func() {
	if len(params) == 0 {
		return func() {}
	}
	type prev struct {
		t   *Type
		had bool
	}
	saved := make(map[string]prev, len(params))
	for _, tp := range params {
		t, had := c.env.aliases[tp.Name]
		saved[tp.Name] = prev{t: t, had: had}
		c.env.aliases[tp.Name] = &Type{Kind: KindTypeParam, AliasName: tp.Name}
	}
	for _, tp := range params {
		if tp.Constraint == nil {
			continue
		}
		c.env.aliases[tp.Name].Bound = c.resolveAST(tp.Constraint)
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

func (c *checker) typeBounds(params []ast.TypeParam) map[string]*Type {
	var out map[string]*Type
	for _, tp := range params {
		if tp.Constraint == nil {
			continue
		}
		if out == nil {
			out = map[string]*Type{}
		}
		out[tp.Name] = c.resolveAST(tp.Constraint)
	}
	return out
}

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

	args := make([]*Type, len(app.Args))
	for i, a := range app.Args {
		args[i] = c.resolveAST(a)
	}
	for i, tp := range g.params {
		if tp.Constraint == nil {
			continue
		}
		bound := c.resolveAST(tp.Constraint)
		if !assignable(args[i], bound) {
			c.errf(app.Line(), "constraint",
				"type argument %q does not satisfy the constraint %q on %q",
				args[i].String(), bound.String(), tp.Name)
		}
	}
	restore := c.bindParamTypes(g.params, args)
	defer restore()

	t := c.resolveAST(g.target)
	if t != nil && t.AliasName == "" {
		withName := *t
		withName.AliasName = app.String()
		return &withName
	}
	return t
}

func (c *checker) bindParamTypes(params []ast.TypeParam, args []*Type) func() {
	type prev struct {
		t   *Type
		had bool
	}
	saved := make(map[string]prev, len(params))
	for i, tp := range params {
		n := tp.Name
		t, had := c.env.aliases[n]
		saved[n] = prev{t: t, had: had}
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

func (c *checker) instantiateCall(line int, fn *FunctionShape, args []*Type) []*Type {
	subst := map[string]*Type{}
	for _, name := range fn.TypeParams {
		subst[name] = nil
	}
	n := min(len(args), len(fn.Params))
	for i := range n {
		unify(fn.Params[i], args[i], subst)
	}
	for name, t := range subst {
		if t == nil {
			subst[name] = anyT
			continue
		}
		if bound, ok := fn.TypeBounds[name]; ok && !assignable(t, bound) {
			c.errf(line, "constraint",
				"type argument %q does not satisfy the constraint %q on %q",
				t.String(), bound.String(), name)
		}
	}
	out := make([]*Type, len(fn.Returns))
	for i, r := range fn.Returns {
		out[i] = substitute(r, subst)
	}
	return out
}

func unify(declared, actual *Type, subst map[string]*Type) {
	if declared == nil || actual == nil {
		return
	}
	if declared.Kind == KindTypeParam {
		if _, tracked := subst[declared.AliasName]; tracked {
			if subst[declared.AliasName] == nil {
				subst[declared.AliasName] = widen(actual)
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
			VarargType: va, TypeParams: t.Fn.TypeParams, TypeBounds: t.Fn.TypeBounds,
			Struct: t.Fn.Struct,
		}}
	case KindTable:
		if t.Table == nil {
			return t
		}
		changed := false
		fields := make([]TableField, len(t.Table.Fields))
		for i, f := range t.Table.Fields {
			fields[i] = TableField{Key: f.Key, Type: substitute(f.Type, subst)}
			changed = changed || fields[i].Type != f.Type
		}
		var idx *Indexer
		if t.Table.Indexer != nil {
			idx = &Indexer{
				Key:   substitute(t.Table.Indexer.Key, subst),
				Value: substitute(t.Table.Indexer.Value, subst),
			}
			changed = changed || idx.Key != t.Table.Indexer.Key || idx.Value != t.Table.Indexer.Value
		}
		nt := NewTable(fields, idx)
		// A name like "Box<T>" would misdescribe the instantiated shape.
		if !changed {
			nt.AliasName = t.AliasName
		}
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
