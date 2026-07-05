// Package precedence encodes the Lua 5.4 binary-operator precedence ladder
// (see Lua 5.4 reference manual §3.4.8). Levels run from Lowest (a sentinel
// used as the entrypoint floor) to Call (postfix suffixes that bind tighter
// than any binary operator).
//
//	or                                                     -- Or
//	and                                                    -- And
//	<   >   <=   >=   ==   ~=                              -- Compare
//	|                                                      -- BitOr
//	~  (binary xor)                                        -- BitXor
//	&                                                      -- BitAnd
//	<<  >>                                                 -- Shift
//	..                          (right-associative)        -- Concat
//	+   -                                                  -- Sum
//	*   /   //   %                                         -- Product
//	unary  not  #  -  ~                                    -- Unary
//	^                           (right-associative)        -- Pow
//	postfix  .  [  (  :  {  string-call                    -- Call
package precedence

import "github.com/hilthontt/luascript/internal/compiler/token"

// Precedence levels, low → high. Right-associativity is *not* encoded here;
// callers handle it at parse time by recursing with Level-1 on the RHS.
const (
	Lowest  = iota // sentinel: any real precedence is greater
	Or             // or
	And            // and
	Compare        // < > <= >= == ~=
	BitOr          // |
	BitXor         // ~ (binary)
	BitAnd         // &
	Shift          // << >>
	Concat         // ..   (right-assoc)
	Sum            // + -
	Product        // * / // %
	Unary          // not  #  -  ~
	Pow            // ^   (right-assoc)
	Call           // postfix: .  [  (  :  {  string
)

// LookupTable maps a binary or postfix token to its precedence level. The
// parser consults this for the standard Pratt loop. Unary precedence is not
// listed because unary operators are handled in the prefix branch.
var LookupTable = map[token.Type]int{
	// Logical
	token.Or:  Or,
	token.And: And,

	// Comparison
	token.LT:    Compare,
	token.GT:    Compare,
	token.LTE:   Compare,
	token.GTE:   Compare,
	token.Eq:    Compare,
	token.NotEq: Compare,

	// Bitwise
	token.Pipe:      BitOr,
	token.Tilde:     BitXor, // binary `~` (xor) — unary `~` is a prefix
	token.Ampersand: BitAnd,
	token.LShift:    Shift,
	token.RShift:    Shift,

	// String concatenation
	token.Concat: Concat,

	// Arithmetic
	token.Plus:     Sum,
	token.Minus:    Sum, // binary; unary `-` is a prefix
	token.Asterisk: Product,
	token.Slash:    Product,
	token.FloorDiv: Product,
	token.Percent:  Product,

	// Exponent
	token.Caret: Pow,

	// Postfix suffixes — `{` and STRING also start a call but they are
	// handled by direct lookup at the parse site (no infix function).
	token.Dot:      Call,
	token.LBracket: Call,
	token.LParen:   Call,
	token.Colon:    Call,

	// Type assertion `expr :: T` is a postfix that binds at Call level so
	// `a + b :: T` parses as `a + (b :: T)`, matching Luau. The same `::`
	// token represents goto labels in statement position; the parser
	// disambiguates by context (label statements never enter expression
	// parsing).
	token.Label: Call,
}

// IsRightAssoc reports whether the operator at this token type associates
// to the right (`..` and `^`). The parser handles right-associativity by
// recursing on the RHS with `prec - 1`.
func IsRightAssoc(t token.Type) bool {
	return t == token.Concat || t == token.Caret
}
