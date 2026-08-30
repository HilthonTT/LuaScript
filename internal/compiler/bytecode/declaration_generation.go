package bytecode

// Codegen for enum and struct declarations (lowered onto runtime helpers).

import "github.com/hilthontt/luascript/internal/compiler/ast"

// compileEnumStatement lowers `enum Name V1, V2, ... end` to the
// equivalent of
//
//	local Name = __enum_freeze({V1=1, V2=2, ...}, "Name")
//
// where `__enum_freeze` is the runtime helper installed by
// native/enumrt at VM startup. The helper attaches a __newindex
// metamethod that raises on assignment and locks __metatable so the
// shield can't be removed.
//
// At REPL top-level the binding is promoted to a global (same rule
// `compileLocal` uses) so the name survives across REPL chunks.
//
// The emit sequence mirrors what compileTableConstructor does for a
// record literal:
//
//	GetGlobal "__enum_freeze"        ; push helper
//	NewTable 0 N                     ; push fresh table
//	for each variant i (1..N):
//	    Dup                          ; copy table reference
//	    LoadInt i                    ; push value
//	    SetField "VARIANT"           ; pops (value, table-copy)
//	LoadString "Name"                ; push enum name for the helper's diagnostic
//	Call 2 1                         ; helper(table, name) → frozen table
//	SetLocal slot / SetGlobal name   ; bind result
func (g *Generator) compileEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	if s.Name == nil || len(s.Variants) == 0 {
		// Parser already errors on these; defensive guard so codegen
		// doesn't panic on a partial AST under recovery.
		return
	}

	if s.IsTagged() {
		g.compileTaggedEnumStatement(is, s)
		return
	}

	line := s.Line()

	// Stack: [fn]
	is.define(GetGlobal, line, "__enum_freeze")

	// Stack: [fn, t]
	is.define(NewTable, line, 0, len(s.Variants))

	for i, v := range s.Variants {
		// Stack: [fn, t, t]
		is.define(Dup, line)
		// Stack: [fn, t, t, value]
		is.define(LoadInt, line, int64(i+1))
		// SetField pops (value, table-copy); back to [fn, t].
		is.define(SetField, line, v.Name)
	}

	// Stack: [fn, t, name]
	is.define(LoadString, line, s.Name.Name)

	// Stack: [frozen-t]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

// compileTaggedEnumStatement lowers a tagged (sum-type) enum
//
//	enum Shape
//	    Circle(number),
//	    Rect(number, number),
//	    Unit,
//	end
//
// to the equivalent of
//
//	local Shape = __enum_adt("Shape", { Circle = 1, Rect = 2, Unit = 0 })
//
// where the second argument maps each variant name to its payload arity.
// `__enum_adt` (installed by native/enumrt) builds a frozen namespace whose
// payload variants become constructor functions and whose nullary variants
// become singleton tagged values. Payload *types* are erased here — the
// checker already validated them; the runtime only needs the arity.
func (g *Generator) compileTaggedEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	line := s.Line()

	// Stack: [fn]
	is.define(GetGlobal, line, "__enum_adt")
	// Stack: [fn, name]
	is.define(LoadString, line, s.Name.Name)
	// Stack: [fn, name, arities]
	is.define(NewTable, line, 0, len(s.Variants))

	for _, v := range s.Variants {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(len(v.Payload)))
		is.define(SetField, line, v.Name)
	}

	// Stack: [frozen-namespace]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

// compileStructStatement lowers `struct Name { f1: T, f2: T } end` to the
// equivalent of
//
//	local Name = __struct_define("Name", {"f1", "f2"})
//
// where `__struct_define` is the runtime helper installed by
// native/structrt. It returns a constructor closed over the ordered field
// names; the type annotations are erased here (the checker consumed them).
// Binding follows the same local-vs-global rule enums and locals use so the
// name survives across REPL chunks at top level.
//
//	GetGlobal "__struct_define"      ; push factory
//	LoadString "Name"                ; struct name
//	NewTable N 0                     ; field-name array
//	for each field i (1..N):
//	    Dup; LoadInt i; LoadString "field"; SetTable
//	Call 2 1                         ; factory(name, fields) -> constructor
//	SetLocal slot / SetGlobal name   ; bind constructor
func (g *Generator) compileStructStatement(is *InstructionSet, s *ast.StructStatement) {
	if s.Name == nil || len(s.Fields) == 0 {
		// Parser already errors on these; defensive guard against a partial
		// AST under error recovery.
		return
	}

	line := s.Line()

	// Stack: [factory]
	is.define(GetGlobal, line, "__struct_define")
	// Stack: [factory, name]
	is.define(LoadString, line, s.Name.Name)
	// Stack: [factory, name, fields]
	is.define(NewTable, line, len(s.Fields), 0)

	for i, f := range s.Fields {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(i+1))
		is.define(LoadString, line, f.Name)
		is.define(SetTable, line)
	}

	// Stack: [constructor]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}
