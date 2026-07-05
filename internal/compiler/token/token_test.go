package token

import "testing"

func TestLookupIdentFal(t *testing.T) {
	token := LookupIdent("nonexist")
	if token != Ident {
		t.Fatalf("Expect %s got %s", Ident, token)
	}
}

func TestLookupIdentTrue(t *testing.T) {
	for name, token := range keywords {
		test := LookupIdent(name)
		if test != token {
			t.Fatalf("Expect %s got %s", token, test)
		}
	}
}

func TestCreateOperatorIdentTrue(t *testing.T) {
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
