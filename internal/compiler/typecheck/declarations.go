package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) preResolveAliases(stmts []ast.Statement) {
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
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok && len(a.TypeParams) == 0 {
			c.env.aliases[a.Name] = c.resolveAST(a.Target)
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.recordEnum(e)
			if e.IsTagged() {
				t := NewTable(nil, &Indexer{Key: anyT, Value: anyT})
				t.AliasName = e.Name.Name
				c.env.aliases[e.Name.Name] = t
			} else {
				c.env.aliases[e.Name.Name] = classicEnumType(e)
			}
		}
		if st, ok := s.(*ast.StructStatement); ok && st.Name != nil && len(st.TypeParams) == 0 {
			c.env.aliases[st.Name.Name] = c.structType(st)
		}
	}
}

func structTableAST(s *ast.StructStatement) ast.TypeNode {
	fields := make([]ast.TypeTableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = ast.TypeTableField{Key: f.Name, Value: f.Type}
	}
	return &ast.TypeTable{BaseNode: s.BaseNode, Fields: fields}
}

func (c *checker) structType(s *ast.StructStatement) *Type {
	fields := make([]TableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = TableField{Key: f.Name, Type: c.resolveAST(f.Type)}
	}
	t := NewTable(fields, nil)
	t.AliasName = s.Name.Name
	return t
}

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

func classicEnumType(e *ast.EnumStatement) *Type {
	if len(e.Variants) == 0 {
		return numberT
	}
	members := make([]*Type, len(e.Variants))
	for i := range e.Variants {
		members[i] = NewNumberLiteral(float64(i+1), "")
	}
	t := NewUnion(members...)
	named := *t
	named.AliasName = e.Name.Name
	return &named
}

func classicEnumNamespaceType(e *ast.EnumStatement) *Type {
	fields := make([]TableField, len(e.Variants))
	for i, v := range e.Variants {
		fields[i] = TableField{Key: v.Name, Type: NewNumberLiteral(float64(i+1), "")}
	}
	return NewTable(fields, nil)
}

func (c *checker) taggedEnumNamespaceType(e *ast.EnumStatement) *Type {
	enumT := c.env.alias(e.Name.Name)
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

func (c *checker) structConstructorType(s *ast.StructStatement) *Type {
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
