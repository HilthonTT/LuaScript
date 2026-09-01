package token

import "fmt"

type Type string

type Token struct {
	Type    Type
	Literal string
	Line    int
	Column  int
	Raw     string
}

const (
	Illegal Type = "ILLEGAL"
	EOF     Type = "EOF"

	Ident        Type = "IDENT"
	Int          Type = "INT"
	Float        Type = "FLOAT"
	String       Type = "STRING"
	InterpString Type = "INTERP_STRING"

	Plus     Type = "+"
	Minus    Type = "-"
	Asterisk Type = "*"
	Slash    Type = "/"
	Percent  Type = "%"
	Caret    Type = "^"
	FloorDiv Type = "//"

	Hash Type = "#"

	PlusAssign   Type = "+="
	MinusAssign  Type = "-="
	MulAssign    Type = "*="
	DivAssign    Type = "/="
	BorAssign    Type = "|="
	BandAssign   Type = "&="
	LShiftAssign Type = "<<="
	RShiftAssign Type = ">>="

	Ampersand Type = "&"
	Pipe      Type = "|"
	Tilde     Type = "~"
	LShift    Type = "<<"
	RShift    Type = ">>"

	Eq    Type = "=="
	NotEq Type = "~="
	LT    Type = "<"
	LTE   Type = "<="
	GT    Type = ">"
	GTE   Type = ">="

	Assign Type = "="

	Question Type = "?"
	Arrow    Type = "->"

	And Type = "AND"
	Or  Type = "OR"
	Not Type = "NOT"

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
	Enum     Type = "ENUM"
	Defer    Type = "DEFER"
	Try      Type = "TRY"
	Catch    Type = "CATCH"
	Throw    Type = "THROW"
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
	"enum":     Enum,
	"defer":    Defer,
	"try":      Try,
	"catch":    Catch,
	"throw":    Throw,
}

var operators = map[string]Type{
	"+":  Plus,
	"-":  Minus,
	"*":  Asterisk,
	"/":  Slash,
	"%":  Percent,
	"^":  Caret,
	"//": FloorDiv,
	"#":  Hash,

	"&":  Ampersand,
	"|":  Pipe,
	"~":  Tilde,
	"<<": LShift,
	">>": RShift,

	"==": Eq,
	"~=": NotEq,
	"<":  LT,
	"<=": LTE,
	">":  GT,
	">=": GTE,

	"=":   Assign,
	"+=":  PlusAssign,
	"-=":  MinusAssign,
	"*=":  MulAssign,
	"/=":  DivAssign,
	"|=":  BorAssign,
	"&=":  BandAssign,
	"<<=": LShiftAssign,
	">>=": RShiftAssign,

	"?":  Question,
	"->": Arrow,
}

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

func LookupIdent(ident string) Type {
	if t, ok := keywords[ident]; ok {
		return t
	}
	return Ident
}

func CreateOperator(literal string, line, column int) Token {
	t, ok := operators[literal]
	if !ok {
		panic(fmt.Sprintf("token.CreateOperator: %q is not a registered operator", literal))
	}
	return Token{Type: t, Literal: literal, Line: line, Column: column}
}

func CreateSeparator(literal string, line, column int) Token {
	t, ok := separators[literal]
	if !ok {
		panic(fmt.Sprintf("token.CreateSeparator: %q is not a registered separator", literal))
	}
	return Token{Type: t, Literal: literal, Line: line, Column: column}
}
