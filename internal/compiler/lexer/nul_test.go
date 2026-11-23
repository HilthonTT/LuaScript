package lexer

import (
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler/token"
)

var nul = string(rune(0))

// A NUL byte used to read as end of input, so everything after it was
// silently dropped from the program.
func TestNulByteIsNotEOF(t *testing.T) {
	l := New("x" + nul + "y")
	if tok := l.NextToken(); tok.Type != token.Ident || tok.Literal != "x" {
		t.Fatalf("first token = %v %q", tok.Type, tok.Literal)
	}
	if tok := l.NextToken(); tok.Type != token.Illegal {
		t.Fatalf("NUL lexed as %v %q, want Illegal", tok.Type, tok.Literal)
	}
	if tok := l.NextToken(); tok.Type != token.Ident || tok.Literal != "y" {
		t.Fatalf("token after NUL = %v %q, want identifier y", tok.Type, tok.Literal)
	}
}

func TestNulByteInsideStrings(t *testing.T) {
	for _, src := range []string{`"a` + nul + `b"`, "[[a" + nul + "b]]"} {
		tok := New(src).NextToken()
		if tok.Type != token.String || tok.Literal != "a"+nul+"b" {
			t.Errorf("%q lexed as %v %q", src, tok.Type, tok.Literal)
		}
	}
}

func TestNulByteInsideComment(t *testing.T) {
	l := New("-- a" + nul + "b\nx")
	if tok := l.NextToken(); tok.Type != token.Ident || tok.Literal != "x" {
		t.Fatalf("token after comment = %v %q, want identifier x", tok.Type, tok.Literal)
	}
}
