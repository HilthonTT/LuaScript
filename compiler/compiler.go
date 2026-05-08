package compiler

import (
	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/bytecode"
	"github.com/hilthontt/sakura-lang/compiler/lexer"
	"github.com/hilthontt/sakura-lang/compiler/parser"
)

func CompileToInstructions(input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	return CompileToInstructionsWith(bytecode.NewGenerator(), input, pm)
}

// CompileToInstructionsWith is the REPL-friendly variant: callers pass in a
// generator they own, allowing it to live across multiple compile calls. The
// generator's prior chunk output is cleared on entry; any other state the
// caller has stashed on it is preserved.
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
		stmts = append(append([]ast.Statement(nil), stmts...), program.Block.Return)
	}
	return g.GenerateInstructions(stmts), nil
}
