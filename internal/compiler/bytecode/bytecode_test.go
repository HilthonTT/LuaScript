package bytecode

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func base(line int) ast.BaseNode {
	return ast.BaseNode{Token: token.Token{Line: line}}
}

func ident(name string, line int) *ast.Identifier {
	return &ast.Identifier{BaseNode: base(line), Name: name}
}

func intLit(v int64, line int) *ast.IntegerLiteral {
	return &ast.IntegerLiteral{BaseNode: base(line), Value: v}
}

func strLit(v string, line int) *ast.StringLiteral {
	return &ast.StringLiteral{BaseNode: base(line), Value: v}
}

func boolLit(v bool, line int) *ast.BooleanLiteral {
	return &ast.BooleanLiteral{BaseNode: base(line), Value: v}
}

func binOp(op string, l, r ast.Expression, line int) *ast.BinaryExpression {
	return &ast.BinaryExpression{BaseNode: base(line), Op: op, Left: l, Right: r}
}

func unOp(op string, operand ast.Expression, line int) *ast.UnaryExpression {
	return &ast.UnaryExpression{BaseNode: base(line), Op: op, Operand: operand}
}

func block(stmts ...ast.Statement) *ast.Block {
	return &ast.Block{BaseNode: base(0), Statements: stmts}
}

// generate runs the generator over the supplied statements and returns the
// emitted instruction sets (main chunk first).
func generate(t *testing.T, stmts []ast.Statement) []*InstructionSet {
	t.Helper()
	g := NewGenerator()
	g.InitTopLevelScope(&ast.Program{})
	return g.GenerateInstructions(stmts)
}

// opcodes returns just the opcode mnemonics from an instruction set, in order.
func opcodes(is *InstructionSet) []string {
	out := make([]string, len(is.Instructions))
	for i, ins := range is.Instructions {
		out[i] = ins.ActionName()
	}
	return out
}

func assertOpcodes(t *testing.T, is *InstructionSet, want ...string) {
	t.Helper()
	got := opcodes(is)
	if len(got) != len(want) {
		t.Fatalf("opcode count mismatch:\n got: %v\nwant: %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("opcode[%d] = %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}

// findFirst returns the first instruction with the given opcode, or nil.
func findFirst(is *InstructionSet, op uint8) *Instruction {
	for _, ins := range is.Instructions {
		if ins.Opcode == op {
			return ins
		}
	}
	return nil
}

func TestInstructionNameTableComplete(t *testing.T) {
	if len(InstructionNameTable) != int(InstructionCount) {
		t.Fatalf("InstructionNameTable has %d entries, expected %d (one per opcode)",
			len(InstructionNameTable), InstructionCount)
	}
	for op := range int(InstructionCount) {
		if InstructionNameTable[op] == "" {
			t.Errorf("opcode %d has empty mnemonic", op)
		}
	}
}

func TestInstructionInspect(t *testing.T) {
	is := &InstructionSet{}
	ins := is.define(LoadInt, 4, int64(42))
	got := ins.Inspect()
	if !strings.Contains(got, "loadint") {
		t.Errorf("Inspect missing mnemonic: %q", got)
	}
	if !strings.Contains(got, "42") {
		t.Errorf("Inspect missing param: %q", got)
	}
	if !strings.Contains(got, "source line: 4") {
		t.Errorf("Inspect missing source line: %q", got)
	}
	if ins.Line() != 0 {
		t.Errorf("Line() = %d, want 0 for first instruction", ins.Line())
	}
	if ins.SourceLine() != 4 {
		t.Errorf("SourceLine() = %d, want 4", ins.SourceLine())
	}
}

func TestAnchorLinePanicsWhenNoAnchor(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic from AnchorLine on instruction without anchor")
		}
	}()
	ins := &Instruction{Opcode: LoadNil}
	_ = ins.AnchorLine()
}

func TestEmptyProgramEmitsLeave(t *testing.T) {
	chunks := generate(t, nil)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	main := chunks[0]
	if main.Type() != Program {
		t.Errorf("main chunk type = %q, want %q", main.Type(), Program)
	}
	if main.Name() != Program {
		t.Errorf("main chunk name = %q, want %q", main.Name(), Program)
	}
	if !main.IsVararg {
		t.Errorf("main chunk should be vararg (Lua main chunk semantics)")
	}
	assertOpcodes(t, main, "leave")
}

