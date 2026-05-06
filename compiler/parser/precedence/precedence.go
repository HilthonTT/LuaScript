package precedence

import "github.com/hilthontt/sakura-lang/compiler/token"

// Constants for denoting precedence
const (
	_ = iota
	Lowest
	Normal
	Assign
	Logic
	Range
	Equals
	Compare
	Sum
	Product
	BangPrefix
	Index
	Call
	MinusPrefix
)

// LookupTable maps token to its corresponding precedence
var LookupTable = map[token.Type]int{
	token.Eq:       Equals,
	token.NotEq:    Equals,
	token.LT:       Compare,
	token.LTE:      Compare,
	token.GT:       Compare,
	token.GTE:      Compare,
	token.And:      Logic,
	token.Or:       Logic,
	token.Plus:     Sum,
	token.Minus:    Sum,
	token.Slash:    Product,
	token.Asterisk: Product,
	token.LBracket: Index,
	token.Dot:      Call,
	token.LParen:   Call,
	token.Assign:   Assign,
	token.Colon:    Assign,
}
