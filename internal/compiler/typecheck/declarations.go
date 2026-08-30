package typecheck

// Declaration handling: type aliases, structs, and enums.

import "github.com/hilthontt/luascript/internal/compiler/ast"

// preResolveAliases scans top-level statements for `type Name = T` and
// registers each in the alias table. Resolution happens in two passes so
// forward references work.
//
// Enum declarations contribute to the same alias table. A classic
// integer enum `enum Color RED, GREEN end` registers `Color` as the
// literal union `1 | 2` — the exact set of values the runtime can
// produce, since variants are 1-based and assigned in source order. That
// makes `function paint(c: Color)` reject `paint(99)`, lets `c == Color.RED`
// narrow, and gives `match` a finite domain to check for exhaustiveness.
// A tagged enum instead registers a nominal opaque table (see below).
func (c *checker) preResolveAliases(stmts []ast.Statement) {
	// Pass 1: register placeholders so name → never (sentinel). Generic
	// declarations register their template immediately (instantiation is
	// lazy, so no forward-reference problem) and skip the placeholder.
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok {
			if len(a.TypeParams) > 0 {
				c.env.generics[a.Name] = &genericAlias{params: a.TypeParams, target: a.Target}
			} else {
				c.env.aliases[a.Name] = neverT
			}
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.env.aliases[e.Name.Name] = neverT
		}
		if st, ok := s.(*ast.StructStatement); ok && st.Name != nil {
			if len(st.TypeParams) > 0 {
				c.env.generics[st.Name.Name] = &genericAlias{
					params: st.TypeParams,
					target: structTableAST(st),
				}
			} else {
				c.env.aliases[st.Name.Name] = neverT
			}
		}
	}
	// Pass 2: resolve each alias's RHS with the table populated. Self-
	// references at top level resolve to whatever they reference, with
	// recursive cycles yielding neverT (which produces a downstream
	// "unknown" error if used).
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok && len(a.TypeParams) == 0 {
			c.env.aliases[a.Name] = c.resolveAST(a.Target)
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.recordEnum(e)
			if e.IsTagged() {
				// A tagged enum's *type* is a nominal opaque table: field/
				// index access on a value yields `any` (so `s.__tag`, `s[1]`
				// don't error), but a bare number/string is NOT a Shape.
				t := NewTable(nil, &Indexer{Key: anyT, Value: anyT})
				t.AliasName = e.Name.Name
				c.env.aliases[e.Name.Name] = t
			} else {
				c.env.aliases[e.Name.Name] = classicEnumType(e)
			}
		}
		if st, ok := s.(*ast.StructStatement); ok && st.Name != nil && len(st.TypeParams) == 0 {
			// A struct name resolves, as a *type*, to its structural table
			// `{ f1: T1, ... }` tagged with the struct's name for readable
			// diagnostics. The *value* side (the constructor) is bound later
			// in walkStatement. Generic structs live in the generics table
			// instead and are instantiated via `Name<Args>`.
			c.env.aliases[st.Name.Name] = c.structType(st)
		}
	}
}

// structTableAST builds a synthetic table-type AST for a struct's fields, so
// a generic struct can be stored as a generic alias template and instantiated
// through the same `Name<Args>` path as a generic `type` alias.
func structTableAST(s *ast.StructStatement) ast.TypeNode {
	fields := make([]ast.TypeTableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = ast.TypeTableField{Key: f.Name, Value: f.Type}
	}
	return &ast.TypeTable{BaseNode: s.BaseNode, Fields: fields}
}

// structType builds the nominal table type a struct name resolves to. The
// AliasName is the struct name so `typeof(p)`-style diagnostics show `Point`
// rather than the expanded `{ x: number, y: number }`.
func (c *checker) structType(s *ast.StructStatement) *Type {
	fields := make([]TableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = TableField{Key: f.Name, Type: c.resolveAST(f.Type)}
	}
	t := NewTable(fields, nil)
	t.AliasName = s.Name.Name
	return t
}