func TestREPLModeOmitsTrailingLeave(t *testing.T) {
	g := NewGenerator()
	g.REPL = true
	g.InitTopLevelScope(&ast.Program{})
	chunks := g.GenerateInstructions(nil)
	if len(chunks[0].Instructions) != 0 {
		t.Errorf("REPL main chunk should not emit Leave; got %v", opcodes(chunks[0]))
	}
}

// In REPL mode, top-level `local x = v` is promoted to a global assignment so
// the binding survives across REPL inputs (each line is otherwise its own
// chunk and stack-frame locals die on return).
func TestREPLModePromotesTopLevelLocalToGlobal(t *testing.T) {
	g := NewGenerator()
	g.REPL = true
	g.InitTopLevelScope(&ast.Program{})
	chunks := g.GenerateInstructions([]ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "a"}},
			Values:   []ast.Expression{intLit(1, 1)},
		},
	})
	main := chunks[0]
	// loadint 1; setglobal "a"  (no setlocal, no leave because REPL mode)
	assertOpcodes(t, main, "loadint", "setglobal")
	if got := main.Instructions[1].Params[0].(string); got != "a" {
		t.Errorf("SetGlobal name = %q, want %q", got, "a")
	}
}

// Top-level `local function` in REPL mode is also promoted so the function
// is callable from later REPL inputs.
func TestREPLModePromotesTopLevelLocalFunctionToGlobal(t *testing.T) {
	g := NewGenerator()
	g.REPL = true
	g.InitTopLevelScope(&ast.Program{})
	chunks := g.GenerateInstructions([]ast.Statement{
		&ast.LocalFunctionStatement{
			BaseNode: base(1),
			Name:     "f",
			Func:     &ast.FunctionExpression{BaseNode: base(1), Body: block()},
		},
	})
	main := chunks[0]
	assertOpcodes(t, main, "closure", "setglobal")
	if got := main.Instructions[1].Params[0].(string); got != "f" {
		t.Errorf("SetGlobal name = %q, want %q", got, "f")
	}
}

// Locals inside a nested scope (do/if/loops/functions) keep normal Lua
// semantics even under REPL mode — only chunk-root declarations are promoted.
func TestREPLModeKeepsNestedLocalsAsLocals(t *testing.T) {
	g := NewGenerator()
	g.REPL = true
	g.InitTopLevelScope(&ast.Program{})
	chunks := g.GenerateInstructions([]ast.Statement{
		&ast.DoStatement{
			BaseNode: base(1),
			Body: block(&ast.LocalStatement{
				BaseNode: base(1),
				Names:    []ast.LocalName{{Name: "x"}},
				Values:   []ast.Expression{intLit(7, 1)},
			}),
		},
	})
	main := chunks[0]
	for _, ins := range main.Instructions {
		if ins.Opcode == SetGlobal {
			t.Fatalf("nested local must not become SetGlobal; got %v", opcodes(main))
		}
	}
	if findFirst(main, SetLocal) == nil {
		t.Fatalf("expected SetLocal for nested local; got %v", opcodes(main))
	}
}

// In NormalMode, top-level locals stay local (no behavior change for scripts).
func TestNormalModeTopLevelLocalStaysLocal(t *testing.T) {
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "a"}},
			Values:   []ast.Expression{intLit(1, 1)},
		},
	}
	main := generate(t, stmts)[0]
	for _, ins := range main.Instructions {
		if ins.Opcode == SetGlobal {
			t.Fatalf("NormalMode local must not become SetGlobal; got %v", opcodes(main))
		}
	}
	if findFirst(main, SetLocal) == nil {
		t.Fatalf("expected SetLocal in NormalMode; got %v", opcodes(main))
	}
}

func TestResetInstructionSetsClearsChunks(t *testing.T) {
	g := NewGenerator()
	g.InitTopLevelScope(&ast.Program{})
	g.GenerateInstructions([]ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "f"}},
			Values: []ast.Expression{&ast.FunctionExpression{
				BaseNode: base(1), Body: block(),
			}},
		},
	})
	if len(g.chunks) == 0 {
		t.Fatalf("expected at least one nested chunk after generating a function")
	}
	g.ResetInstructionSets()
	if len(g.chunks) != 0 {
		t.Errorf("ResetInstructionSets did not clear chunks: %d remain", len(g.chunks))
	}
}

