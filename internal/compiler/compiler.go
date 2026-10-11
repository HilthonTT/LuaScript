package compiler

import (
	"github.com/hilthontt/luascript/v3/internal/compiler/ast"
	"github.com/hilthontt/luascript/v3/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/v3/internal/compiler/constcheck"
	"github.com/hilthontt/luascript/v3/internal/compiler/lexer"
	"github.com/hilthontt/luascript/v3/internal/compiler/optimize"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
	"github.com/hilthontt/luascript/v3/internal/compiler/typecheck"
)

func CompileToInstructions(input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	return CompileToInstructionsWith(bytecode.NewGenerator(), input, pm)
}

func CompileToInstructionsWith(g *bytecode.Generator, input string, pm parser.Mode) ([]*bytecode.InstructionSet, error) {
	l := lexer.New(input)
	p := parser.New(l)
	p.Mode = pm
	g.REPL = pm == parser.REPLMode

	program, err := p.ParseProgram()
	if err != nil {
		return nil, err
	}

	if err := constcheck.Check(program); err != nil {
		return nil, err
	}

	if l.ModeDirective != "nocheck" {
		opts := typecheck.Options{
			Strict: l.ModeDirective == "strict",
			REPL:   pm == parser.REPLMode,
		}
		if errs := typecheck.Check(program, opts); len(errs) > 0 {
			return nil, &typecheck.TypeErrors{Errors: errs}
		}
	}

	optimize.Fold(program)

	g.ResetInstructionSets()
	g.InitTopLevelScope(program)

	stmts := program.Block.Statements
	if program.Block.Return != nil {
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
