package token

// Type is used to determine token type
type Type string

// Token is a structure for identifying input stream of characters
type Token struct {
	Type    Type
	Literal string
	Line    int
}

const (
	Illegal = "ILLEGAL"
	EOF     = "EOF"

	// Literals
	Ident  = "IDENT"
	Int    = "INT"
	Float  = "FLOAT"
	String = "STRING"

	// Arithmetic
	Plus     = "+"
	Minus    = "-"
	Asterisk = "*"
	Slash    = "/"
	Percent  = "%"
	Caret    = "^"
	FloorDiv = "//"
	Hash     = "#"

	// Compound assignement
	PlusAssign   = "+="
	MinusAssign  = "-="
	MulAssign    = "*="
	DivAssign    = "/="
	OrAssign     = "|="
	AndAssign    = "&="
	LShiftAssign = "<<="
	RShiftAssign = ">>="

	// Bitwise
	Ampersand = "&"
	Pipe      = "|"
	Tilde     = "~"
	LShift    = "<<"
	RShift    = ">>"

	// Comparison
	Eq    = "=="
	NotEq = "~="
	LT    = "<"
	LTE   = "<="
	GT    = ">"
	GTE   = ">="

	// Assignment
	Assign = "="

	// Type-syntax operators (Luau-style annotations).
	// `Question` is the postfix-optional sugar (`T?` ≡ `T | nil`).
	// `Arrow` separates a function type's params from its return (`(A) -> B`).
	// `::` is already represented by Label and is reused for type assertions
	// (`expr :: T`); the parser disambiguates by context.
	Question = "?"
	Arrow    = "->"

	// Logical (keywords in Lua, but typed for AST use)
	And = "AND"
	Or  = "OR"
	Not = "NOT"

	// Delimiters
	Comma     = ","
	Semicolon = ";"
	Colon     = ":"
	Dot       = "."
	Concat    = ".."
	Vararg    = "..."
	Label     = "::"

	LParen   = "("
	RParen   = ")"
	LBrace   = "{"
	RBrace   = "}"
	LBracket = "["
	RBracket = "]"

	// Keywords
	True     = "TRUE"
	False    = "FALSE"
	Nil      = "NIL"
	If       = "IF"
	ElseIf   = "ELSEIF"
	Else     = "ELSE"
	Then     = "THEN"
	End      = "END"
	Do       = "DO"
	While    = "WHILE"
	Repeat   = "REPEAT"
	Until    = "UNTIL"
	For      = "FOR"
	In       = "IN"
	Function = "FUNCTION"
	Local    = "LOCAL"
	Return   = "RETURN"
	Break    = "BREAK"
	Goto     = "GOTO"
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
}

// Unambiguous single-character operators only.
// Multi-character operators sharing a prefix (==/=, ~=/~, //slash, <</>>, <=/>= etc.)
// must be handled with lookahead in the lexer switch.
var operators = map[string]Type{
	"+": Plus,
	"-": Minus,
	"*": Asterisk,
	"/": Slash,
	"%": Percent,
	"^": Caret,
	"#": Hash,
	"&": Ampersand,
	"|": Pipe,
	"<": LT,
	">": GT,
	"=": Assign,
}

var separators = map[string]Type{
	",": Comma,
	";": Semicolon,
	":": Colon,
	"(": LParen,
	")": RParen,
	"{": LBrace,
	"}": RBrace,
	"[": LBracket,
	"]": RBracket,
}

// LookupIdent checks if an identifier is a reserved keyword.
func LookupIdent(ident string) Type {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return Ident
}

func getOperatorType(literal string) Type {
	if t, ok := operators[literal]; ok {
		return t
	}
	return Illegal
}

func getSeparatorType(literal string) Type {
	if t, ok := separators[literal]; ok {
		return t
	}
	return Illegal
}

// CreateOperator is a factory method for creating operator tokens.
func CreateOperator(literal string, line int) Token {
	return Token{Type: getOperatorType(literal), Literal: literal, Line: line}
}

// CreateSeparator is a factory method for creating separator tokens.
func CreateSeparator(literal string, line int) Token {
	return Token{Type: getSeparatorType(literal), Literal: literal, Line: line}
}