func TestLocalSingleAssignment(t *testing.T) {
	// local x = 7
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "x"}},
			Values:   []ast.Expression{intLit(7, 1)},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main, "loadint", "setlocal", "leave")

	if got := main.Instructions[0].Params[0].(int64); got != 7 {
		t.Errorf("LoadInt param = %d, want 7", got)
	}
	if got := main.Instructions[1].Params[0].(int); got != 0 {
		t.Errorf("SetLocal slot = %d, want 0", got)
	}
	if main.NumLocals != 0 {
		// NumLocals on the main chunk isn't populated (only set by popFunction);
		// but the field default should remain 0.
		t.Errorf("main chunk NumLocals = %d, want 0 (set only on nested protos)", main.NumLocals)
	}
}

func TestLocalAdjustsExtraNamesWithNil(t *testing.T) {
	// local a, b, c = 1
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			Values:   []ast.Expression{intLit(1, 1)},
		},
	}
	main := generate(t, stmts)[0]
	// Expect: loadint(1), loadnil(2), then 3 setlocals
	assertOpcodes(t, main, "loadint", "loadnil", "setlocal", "setlocal", "setlocal", "leave")

	loadnil := main.Instructions[1]
	if got := loadnil.Params[0].(int); got != 2 {
		t.Errorf("LoadNil count = %d, want 2", got)
	}
}

func TestLocalDropsExtraValues(t *testing.T) {
	// local a = 1, 2, 3   → keep 1, pop the extras
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "a"}},
			Values:   []ast.Expression{intLit(1, 1), intLit(2, 1), intLit(3, 1)},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main,
		"loadint", "loadint", "loadint", // push 1, 2, 3
		"pop",      // discard the surplus
		"setlocal", // store the kept value
		"leave",
	)
	pop := main.Instructions[3]
	if got := pop.Params[0].(int); got != 2 {
		t.Errorf("Pop count = %d, want 2", got)
	}
}

func TestGlobalAssignment(t *testing.T) {
	// x = 1   (no `local`, no prior decl → global)
	stmts := []ast.Statement{
		&ast.AssignStatement{
			BaseNode: base(1),
			Targets:  []ast.Expression{ident("x", 1)},
			Values:   []ast.Expression{intLit(1, 1)},
		},
	}
	main := generate(t, stmts)[0]
	// emitExplistTo → loadint; then SetLocal into temp; GetLocal; SetGlobal x
	assertOpcodes(t, main, "loadint", "setlocal", "getlocal", "setglobal", "leave")
	if got := main.Instructions[3].Params[0].(string); got != "x" {
		t.Errorf("SetGlobal name = %q, want %q", got, "x")
	}
}

func TestIdentifierResolvesLocalBeforeGlobal(t *testing.T) {
	// local x = 1; x = 2
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "x"}},
			Values:   []ast.Expression{intLit(1, 1)},
		},
		&ast.AssignStatement{
			BaseNode: base(2),
			Targets:  []ast.Expression{ident("x", 2)},
			Values:   []ast.Expression{intLit(2, 2)},
		},
	}
	main := generate(t, stmts)[0]
	// no SetGlobal should appear
	for _, ins := range main.Instructions {
		if ins.Opcode == SetGlobal || ins.Opcode == GetGlobal {
			t.Fatalf("expected no global ops; got %v", opcodes(main))
		}
	}
}

func TestBinaryArithmeticOperators(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"+", "add"}, {"-", "sub"}, {"*", "mul"}, {"/", "div"},
		{"//", "floordiv"}, {"%", "mod"}, {"^", "pow"},
		{"&", "band"}, {"|", "bor"}, {"~", "bxor"},
		{"<<", "shl"}, {">>", "shr"},
		{"==", "eq"}, {"~=", "neq"},
		{"<", "lt"}, {"<=", "le"}, {">", "gt"}, {">=", "ge"},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			// local r = 1 <op> 2
			stmts := []ast.Statement{
				&ast.LocalStatement{
					BaseNode: base(1),
					Names:    []ast.LocalName{{Name: "r"}},
					Values: []ast.Expression{
						binOp(c.op, intLit(1, 1), intLit(2, 1), 1),
					},
				},
			}
			main := generate(t, stmts)[0]
			assertOpcodes(t, main, "loadint", "loadint", c.want, "setlocal", "leave")
		})
	}
}

func TestConcatEmitsExplicitCount(t *testing.T) {
	// local s = "a" .. "b"
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "s"}},
			Values: []ast.Expression{
				binOp("..", strLit("a", 1), strLit("b", 1), 1),
			},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main, "loadstring", "loadstring", "concat", "setlocal", "leave")
	concat := main.Instructions[2]
	if got := concat.Params[0].(int); got != 2 {
		t.Errorf("Concat count = %d, want 2", got)
	}
}