// recordEnum notes an enum's variant names so `match` exhaustiveness can
// enumerate its domain and name the cases an arm list is missing. Called
// from both the top-level pre-pass and the walker, so an enum declared
// inside a function body is registered too; re-registering is harmless.
func (c *checker) recordEnum(e *ast.EnumStatement) {
	names := make([]string, len(e.Variants))
	for i, v := range e.Variants {
		names[i] = v.Name
	}
	if e.IsTagged() {
		c.taggedEnums[e.Name.Name] = names
	} else {
		c.classicEnums[e.Name.Name] = names
	}
}

// classicEnumType builds the type a classic integer enum's *name* denotes:
// the union of its members' values. Variants are 1-based and numbered in
// source order (the runtime contract enumrt implements), so `enum Color RED,
// GREEN, BLUE end` is `1 | 2 | 3`. The AliasName keeps diagnostics reading
// `Color` rather than the expansion.
//
// A variant-less enum has no inhabitants to enumerate; it falls back to
// `number` rather than `never`, which would reject every use.
func classicEnumType(e *ast.EnumStatement) *Type {
	if len(e.Variants) == 0 {
		return numberT
	}
	members := make([]*Type, len(e.Variants))
	for i := range e.Variants {
		members[i] = NewNumberLiteral(float64(i+1), "")
	}
	t := NewUnion(members...)
	// NewUnion may have collapsed to a single member; copy before naming it
	// so the shared singleton isn't mutated.
	named := *t
	named.AliasName = e.Name.Name
	return &named
}

// classicEnumNamespaceType builds the type of a classic enum's namespace
// value: one field per variant, each typed as its own singleton number.
func classicEnumNamespaceType(e *ast.EnumStatement) *Type {
	fields := make([]TableField, len(e.Variants))
	for i, v := range e.Variants {
		fields[i] = TableField{Key: v.Name, Type: NewNumberLiteral(float64(i+1), "")}
	}
	return NewTable(fields, nil)
}

// taggedEnumNamespaceType builds the type of a tagged enum's namespace
// value. Each variant becomes a field: payload variants map to constructor
// functions `(P...) -> Enum`, nullary variants map to `Enum` itself.
func (c *checker) taggedEnumNamespaceType(e *ast.EnumStatement) *Type {
	enumT := c.env.alias(e.Name.Name) // the nominal type registered in the pre-pass
	fields := make([]TableField, 0, len(e.Variants))
	for _, v := range e.Variants {
		if len(v.Payload) == 0 {
			fields = append(fields, TableField{Key: v.Name, Type: enumT})
			continue
		}
		params := make([]*Type, len(v.Payload))
		for i, p := range v.Payload {
			params[i] = c.resolveAST(p)
		}
		ctor := NewFunction(params, []*Type{enumT}, false, nil)
		fields = append(fields, TableField{Key: v.Name, Type: ctor})
	}
	return NewTable(fields, nil)
}

// structConstructorType builds the constructor function type bound to a
// struct's value name. Positionally it is `(T1, T2, ...) -> Struct`; the
// Struct marker also lets typeOfCall accept the `Name{ ... }` named form.
func (c *checker) structConstructorType(s *ast.StructStatement) *Type {
	// For a generic struct the field types mention the parameters, so bring
	// them into scope as type variables while building the constructor. A
	// call like `Box(5)` then infers `T` and yields `Box<number>`.
	restore := c.pushTypeParams(s.TypeParams)
	defer restore()

	shape := c.structType(s)
	params := make([]*Type, len(shape.Table.Fields))
	for i, f := range shape.Table.Fields {
		params[i] = f.Type
	}
	return &Type{
		Kind: KindFunction,
		Fn: &FunctionShape{
			Params:     params,
			Returns:    []*Type{shape},
			TypeParams: s.TypeParams,
			Struct:     &StructCtor{Name: s.Name.Name, Shape: shape.Table},
		},
	}
}
