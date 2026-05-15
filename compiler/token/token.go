package token

import "fmt"

// Type is the discriminator for a Token.
type Type string

// Token is a lexed lexeme tagged with its type and source location.
type Token struct {
	Type    Type
	Literal string
	Line    int
	Column  int
}

const (
	Illegal Type = "ILLEGAL"
	EOF     Type = "EOF"

	// Literals
	Ident  Type = "IDENT"
	Int    Type = "INT"
	Float  Type = "FLOAT"
	String Type = "STRING"

	// Arithmetic
	Plus     Type = "+"
	Minus    Type = "-"
	Asterisk Type = "*"
	Slash    Type = "/"
	Percent  Type = "%"
	Caret    Type = "^"
	FloorDiv Type = "//"

	// Unary-only operators.
	// `Hash` is length-of (`#t`). `Not` is logical not. `Tilde` is also a
	// unary operator (bitwise not) but lives in the bitwise section because
	// it doubles as binary XOR.
	Hash Type = "#"

	// Compound assignment.
	// `BorAssign` and `BandAssign` are *bitwise* OR/AND assign (`|=`, `&=`),
	// not logical — Lua's logical `or`/`and` have no compound form.
	PlusAssign   Type = "+="
	MinusAssign  Type = "-="
	MulAssign    Type = "*="
	DivAssign    Type = "/="
	BorAssign    Type = "|="
	BandAssign   Type = "&="
	LShiftAssign Type = "<<="
	RShiftAssign Type = ">>="

	// Bitwise
	Ampersand Type = "&"
	Pipe      Type = "|"
	Tilde     Type = "~"
	LShift    Type = "<<"
	RShift    Type = ">>"

	// Comparison
	Eq    Type = "=="
	NotEq Type = "~="
	LT    Type = "<"
	LTE   Type = "<="
	GT    Type = ">"
	GTE   Type = ">="

	// Assignment
	Assign Type = "="

	// Type-syntax operators (Luau-style annotations).
	// `Question` is the postfix-optional sugar (`T?` ≡ `T | nil`).
	// `Arrow` separates a function type's params from its return (`(A) -> B`)
	// and also acts as the arm separator in `match` expressions; the parser
	// disambiguates by context.
	// `::` is represented by Label and is reused for type assertions
	// (`expr :: T`); again the parser disambiguates by context.
	Question Type = "?"
	Arrow    Type = "->"

	// Logical (keywords in Lua, but typed for AST use)
	And Type = "AND"
	Or  Type = "OR"
	Not Type = "NOT"

	// Delimiters
	Comma     Type = ","
	Semicolon Type = ";"
	Colon     Type = ":"
	Dot       Type = "."
	Concat    Type = ".."
	Vararg    Type = "..."
	Label     Type = "::"

	LParen   Type = "("
	RParen   Type = ")"
	LBrace   Type = "{"
	RBrace   Type = "}"
	LBracket Type = "["
	RBracket Type = "]"

	// Keywords
	True     Type = "TRUE"
	False    Type = "FALSE"
	Nil      Type = "NIL"
	If       Type = "IF"
	ElseIf   Type = "ELSEIF"
	Else     Type = "ELSE"
	Then     Type = "THEN"
	End      Type = "END"
	Do       Type = "DO"
	While    Type = "WHILE"
	Repeat   Type = "REPEAT"
	Until    Type = "UNTIL"
	For      Type = "FOR"
	In       Type = "IN"
	Function Type = "FUNCTION"
	Local    Type = "LOCAL"
	Return   Type = "RETURN"
	Break    Type = "BREAK"
	Goto     Type = "GOTO"
	Match    Type = "MATCH"
)

var keywords = map[string]Type{
	"and":      And,
	"break":    Break,
	"do":       Do,
	"else":     Else,
	"elseif":   ElseIf,
	"end":      End,
	"false":    False,
	"for":      For,
	"function": Function,
	"goto":     Goto,
	"if":       If,
	"in":       In,
	"local":    Local,
	"nil":      Nil,
	"not":      Not,
	"or":       Or,
	"repeat":   Repeat,
	"return":   Return,
	"then":     Then,
	"true":     True,
	"until":    Until,
	"while":    While,
	"match":    Match,
}

// operators maps every operator literal — single- and multi-character — to
// its token type. The lexer is responsible for doing its own lookahead to
// determine the full literal (`==` vs `=`, `<<=` vs `<<` vs `<`, `~=` vs `~`,
// `//` vs `/`, etc.), then this map gives the type for whatever it committed
// to. Structural punctuation (parens, braces, commas, dots, colons, labels)
// lives in `separators` instead.
var operators = map[string]Type{
	// Arithmetic
	"+":  Plus,
	"-":  Minus,
	"*":  Asterisk,
	"/":  Slash,
	"%":  Percent,
	"^":  Caret,
	"//": FloorDiv,
	"#":  Hash,

	// Bitwise
	"&":  Ampersand,
	"|":  Pipe,
	"~":  Tilde,
	"<<": LShift,
	">>": RShift,

	// Comparison
	"==": Eq,
	"~=": NotEq,
	"<":  LT,
	"<=": LTE,
	">":  GT,
	">=": GTE,

	// Assignment + compound assignment
	"=":   Assign,
	"+=":  PlusAssign,
	"-=":  MinusAssign,
	"*=":  MulAssign,
	"/=":  DivAssign,
	"|=":  BorAssign,
	"&=":  BandAssign,
	"<<=": LShiftAssign,
	">>=": RShiftAssign,

	// Type syntax / match-arm
	"?":  Question,
	"->": Arrow,
}

// separators maps structural punctuation literals to their token type. As
// with operators, the lexer is responsible for lookahead to disambiguate
// dotted forms (`.`, `..`, `...`) and `:` vs `::`.
var separators = map[string]Type{
	",":   Comma,
	";":   Semicolon,
	":":   Colon,
	".":   Dot,
	"..":  Concat,
	"...": Vararg,
	"::":  Label,
	"(":   LParen,
	")":   RParen,
	"{":   LBrace,
	"}":   RBrace,
	"[":   LBracket,
	"]":   RBracket,
}

// LookupIdent returns the keyword type for ident, or Ident if it is not a
// reserved word.
func LookupIdent(ident string) Type {
	if t, ok := keywords[ident]; ok {
		return t
	}
	return Ident
}

// CreateOperator builds an operator token. Panics if literal is not a known
// operator — that is a lexer bug, not user input, and silently producing an
// Illegal token would hide the cause until it surfaced in the parser.
func CreateOperator(literal string, line, column int) Token {
	t, ok := operators[literal]
	if !ok {
		panic(fmt.Sprintf("token.CreateOperator: %q is not a registered operator", literal))
	}
	return Token{Type: t, Literal: literal, Line: line, Column: column}
}

// CreateSeparator builds a separator token, with the same panic-on-unknown
// contract as CreateOperator.
func CreateSeparator(literal string, line, column int) Token {
	t, ok := separators[literal]
	if !ok {
		panic(fmt.Sprintf("token.CreateSeparator: %q is not a registered separator", literal))
	}
	return Token{Type: t, Literal: literal, Line: line, Column: column}
}