func TestUnaryOperators(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"-", "neg"}, {"not", "not"}, {"#", "len"}, {"~", "bnot"},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			stmts := []ast.Statement{
				&ast.LocalStatement{
					BaseNode: base(1),
					Names:    []ast.LocalName{{Name: "r"}},
					Values:   []ast.Expression{unOp(c.op, intLit(5, 1), 1)},
				},
			}
			main := generate(t, stmts)[0]
			assertOpcodes(t, main, "loadint", c.want, "setlocal", "leave")
		})
	}
}

func TestShortCircuitAndResolvesAnchor(t *testing.T) {
	// local r = true and 1
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "r"}},
			Values: []ast.Expression{
				binOp("and", boolLit(true, 1), intLit(1, 1), 1),
			},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main, "loadtrue", "jumpiffalsekeep", "loadint", "setlocal", "leave")
	jmp := main.Instructions[1]
	target, ok := jmp.Params[0].(int)
	if !ok {
		t.Fatalf("jump target not resolved to int: %T %v", jmp.Params[0], jmp.Params[0])
	}
	// The jump should land just past the `loadint` (i.e. on `setlocal`, line 3).
	if target != 3 {
		t.Errorf("`and` jump target = %d, want 3", target)
	}
}

func TestShortCircuitOrUsesJumpIfTrueKeep(t *testing.T) {
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "r"}},
			Values: []ast.Expression{
				binOp("or", boolLit(false, 1), intLit(1, 1), 1),
			},
		},
	}
	main := generate(t, stmts)[0]
	if main.Instructions[1].Opcode != JumpIfTrueKeep {
		t.Errorf("expected JumpIfTrueKeep for `or`, got %s", main.Instructions[1].ActionName())
	}
}

func TestIfElseAnchorsResolved(t *testing.T) {
	// if true then local a = 1 else local a = 2 end
	stmts := []ast.Statement{
		&ast.IfStatement{
			BaseNode: base(1),
			Clauses: []ast.IfClause{{
				Condition: boolLit(true, 1),
				Body: block(&ast.LocalStatement{
					BaseNode: base(2),
					Names:    []ast.LocalName{{Name: "a"}},
					Values:   []ast.Expression{intLit(1, 2)},
				}),
			}},
			Else: block(&ast.LocalStatement{
				BaseNode: base(3),
				Names:    []ast.LocalName{{Name: "a"}},
				Values:   []ast.Expression{intLit(2, 3)},
			}),
		},
	}
	main := generate(t, stmts)[0]
	// loadtrue, jumpiffalse(else), loadint, setlocal, jump(end), loadint, setlocal, leave
	assertOpcodes(t, main,
		"loadtrue", "jumpiffalse", "loadint", "setlocal", "jump", "loadint", "setlocal", "leave",
	)
	jf := main.Instructions[1]
	jend := main.Instructions[4]
	if jf.Params[0].(int) != 5 {
		t.Errorf("else jump target = %v, want 5 (start of else body)", jf.Params[0])
	}
	if jend.Params[0].(int) != 7 {
		t.Errorf("end jump target = %v, want 7 (leave)", jend.Params[0])
	}
}

func TestWhileLoopHasBackwardJump(t *testing.T) {
	// while true do break end
	stmts := []ast.Statement{
		&ast.WhileStatement{
			BaseNode:  base(1),
			Condition: boolLit(true, 1),
			Body:      block(&ast.BreakStatement{BaseNode: base(2)}),
		},
	}
	main := generate(t, stmts)[0]
	// loadtrue, jumpiffalse(exit), jump(break→exit), jump(back→top), leave
	assertOpcodes(t, main, "loadtrue", "jumpiffalse", "jump", "jump", "leave")

	jExit := main.Instructions[1]
	jBreak := main.Instructions[2]
	jBack := main.Instructions[3]

	if jExit.Params[0].(int) != 4 {
		t.Errorf("while exit jump = %v, want 4", jExit.Params[0])
	}
	if jBreak.Params[0].(int) != 4 {
		t.Errorf("break jump = %v, want 4 (loop exit)", jBreak.Params[0])
	}
	if jBack.Params[0].(int) != 0 {
		t.Errorf("backward jump = %v, want 0 (top of loop)", jBack.Params[0])
	}
}

