package bytecode

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (g *Generator) compileEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	if s.Name == nil || len(s.Variants) == 0 {
		return
	}

	if s.IsTagged() {
		g.compileTaggedEnumStatement(is, s)
		return
	}

	line := s.Line()

	is.define(GetGlobal, line, "__enum_freeze")

	is.define(NewTable, line, 0, len(s.Variants))

	for i, v := range s.Variants {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(i+1))
		is.define(SetField, line, v.Name)
	}

	is.define(LoadString, line, s.Name.Name)

	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

func (g *Generator) compileTaggedEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	line := s.Line()

	is.define(GetGlobal, line, "__enum_adt")
	is.define(LoadString, line, s.Name.Name)
	is.define(NewTable, line, 0, len(s.Variants))

	for _, v := range s.Variants {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(len(v.Payload)))
		is.define(SetField, line, v.Name)
	}

	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

func (g *Generator) compileStructStatement(is *InstructionSet, s *ast.StructStatement) {
	if s.Name == nil || len(s.Fields) == 0 {
		return
	}

	line := s.Line()

	is.define(GetGlobal, line, "__struct_define")
	is.define(LoadString, line, s.Name.Name)
	is.define(NewTable, line, len(s.Fields), 0)

	for i, f := range s.Fields {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(i+1))
		is.define(LoadString, line, f.Name)
		is.define(SetTable, line)
	}

	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}
