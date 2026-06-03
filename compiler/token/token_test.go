package token

import "testing"

func TestLookupIdentFal(t *testing.T) {
	token := LookupIdent("nonexist")
	if token != Ident {
		t.Fatalf("Expect %s got %s", Ident, token)
	}
}

func TestLookupIdentTrue(t *testing.T) {
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

	for name, token := range keywords {
		test := LookupIdent(name)
		if test != token {
			t.Fatalf("Expect %s got %s", token, test)
		}
	}
}

func TestCreateOperatorIdentTrue(t *testing.T) {
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

	line := 123
	col := 1

	for name, tokenType := range operators {
		tok := CreateOperator(name, line, col)
		if tok.Type != tokenType {
			t.Fatalf("Expect token type %s, got: %s", tokenType, tok.Type)
		}
		if tok.Line != line {
			t.Fatalf("Expect token line %v, got: %v", line, tok.Line)
		}
	}
}

func TestCreateSeparatorIdentTrue(t *testing.T) {
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

	line := 123
	col := 1

	for name, tokenType := range separators {
		tok := CreateSeparator(name, line, col)
		if tok.Type != tokenType {
			t.Fatalf("Expect token type %s, got: %s", tokenType, tok.Type)
		}
		if tok.Line != line {
			t.Fatalf("Expect token line %v, got: %v", line, tok.Line)
		}
	}
}