func TestNumericForEmitsForPrepAndForLoop(t *testing.T) {
	// for i = 1, 10 do end
	stmts := []ast.Statement{
		&ast.NumericForStatement{
			BaseNode: base(1),
			Name:     "i",
			Start:    intLit(1, 1),
			Limit:    intLit(10, 1),
			Body:     block(),
		},
	}
	main := generate(t, stmts)[0]

	fp := findFirst(main, ForPrep)
	fl := findFirst(main, ForLoop)
	if fp == nil {
		t.Fatalf("expected a ForPrep instruction; got %v", opcodes(main))
	}
	if fl == nil {
		t.Fatalf("expected a ForLoop instruction; got %v", opcodes(main))
	}
	// Both should reference index slot 0.
	if got := fp.Params[0].(int); got != 0 {
		t.Errorf("ForPrep index slot = %d, want 0", got)
	}
	if got := fl.Params[0].(int); got != 0 {
		t.Errorf("ForLoop index slot = %d, want 0", got)
	}
	// Step omitted → generator should synthesize a LoadInt 1.
	loadint := findFirst(main, LoadInt)
	if loadint == nil {
		t.Fatalf("expected a LoadInt for synthesized step")
	}
}

func TestNumericForOmittedStepDefaultsToOne(t *testing.T) {
	stmts := []ast.Statement{
		&ast.NumericForStatement{
			BaseNode: base(1),
			Name:     "i",
			Start:    intLit(2, 1),
			Limit:    intLit(4, 1),
			Body:     block(),
		},
	}
	main := generate(t, stmts)[0]
	// First three instructions should be loadint(2), loadint(4), loadint(1)
	if main.Instructions[2].Opcode != LoadInt {
		t.Fatalf("third instruction should be LoadInt for default step; got %s",
			main.Instructions[2].ActionName())
	}
	if got := main.Instructions[2].Params[0].(int64); got != 1 {
		t.Errorf("default step value = %d, want 1", got)
	}
}

func TestTableConstructorMixedFields(t *testing.T) {
	// local t = { 10, x = 20, [99] = 30 }
	tc := &ast.TableConstructor{
		BaseNode: base(1),
		Fields: []ast.TableField{
			{Value: intLit(10, 1)},                                        // array
			{Key: ident("x", 1), Value: intLit(20, 1)},                    // record
			{Key: intLit(99, 1), Value: intLit(30, 1), IsBracketed: true}, // bracketed
		},
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "t"}},
			Values:   []ast.Expression{tc},
		},
	}
	main := generate(t, stmts)[0]

	nt := findFirst(main, NewTable)
	if nt == nil {
		t.Fatalf("expected NewTable; got %v", opcodes(main))
	}
	if nt.Params[0].(int) != 1 || nt.Params[1].(int) != 2 {
		t.Errorf("NewTable hints = (%v, %v), want (1, 2)", nt.Params[0], nt.Params[1])
	}

	// Record-style fields use SetField; array & bracketed use SetTable.
	var setFieldCount, setTableCount int
	for _, ins := range main.Instructions {
		switch ins.Opcode {
		case SetField:
			setFieldCount++
		case SetTable:
			setTableCount++
		}
	}
	if setFieldCount != 1 {
		t.Errorf("setfield count = %d, want 1", setFieldCount)
	}
	if setTableCount != 2 {
		t.Errorf("settable count = %d, want 2", setTableCount)
	}
}

func TestIndexDotUsesGetField(t *testing.T) {
	// local v = t.x   (assumes t global)
	idx := &ast.IndexExpression{
		BaseNode: base(1),
		Object:   ident("t", 1),
		Index:    strLit("x", 1),
		IsDot:    true,
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "v"}},
			Values:   []ast.Expression{idx},
		},
	}
	main := generate(t, stmts)[0]
	gf := findFirst(main, GetField)
	if gf == nil {
		t.Fatalf("expected GetField for dot-index; got %v", opcodes(main))
	}
	if gf.Params[0].(string) != "x" {
		t.Errorf("GetField key = %q, want %q", gf.Params[0], "x")
	}
}

func TestIndexBracketUsesGetTable(t *testing.T) {
	// local v = t[1]
	idx := &ast.IndexExpression{
		BaseNode: base(1),
		Object:   ident("t", 1),
		Index:    intLit(1, 1),
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "v"}},
			Values:   []ast.Expression{idx},
		},
	}
	main := generate(t, stmts)[0]
	if findFirst(main, GetTable) == nil {
		t.Errorf("expected GetTable; got %v", opcodes(main))
	}
	if findFirst(main, GetField) != nil {
		t.Errorf("did not expect GetField for bracket-index; got %v", opcodes(main))
	}
}

