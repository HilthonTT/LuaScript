package parser

import "github.com/hilthontt/luascript/internal/compiler/token"

// compoundOps maps a compound-assignment token to the binary operator
// string used by ast.BinaryExpression. We desugar `x op= e` into
// `x = x op e` at parse time, so the bytecode generator never needs to
// know about compound forms.
var compoundOps = map[token.Type]string{
	token.PlusAssign:   "+",
	token.MinusAssign:  "-",
	token.MulAssign:    "*",
	token.DivAssign:    "/",
	token.BandAssign:   "&",
	token.BorAssign:    "|",
	token.LShiftAssign: "<<",
	token.RShiftAssign: ">>",
}
