package compiler

import (
	"fmt"

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
	return g.GenerateInstructions(program.Block.Statements), nil
}