func TestCallEmitsCallOpcode(t *testing.T) {
	// f(1, 2)   as expression statement
	call := &ast.CallExpression{
		BaseNode: base(1),
		Func:     ident("f", 1),
		Args:     []ast.Expression{intLit(1, 1), intLit(2, 1)},
	}
	stmts := []ast.Statement{
		&ast.ExpressionStatement{BaseNode: base(1), Expression: call},
	}
	main := generate(t, stmts)[0]
	c := findFirst(main, Call)
	if c == nil {
		t.Fatalf("expected Call; got %v", opcodes(main))
	}
	if got := c.Params[0].(int); got != 2 {
		t.Errorf("Call nargs = %d, want 2", got)
	}
	if got := c.Params[1].(int); got != 0 {
		// Expression-statement context wants 0 results.
		t.Errorf("Call nresults = %d, want 0", got)
	}
}

func TestMethodCallEmitsSelf(t *testing.T) {
	// obj:m()
	mc := &ast.MethodCallExpression{
		BaseNode: base(1),
		Object:   ident("obj", 1),
		Method:   "m",
	}
	stmts := []ast.Statement{
		&ast.ExpressionStatement{BaseNode: base(1), Expression: mc},
	}
	main := generate(t, stmts)[0]
	self := findFirst(main, Self)
	if self == nil {
		t.Fatalf("expected Self for method call; got %v", opcodes(main))
	}
	if self.Params[0].(string) != "m" {
		t.Errorf("Self method key = %q, want %q", self.Params[0], "m")
	}
	c := findFirst(main, Call)
	if c == nil || c.Params[0].(int) != 1 {
		// nargs == 0 visible + 1 self = 1
		t.Errorf("Call nargs = %v, want 1 (implicit self)", c.Params[0])
	}
}

func TestReturnNoValues(t *testing.T) {
	stmts := []ast.Statement{
		&ast.ReturnStatement{BaseNode: base(1)},
	}
	main := generate(t, stmts)[0]
	r := findFirst(main, Return)
	if r == nil {
		t.Fatalf("expected Return; got %v", opcodes(main))
	}
	if got := r.Params[0].(int); got != 0 {
		t.Errorf("Return count = %d, want 0", got)
	}
}

func TestReturnMultipleValues(t *testing.T) {
	stmts := []ast.Statement{
		&ast.ReturnStatement{
			BaseNode: base(1),
			Values:   []ast.Expression{intLit(1, 1), intLit(2, 1), intLit(3, 1)},
		},
	}
	main := generate(t, stmts)[0]
	r := findFirst(main, Return)
	if got := r.Params[0].(int); got != 3 {
		t.Errorf("Return count = %d, want 3", got)
	}
}

func TestFunctionExpressionEmitsClosureAndProto(t *testing.T) {
	// local f = function() return 1 end
	fe := &ast.FunctionExpression{
		BaseNode: base(1),
		Body: &ast.Block{
			BaseNode: base(1),
			Return: &ast.ReturnStatement{
				BaseNode: base(1),
				Values:   []ast.Expression{intLit(1, 1)},
			},
		},
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "f"}},
			Values:   []ast.Expression{fe},
		},
	}
	chunks := generate(t, stmts)
	main := chunks[0]
	if findFirst(main, Closure) == nil {
		t.Fatalf("expected Closure in main chunk; got %v", opcodes(main))
	}
	if len(main.Protos) != 1 {
		t.Fatalf("expected 1 nested proto, got %d", len(main.Protos))
	}
	proto := main.Protos[0]
	if proto.Type() != FunctionDef {
		t.Errorf("proto type = %q, want %q", proto.Type(), FunctionDef)
	}
	// Body was `return 1` → loadint, return, leave
	assertOpcodes(t, proto, "loadint", "return", "leave")

	// chunks should be: [main, proto] (proto appended via popFunction → g.chunks)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (main + nested), got %d", len(chunks))
	}
}

