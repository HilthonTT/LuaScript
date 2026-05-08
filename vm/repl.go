package vm

import "github.com/hilthontt/sakura-lang/compiler/parser"

func (vm *VM) InitForREPL() {
	vm.mode = parser.REPLMode
}
