package compiler

import (
	"fmt"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/bytecode"
	"github.com/hilthontt/sakura-lang/compiler/lexer"
	"github.com/hilthontt/sakura-lang/compiler/parser"
)

func CompileToInstructions(input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	l := lexer.New(input)
	p := parser.New(l)
	p.Mode = pm

	program, err := p.ParseProgram()
	if err != nil {
		return nil, fmt.Errorf("%s", err.Message)
	}
	g := bytecode.NewGenerator()
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
