package repl

import (
	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/bytecode"
	"github.com/hilthontt/sakura-lang/compiler/parser"
	"github.com/hilthontt/sakura-lang/vm"
)

type engine struct {
	vm  *vm.VM
	gen *bytecode.Generator
}

func newEngine(v *vm.VM) *engine {
	return &engine{vm: v, gen: bytecode.NewGenerator()}
}

func (e *engine) compile(src string) ([]*bytecode.InstructionSet, error) {
	return compiler.CompileToInstructionsWith(e.gen, src, parser.REPLMode)
}

func (e *engine) runMain(chunk *bytecode.InstructionSet) error {
	return e.vm.Run(chunk)
}

func (e *engine) runMainWithResults(chunk *bytecode.InstructionSet) ([]vm.Value, error) {
	return e.vm.RunMainChunkWithResults(chunk)
}
