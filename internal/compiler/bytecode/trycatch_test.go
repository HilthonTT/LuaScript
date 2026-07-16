package bytecode

// Codegen tests for `try ... catch ... end` and `throw`. These assert the
// exact shape of the protected region, since the VM's unwind depends on it:
// the Try's operand must address the catch clause, and every non-raising exit
// from the region must be preceded by an EndTry.

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// tryStmt builds `try <try> catch [name] do <catch> end`. An empty name means
// the handler takes no binding.
func tryStmt(name string, try, catch *ast.Block) *ast.TryCatchStatement {
	s := &ast.TryCatchStatement{BaseNode: base(1), Try: try, Catch: catch}
	if name != "" {
		s.CatchVar = ident(name, 1)
	}
	return s
}

func throwStmt(v ast.Expression) *ast.ThrowStatement {
	return &ast.ThrowStatement{BaseNode: base(1), Value: v}
}

func TestTryCatchEmitsProtectedRegion(t *testing.T) {
	main := generate(t, []ast.Statement{
		tryStmt("e", block(throwStmt(strLit("boom", 1))), block()),
	})[0]

	assertOpcodes(t, main,
		"try",        // 0: install handler; operand addresses the catch clause
		"loadstring", // 1: "boom"
		"throw",      // 2
		"endtry",     // 3: body finished cleanly — pop the handler
		"jump",       // 4: skip the handler
		"setlocal",   // 5: catch target — bind the error value the VM pushed
		"leave",      // 6
	)

	if got, want := int(main.Instructions[0].A), 5; got != want {
		t.Errorf("try target = %d, want %d (the setlocal that binds the error)", got, want)
	}
	if got := int(main.Instructions[3].A); got != 1 {
		t.Errorf("endtry count = %d, want 1", got)
	}
	if got, want := int(main.Instructions[4].A), 6; got != want {
		t.Errorf("jump target = %d, want %d (past the handler)", got, want)
	}
}

// A handler with no binding must still consume the error value the VM pushes,
// or the stack would be left one deep.
func TestTryCatchWithoutBindingPopsErrorValue(t *testing.T) {
	main := generate(t, []ast.Statement{
		tryStmt("", block(throwStmt(strLit("x", 1))), block()),
	})
	assertOpcodes(t, main[0],
		"try", "loadstring", "throw", "endtry", "jump", "pop", "leave",
	)
	if got := int(main[0].Instructions[5].A); got != 1 {
		t.Errorf("pop count = %d, want 1", got)
	}
}

func TestThrowEmitsThrowOpcode(t *testing.T) {
	main := generate(t, []ast.Statement{throwStmt(strLit("boom", 1))})[0]
	assertOpcodes(t, main, "loadstring", "throw", "leave")
}

// TestBreakInsideTryEmitsEndTry is the codegen half of the stale-handler guard:
// a `break` that leaves a protected region must pop its handler before jumping.
func TestBreakInsideTryEmitsEndTry(t *testing.T) {
	main := generate(t, []ast.Statement{
		&ast.WhileStatement{
			BaseNode:  base(1),
			Condition: boolLit(true, 1),
			Body: block(tryStmt("e",
				block(&ast.BreakStatement{BaseNode: base(1)}),
				block(),
			)),
		},
	})[0]

	endtry := findFirst(main, EndTry)
	if endtry == nil {
		t.Fatal("no endtry emitted for a break out of a try")
	}
	// The first EndTry must be the break's, ahead of the body's own.
	if got := int(endtry.A); got != 1 {
		t.Errorf("endtry count = %d, want 1", got)
	}
	// [loadtrue jumpiffalse try | endtry jump | endtry jump setlocal ...]:
	// indices 3-4 are the break's pop-then-jump, ahead of the body's own pair.
	ops := opcodes(main)
	if ops[3] != "endtry" || ops[4] != "jump" {
		t.Errorf("expected the break to emit endtry then jump; got %v", ops)
	}
}

// A break out of two nested try regions pops both with a single EndTry.
func TestBreakOutOfNestedTryEmitsCountedEndTry(t *testing.T) {
	main := generate(t, []ast.Statement{
		&ast.WhileStatement{
			BaseNode:  base(1),
			Condition: boolLit(true, 1),
			Body: block(tryStmt("outer",
				block(tryStmt("inner",
					block(&ast.BreakStatement{BaseNode: base(1)}),
					block(),
				)),
				block(),
			)),
		},
	})[0]

	endtry := findFirst(main, EndTry)
	if endtry == nil {
		t.Fatal("no endtry emitted")
	}
	if got := int(endtry.A); got != 2 {
		t.Errorf("endtry count = %d, want 2 (one per escaped try)", got)
	}
}

// A break in a loop *inside* a try stays within the protected region, so it
// escapes nothing and must not pop the handler.
func TestBreakInsideLoopInsideTryEmitsNoEndTry(t *testing.T) {
	main := generate(t, []ast.Statement{
		tryStmt("e",
			block(&ast.WhileStatement{
				BaseNode:  base(1),
				Condition: boolLit(true, 1),
				Body:      block(&ast.BreakStatement{BaseNode: base(1)}),
			}),
			block(),
		),
	})[0]

	// Exactly one endtry: the try body's own clean-exit pop.
	n := 0
	for _, ins := range main.Instructions {
		if ins.Opcode == EndTry {
			n++
		}
	}
	if n != 1 {
		t.Errorf("emitted %d endtry instructions, want 1 (the body's clean exit only)\n%v", n, opcodes(main))
	}
}

// A `return` out of a try needs no EndTry: handlers live on the frame, so the
// return discards them along with it.
func TestReturnInsideTryEmitsNoEndTry(t *testing.T) {
	body := block()
	body.Return = &ast.ReturnStatement{BaseNode: base(1), Values: []ast.Expression{intLit(1, 1)}}
	main := generate(t, []ast.Statement{tryStmt("e", body, block())})[0]

	ops := opcodes(main)
	if ops[1] != "loadint" || ops[2] != "return" {
		t.Errorf("expected the return to emit directly with no endtry; got %v", ops)
	}
}

func TestGotoOutOfTryIsRejected(t *testing.T) {
	g := NewGenerator()
	g.InitTopLevelScope(&ast.Program{})
	g.GenerateInstructions([]ast.Statement{
		tryStmt("e",
			block(&ast.GotoStatement{BaseNode: base(1), Label: "out"}),
			block(),
		),
		&ast.LabelStatement{BaseNode: base(1), Name: "out"},
	})
	err := g.Err()
	if err == nil {
		t.Fatal("expected an error for a goto that jumps out of a try")
	}
	if want := "jumps out of a 'try' block"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestGotoIntoTryIsRejected(t *testing.T) {
	g := NewGenerator()
	g.InitTopLevelScope(&ast.Program{})
	g.GenerateInstructions([]ast.Statement{
		&ast.GotoStatement{BaseNode: base(1), Label: "inside"},
		tryStmt("e",
			block(&ast.LabelStatement{BaseNode: base(1), Name: "inside"}),
			block(),
		),
	})
	err := g.Err()
	if err == nil {
		t.Fatal("expected an error for a goto that jumps into a try")
	}
	if want := "jumps into a 'try' block"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}
