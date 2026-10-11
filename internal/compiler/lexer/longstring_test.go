package lexer

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

func TestLongStringDropsLeadingNewline(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"lf", "[[\nhello]]", "hello"},
		{"crlf", "[[\r\nhello]]", "hello"},
		{"cr", "[[\rhello]]", "hello"},
		{"lfcr", "[[\n\rhello]]", "hello"},
		{"leveled", "[==[\nhello]==]", "hello"},
		{"only first newline", "[[\n\nhello]]", "\nhello"},
		{"crlf body normalised", "[[\r\na\r\nb\r\n]]", "a\nb\n"},
		{"no leading newline", "[[ hello]]", " hello"},
		{"empty after newline", "[[\n]]", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := New(tc.input).NextToken()
			if tok.Type != token.String || tok.Literal != tc.want {
				t.Fatalf("got %v %q, want String %q", tok.Type, tok.Literal, tc.want)
			}
		})
	}
}

func TestLongStringLineCountingCRLF(t *testing.T) {
	l := New("x\r\n[[\r\nline1\r\nline2]]\r\ny")
	l.NextToken()
	l.NextToken()
	if tok := l.NextToken(); tok.Line != 5 {
		t.Fatalf("want line 5, got %d", tok.Line)
	}
}
