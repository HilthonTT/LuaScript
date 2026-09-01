package parser

import "github.com/hilthontt/luascript/internal/compiler/token"

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
