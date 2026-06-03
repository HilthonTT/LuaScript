package vm

import "github.com/hilthontt/luascript/compiler/parser"

func (vm *VM) InitForREPL() {
	vm.mode = parser.REPLMode
}