func TestUpvalueCapturedFromEnclosingFunction(t *testing.T) {
	// local x = 1; local f = function() return x end
	fe := &ast.FunctionExpression{
		BaseNode: base(2),
		Body: &ast.Block{
			BaseNode: base(2),
			Return: &ast.ReturnStatement{
				BaseNode: base(2),
				Values:   []ast.Expression{ident("x", 2)},
			},
		},
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "x"}},
			Values:   []ast.Expression{intLit(1, 1)},
		},
		&ast.LocalStatement{
			BaseNode: base(2),
			Names:    []ast.LocalName{{Name: "f"}},
			Values:   []ast.Expression{fe},
		},
	}
	chunks := generate(t, stmts)
	if len(chunks) < 2 {
		t.Fatalf("expected nested proto, got %d chunks", len(chunks))
	}
	proto := chunks[0].Protos[0]
	if findFirst(proto, GetUpvalue) == nil {
		t.Fatalf("expected GetUpvalue for `x`; got %v", opcodes(proto))
	}
	if len(proto.Upvalues) != 1 {
		t.Fatalf("expected 1 upvalue, got %d", len(proto.Upvalues))
	}
	uv := proto.Upvalues[0]
	if uv.Name != "x" {
		t.Errorf("upvalue name = %q, want %q", uv.Name, "x")
	}
	if !uv.InStack {
		t.Errorf("upvalue should reference enclosing local (InStack=true)")
	}
	if uv.Index != 0 {
		t.Errorf("upvalue index = %d, want 0 (slot of `x` in main)", uv.Index)
	}
}

func TestFunctionParametersBecomeLocals(t *testing.T) {
	// local f = function(a, b) return a end   → proto has GetLocal(0)
	fe := &ast.FunctionExpression{
		BaseNode: base(1),
		Params: []ast.TypedParam{
			{Name: ident("a", 1)},
			{Name: ident("b", 1)},
		},
		Body: &ast.Block{
			BaseNode: base(1),
			Return: &ast.ReturnStatement{
				BaseNode: base(1),
				Values:   []ast.Expression{ident("a", 1)},
			},
		},
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "f"}},
			Values:   []ast.Expression{fe},
		},
	}
	proto := generate(t, stmts)[0].Protos[0]
	if proto.NumParams != 2 {
		t.Errorf("NumParams = %d, want 2", proto.NumParams)
	}
	gl := findFirst(proto, GetLocal)
	if gl == nil {
		t.Fatalf("expected GetLocal for parameter; got %v", opcodes(proto))
	}
	if gl.Params[0].(int) != 0 {
		t.Errorf("GetLocal slot = %d, want 0 (first parameter)", gl.Params[0])
	}
	// `a` and `b` declared in the function's root scope ⇒ NumLocals == 2.
	if proto.NumLocals != 2 {
		t.Errorf("NumLocals = %d, want 2", proto.NumLocals)
	}
}

func TestVarargFunctionFlag(t *testing.T) {
	fe := &ast.FunctionExpression{
		BaseNode: base(1),
		IsVararg: true,
		Body:     block(),
	}
	stmts := []ast.Statement{
		&ast.LocalStatement{
			BaseNode: base(1),
			Names:    []ast.LocalName{{Name: "f"}},
			Values:   []ast.Expression{fe},
		},
	}
	proto := generate(t, stmts)[0].Protos[0]
	if !proto.IsVararg {
		t.Errorf("proto IsVararg = false, want true")
	}
}

func TestMethodFunctionDeclarationInjectsSelfParam(t *testing.T) {
	// function obj:m() end   → proto NumParams == 1 (implicit self)
	stmts := []ast.Statement{
		&ast.FunctionDeclaration{
			BaseNode:   base(1),
			Name:       ident("obj", 1),
			MethodName: "m",
			Func: &ast.FunctionExpression{
				BaseNode: base(1),
				Body:     block(),
			},
		},
	}
	chunks := generate(t, stmts)
	if len(chunks) < 2 {
		t.Fatalf("expected nested proto for method")
	}
	proto := chunks[0].Protos[0]
	if proto.NumParams != 1 {
		t.Errorf("method proto NumParams = %d, want 1 (implicit self)", proto.NumParams)
	}
}

func TestLocalTableScopingAndShadowing(t *testing.T) {
	lt := newLocalTable(nil)

	a := lt.define("a")
	b := lt.define("b")
	if a != 0 || b != 1 {
		t.Errorf("initial slots = (%d, %d), want (0, 1)", a, b)
	}

	lt.openScope()
	c := lt.define("c")
	a2 := lt.define("a") // shadow
	if c != 2 || a2 != 3 {
		t.Errorf("inner-scope slots = (%d, %d), want (2, 3)", c, a2)
	}

	got, ok := lt.lookupLocal("a")
	if !ok || got != 3 {
		t.Errorf("lookup `a` after shadow = (%d, %v), want (3, true)", got, ok)
	}
	got, ok = lt.lookupLocal("b")
	if !ok || got != 1 {
		t.Errorf("lookup `b` from inner scope = (%d, %v), want (1, true)", got, ok)
	}

	lt.closeScope()

	got, ok = lt.lookupLocal("a")
	if !ok || got != 0 {
		t.Errorf("lookup `a` after closing = (%d, %v), want (0, true)", got, ok)
	}
	if _, ok := lt.lookupLocal("c"); ok {
		t.Errorf("`c` should not be visible after its scope closed")
	}

	// maxSlot tracks the high-water mark.
	if lt.maxSlot != 4 {
		t.Errorf("maxSlot = %d, want 4 (shadow grew the frame)", lt.maxSlot)
	}
	// nextSlot rolled back.
	if lt.nextSlot != 2 {
		t.Errorf("nextSlot = %d, want 2 after closeScope", lt.nextSlot)
	}
}

