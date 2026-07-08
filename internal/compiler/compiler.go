package compiler

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/constcheck"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/optimize"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/compiler/typecheck"
)

func CompileToInstructions(input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	return CompileToInstructionsWith(bytecode.NewGenerator(), input, pm)
}

// CompileToInstructionsWith is the REPL-friendly variant: callers pass in a
// generator they own, allowing it to live across multiple compile calls. The
// generator's prior chunk output is cleared on entry; any other state the
// caller has stashed on it is preserved.
//
// The pipeline runs lex → parse → typecheck → bytecode. The type-check
// pass is gated by Luau-style mode directives in the source:
//   - `--!nocheck` skips the pass entirely.
//   - `--!strict` enables strict mode (implicit-`any` errors etc.).
//   - Anything else (default `--!nonstrict`) runs the gradual checker.
//
// Type errors are returned as a *typecheck.TypeErrors with the same
// shape as parser errors (single error value, multi-line via Error()).
func CompileToInstructionsWith(g *bytecode.Generator, input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	l := lexer.New(input)
	p := parser.New(l)
	p.Mode = pm
	g.REPL = pm == parser.REPLMode

	program, err := p.ParseProgram()
	if err != nil {
		// Return the typed parser error directly so REPL callers can
		// inspect its category via err.IsEOF() etc.
		return nil, err
	}

	// Attribute enforcement (`<const>`/`<close>` reassignment) always runs —
	// unlike the type checker it is not gated by `--!nocheck`, matching PUC
	// Lua where assigning to a const local is a compile error.
	if err := constcheck.Check(program); err != nil {
		return nil, err
	}

	if l.ModeDirective != "nocheck" {
		opts := typecheck.Options{Strict: l.ModeDirective == "strict"}
		if errs := typecheck.Check(program, opts); len(errs) > 0 {
			return nil, &typecheck.TypeErrors{Errors: errs}
		}
	}

	// Constant folding runs after type checking (so the checker sees the
	// original expressions) and before bytecode generation. It only applies
	// semantics-preserving rewrites, so it is safe to run unconditionally.
	optimize.Fold(program)

	g.ResetInstructionSets()
	g.InitTopLevelScope(program)

	// Per Lua's BNF a chunk is a block, and a block carries its trailing
	// `return` separately from regular statements. The generator's
	// top-level entrypoint only sees the statement slice, so we splice the
	// retstat onto the end here so chunks like `return foo` actually emit
	// a Return opcode (otherwise loadfile/dofile/require chunks silently
	// produce no value).
	stmts := program.Block.Statements
	if program.Block.Return != nil {
		// Single allocation for the splice — the previous
		// append(append(nil, stmts...), ret) shape grew the slice twice.
		spliced := make([]ast.Statement, len(stmts)+1)
		copy(spliced, stmts)
		spliced[len(stmts)] = program.Block.Return
		stmts = spliced
	}
	chunks := g.GenerateInstructions(stmts)
	if err := g.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}
