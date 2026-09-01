package precedence

import "github.com/hilthontt/luascript/internal/compiler/token"

const (
	Lowest = iota
	Or
	And
	Compare
	BitOr
	BitXor
	BitAnd
	Shift
	Concat
	Sum
	Product
	Unary
	Pow
	Call
)

var LookupTable = map[token.Type]int{
	token.Or:  Or,
	token.And: And,

	token.LT:    Compare,
	token.GT:    Compare,
	token.LTE:   Compare,
	token.GTE:   Compare,
	token.Eq:    Compare,
	token.NotEq: Compare,

	token.Pipe:      BitOr,
	token.Tilde:     BitXor,
	token.Ampersand: BitAnd,
	token.LShift:    Shift,
	token.RShift:    Shift,

	token.Concat: Concat,

	token.Plus:     Sum,
	token.Minus:    Sum,
	token.Asterisk: Product,
	token.Slash:    Product,
	token.FloorDiv: Product,
	token.Percent:  Product,

	token.Caret: Pow,

	token.Dot:      Call,
	token.LBracket: Call,
	token.LParen:   Call,
	token.Colon:    Call,

	token.Label: Call,
}

func IsRightAssoc(t token.Type) bool {
	return t == token.Concat || t == token.Caret
}