func TestLocalTableDoesNotWalkIntoParent(t *testing.T) {
	parent := newLocalTable(nil)
	parent.define("x")
	child := newLocalTable(parent)
	if _, ok := child.lookupLocal("x"); ok {
		t.Errorf("lookupLocal must not cross function boundaries (use upvalue resolution instead)")
	}
}

func TestCloseScopeIsSafeOnEmpty(t *testing.T) {
	lt := &localTable{} // no scope opened
	// Should not panic.
	lt.closeScope()
}

// TestStructLowersToDefineCall asserts a struct declaration lowers to a
// `__struct_define("Name", {fields...})` call bound to a local, mirroring
// the enum lowering strategy.
func TestStructLowersToStructDefineCall(t *testing.T) {
	// struct Point { x: number, y: number }
	stmts := []ast.Statement{
		&ast.StructStatement{
			BaseNode: base(1),
			Name:     ident("Point", 1),
			Fields: []ast.StructField{
				{Name: "x", Type: &ast.TypePrimitive{BaseNode: base(1), Name: "number"}},
				{Name: "y", Type: &ast.TypePrimitive{BaseNode: base(1), Name: "number"}},
			},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main,
		"getglobal",                                // __struct_define
		"loadstring",                               // "Point"
		"newtable",                                 // field-name array
		"dup", "loadint", "loadstring", "settable", // "x"
		"dup", "loadint", "loadstring", "settable", // "y"
		"call",     // __struct_define(name, fields)
		"setlocal", // bind Point
		"leave",
	)

	// The GetGlobal target is the runtime helper.
	gg := findFirst(main, GetGlobal)
	if gg == nil || gg.Params[0].(string) != "__struct_define" {
		t.Fatalf("expected getglobal __struct_define, got %v", opcodes(main))
	}
}

// TestTaggedEnumLowersToADTCall asserts a tagged enum lowers to an
// `__enum_adt("Name", {Variant = arity, ...})` call, distinct from the
// classic integer-freeze path.
func TestTaggedEnumLowersToADTCall(t *testing.T) {
	numT := &ast.TypePrimitive{BaseNode: base(1), Name: "number"}
	// enum Shape Circle(number), Rect(number, number), Unit end
	stmts := []ast.Statement{
		&ast.EnumStatement{
			BaseNode: base(1),
			Name:     ident("Shape", 1),
			Variants: []*ast.EnumVariantDef{
				{Name: "Circle", Payload: []ast.TypeNode{numT}},
				{Name: "Rect", Payload: []ast.TypeNode{numT, numT}},
				{Name: "Unit"},
			},
		},
	}
	main := generate(t, stmts)[0]
	assertOpcodes(t, main,
		"getglobal",                  // __enum_adt
		"loadstring",                 // "Shape"
		"newtable",                   // arities hash
		"dup", "loadint", "setfield", // Circle = 1
		"dup", "loadint", "setfield", // Rect = 2
		"dup", "loadint", "setfield", // Unit = 0
		"call",
		"setlocal",
		"leave",
	)
	gg := findFirst(main, GetGlobal)
	if gg == nil || gg.Params[0].(string) != "__enum_adt" {
		t.Fatalf("expected getglobal __enum_adt, got %v", opcodes(main))
	}
}

// TestPlainEnumStillUsesFreeze guards the classic integer-enum path against
// regressions from the tagged-enum branch.
func TestPlainEnumStillUsesFreeze(t *testing.T) {
	stmts := []ast.Statement{
		&ast.EnumStatement{
			BaseNode: base(1),
			Name:     ident("Color", 1),
			Variants: []*ast.EnumVariantDef{{Name: "RED"}, {Name: "GREEN"}},
		},
	}
	main := generate(t, stmts)[0]
	gg := findFirst(main, GetGlobal)
	if gg == nil || gg.Params[0].(string) != "__enum_freeze" {
		t.Fatalf("expected getglobal __enum_freeze, got %v", opcodes(main))
	}
}
