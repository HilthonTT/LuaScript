package lexer

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

type expectedToken struct {
	expectedType    token.Type
	expectedLiteral string
}

func testLex(t *testing.T, input string, expected []expectedToken) {
	t.Helper()
	l := New(input)
	for i, tt := range expected {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("[token %d] wrong type: want %q, got %q (literal=%q)",
				i, tt.expectedType, tok.Type, tok.Literal)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("[token %d] wrong literal: want %q, got %q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestKeywords(t *testing.T) {
	input := `and break do else elseif end false for function goto
	          if in local nil not or repeat return then true until while`

	expected := []expectedToken{
		{token.And, "and"},
		{token.Break, "break"},
		{token.Do, "do"},
		{token.Else, "else"},
		{token.ElseIf, "elseif"},
		{token.End, "end"},
		{token.False, "false"},
		{token.For, "for"},
		{token.Function, "function"},
		{token.Goto, "goto"},
		{token.If, "if"},
		{token.In, "in"},
		{token.Local, "local"},
		{token.Nil, "nil"},
		{token.Not, "not"},
		{token.Or, "or"},
		{token.Repeat, "repeat"},
		{token.Return, "return"},
		{token.Then, "then"},
		{token.True, "true"},
		{token.Until, "until"},
		{token.While, "while"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestIdentifiers(t *testing.T) {
	input := `foo bar _private myVar123 _`
	expected := []expectedToken{
		{token.Ident, "foo"},
		{token.Ident, "bar"},
		{token.Ident, "_private"},
		{token.Ident, "myVar123"},
		{token.Ident, "_"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestKeywordAsPrefix(t *testing.T) {
	input := `endif whileLoop notTrue`
	expected := []expectedToken{
		{token.Ident, "endif"},
		{token.Ident, "whileLoop"},
		{token.Ident, "notTrue"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestIntegers(t *testing.T) {
	input := `0 1 42 1000`
	expected := []expectedToken{
		{token.Int, "0"},
		{token.Int, "1"},
		{token.Int, "42"},
		{token.Int, "1000"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestFloats(t *testing.T) {
	input := `3.14 0.5 .5 1.0 1.5e10 2.0E-3`
	expected := []expectedToken{
		{token.Float, "3.14"},
		{token.Float, "0.5"},
		{token.Float, ".5"},
		{token.Float, "1.0"},
		{token.Float, "1.5e10"},
		{token.Float, "2.0E-3"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestHexIntegers(t *testing.T) {
	input := `0xff 0xFF 0x0 0xDEAD`
	expected := []expectedToken{
		{token.Int, "0xff"},
		{token.Int, "0xFF"},
		{token.Int, "0x0"},
		{token.Int, "0xDEAD"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestDoubleQuotedStrings(t *testing.T) {
	input := `"hello" "world" ""`
	expected := []expectedToken{
		{token.String, "hello"},
		{token.String, "world"},
		{token.String, ""},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestSingleQuotedStrings(t *testing.T) {
	input := `'hello' 'it''s' ''`
	expected := []expectedToken{
		{token.String, "hello"},
		{token.String, "it"},
		{token.String, "s"},
		{token.String, ""},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestStringEscapes(t *testing.T) {
	input := `"\n\t\r\\\""`
	expected := []expectedToken{
		{token.String, "\n\t\r\\\""},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestLongStrings(t *testing.T) {
	input := `[[hello world]] [[line1
line2]]`
	expected := []expectedToken{
		{token.String, "hello world"},
		{token.String, "line1\nline2"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestArithmeticOperators(t *testing.T) {
	input := `+ - * / % ^ // #`
	expected := []expectedToken{
		{token.Plus, "+"},
		{token.Minus, "-"},
		{token.Asterisk, "*"},
		{token.Slash, "/"},
		{token.Percent, "%"},
		{token.Caret, "^"},
		{token.FloorDiv, "//"},
		{token.Hash, "#"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestBitwiseOperators(t *testing.T) {
	input := `& | ~ << >>`
	expected := []expectedToken{
		{token.Ampersand, "&"},
		{token.Pipe, "|"},
		{token.Tilde, "~"},
		{token.LShift, "<<"},
		{token.RShift, ">>"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestComparisonOperators(t *testing.T) {
	input := `== ~= < <= > >=`
	expected := []expectedToken{
		{token.Eq, "=="},
		{token.NotEq, "~="},
		{token.LT, "<"},
		{token.LTE, "<="},
		{token.GT, ">"},
		{token.GTE, ">="},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestTildeDisambiguation(t *testing.T) {
	input := `~ ~=`
	expected := []expectedToken{
		{token.Tilde, "~"},
		{token.NotEq, "~="},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestDotConcatVararg(t *testing.T) {
	input := `. .. ...`
	expected := []expectedToken{
		{token.Dot, "."},
		{token.Concat, ".."},
		{token.Vararg, "..."},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestColonLabel(t *testing.T) {
	input := `: ::`
	expected := []expectedToken{
		{token.Colon, ":"},
		{token.Label, "::"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestLineComment(t *testing.T) {
	input := `x -- this is a comment
y`
	expected := []expectedToken{
		{token.Ident, "x"},
		{token.Ident, "y"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestLongComment(t *testing.T) {
	input := `x --[[
	  this is a
	  multiline comment
	]] y`
	expected := []expectedToken{
		{token.Ident, "x"},
		{token.Ident, "y"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestLineNumbers(t *testing.T) {
	input := "x\ny\nz"
	l := New(input)

	tok := l.NextToken()
	if tok.Line != 1 {
		t.Fatalf("want line 1, got %d", tok.Line)
	}
	tok = l.NextToken()
	if tok.Line != 2 {
		t.Fatalf("want line 2, got %d", tok.Line)
	}
	tok = l.NextToken()
	if tok.Line != 3 {
		t.Fatalf("want line 3, got %d", tok.Line)
	}
}

func TestLineNumberAfterLongString(t *testing.T) {
	input := "x\n[[line1\nline2]]\ny"
	l := New(input)

	l.NextToken()
	l.NextToken()
	tok := l.NextToken()
	if tok.Line != 4 {
		t.Fatalf("want line 4, got %d", tok.Line)
	}
}

func TestSingleTokenInputsThenEOF(t *testing.T) {
	inputs := []string{"foo", "42", "3.14", `"str"`, "[[x]]"}
	for _, input := range inputs {
		l := New(input)
		l.NextToken()
		if tok := l.NextToken(); tok.Type != token.EOF {
			t.Errorf("input %q: expected EOF after the single token, got %q", input, tok.Type)
		}
	}
}

func TestIntToFloatLexing(t *testing.T) {
	l := New("1.5")
	tok := l.NextToken()
	if tok.Type != token.Float {
		t.Fatalf("expected Float token, got %q", tok.Type)
	}
	if tok.Literal != "1.5" {
		t.Fatalf("expected literal 1.5, got %q", tok.Literal)
	}
}

func TestLongCommentAbsorbed(t *testing.T) {
	l := New("--[[ comment ]] x")
	tok := l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "x" {
		t.Fatalf("expected Ident 'x' after long comment, got %q %q", tok.Type, tok.Literal)
	}
	if tok := l.NextToken(); tok.Type != token.EOF {
		t.Fatalf("expected EOF, got %q", tok.Type)
	}
}

func TestFunctionDeclaration(t *testing.T) {
	input := `function add(a, b)
	return a + b
end`
	expected := []expectedToken{
		{token.Function, "function"},
		{token.Ident, "add"},
		{token.LParen, "("},
		{token.Ident, "a"},
		{token.Comma, ","},
		{token.Ident, "b"},
		{token.RParen, ")"},
		{token.Return, "return"},
		{token.Ident, "a"},
		{token.Plus, "+"},
		{token.Ident, "b"},
		{token.End, "end"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestForLoop(t *testing.T) {
	input := `for i = 1, 10 do
	print(i)
end`
	expected := []expectedToken{
		{token.For, "for"},
		{token.Ident, "i"},
		{token.Assign, "="},
		{token.Int, "1"},
		{token.Comma, ","},
		{token.Int, "10"},
		{token.Do, "do"},
		{token.Ident, "print"},
		{token.LParen, "("},
		{token.Ident, "i"},
		{token.RParen, ")"},
		{token.End, "end"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestTableConstructor(t *testing.T) {
	input := `t = { x = 1, y = 2.5 }`
	expected := []expectedToken{
		{token.Ident, "t"},
		{token.Assign, "="},
		{token.LBrace, "{"},
		{token.Ident, "x"},
		{token.Assign, "="},
		{token.Int, "1"},
		{token.Comma, ","},
		{token.Ident, "y"},
		{token.Assign, "="},
		{token.Float, "2.5"},
		{token.RBrace, "}"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestGotoLabel(t *testing.T) {
	input := `::myLabel:: goto myLabel`
	expected := []expectedToken{
		{token.Label, "::"},
		{token.Ident, "myLabel"},
		{token.Label, "::"},
		{token.Goto, "goto"},
		{token.Ident, "myLabel"},
		{token.EOF, ""},
	}
	testLex(t, input, expected)
}

func TestIllegalCharacter(t *testing.T) {
	input := `@`
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.Illegal {
		t.Fatalf("expected Illegal, got %q", tok.Type)
	}
	if tok.Literal != "@" {
		t.Fatalf("expected literal '@', got %q", tok.Literal)
	}
}

func TestEmptyInput(t *testing.T) {
	l := New("")
	tok := l.NextToken()
	if tok.Type != token.EOF {
		t.Fatalf("expected EOF on empty input, got %q", tok.Type)
	}
}
