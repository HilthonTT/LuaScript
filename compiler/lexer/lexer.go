package lexer

import (
	"strings"

	"github.com/hilthontt/sakura-lang/compiler/token"
)

type Lexer struct {
	input        []rune
	position     int
	readPosition int
	ch           rune
	line         int

	// ModeDirective captures a Luau-style type-mode pragma found in a
	// leading comment of the file: "strict", "nonstrict", or "nocheck".
	// Empty string means none was set. First directive wins. Recognised
	// only in comments that appear before any non-comment token has been
	// produced (matches Luau's "file head" rule).
	ModeDirective string
	hasYielded    bool // set true once any non-EOF token has been returned
}

func New(input string) *Lexer {
	l := &Lexer{
		input: []rune(input),
		line:  1,
	}
	l.readChar()
	return l
}

func (l *Lexer) NextToken() token.Token {
	tok := l.nextToken()
	if tok.Type != token.EOF {
		l.hasYielded = true
	}
	return tok
}

func (l *Lexer) nextToken() token.Token {
	l.skipWhitespace()
	line := l.line

	switch l.ch {
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.PlusAssign, "+=", line)
		}

		return l.singleToken(token.Plus, "+")
	case '-':
		if l.peekChar() == '-' {
			l.absorbComment()
			return l.nextToken()
		}
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.MinusAssign, "-=", line)
		}
		if l.peekChar() == '>' {
			l.readChar()
			return l.makeToken(token.Arrow, "->", line)
		}

		return l.singleToken(token.Minus, "-")
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.MulAssign, "*=", line)
		}
		return l.singleToken(token.Asterisk, "*")
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.DivAssign, "/=", line)
		}
		if l.peekChar() == '/' {
			l.readChar()
			return l.makeToken(token.FloorDiv, "//", line)
		}
		return l.singleToken(token.Slash, "/")
	case '%':
		return l.singleToken(token.Percent, "%")
	case '^':
		return l.singleToken(token.Caret, "^")
	case '#':
		return l.singleToken(token.Hash, "#")
	case '&':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.AndAssign, "&=", line)
		}
		return l.singleToken(token.Ampersand, "&")
	case '|':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.OrAssign, "|=", line)
		}
		return l.singleToken(token.Pipe, "|")
	case '~':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.NotEq, "~=", line)
		}
		return l.singleToken(token.Tilde, "~")
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.LTE, "<=", line)
		}
		if l.peekChar() == '<' {
			l.readChar() // consume the second '<'
			if l.peekChar() == '=' {
				l.readChar() // consume '='
				return l.makeToken(token.LShiftAssign, "<<=", line)
			}
			return l.makeToken(token.LShift, "<<", line)
		}
		return l.singleToken(token.LT, "<")
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.GTE, ">=", line)
		}
		if l.peekChar() == '>' {
			l.readChar() // consume the second '>'
			if l.peekChar() == '=' {
				l.readChar() // consume '='
				return l.makeToken(token.RShiftAssign, ">>=", line)
			}
			return l.makeToken(token.RShift, ">>", line)
		}
		return l.singleToken(token.GT, ">")
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.Eq, "==", line)
		}
		return l.singleToken(token.Assign, "=")
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			if l.peekChar() == '.' {
				l.readChar()
				return l.makeToken(token.Vararg, "...", line)
			}
			return l.makeToken(token.Concat, "..", line)
		}
		if isDigit(l.peekChar()) {
			// .5 case - starts as float immediately
			return l.readDotFloat(line)
		}
		return l.singleToken(token.Dot, ".")
	case ':':
		if l.peekChar() == ':' {
			l.readChar()
			return l.makeToken(token.Label, "::", line)
		}
		return l.singleToken(token.Colon, ":")
	case '?':
		return l.singleToken(token.Question, "?")
	case ',':
		return l.singleToken(token.Comma, ",")
	case ';':
		return l.singleToken(token.Semicolon, ";")
	case '(':
		return l.singleToken(token.LParen, "(")
	case ')':
		return l.singleToken(token.RParen, ")")
	case '{':
		return l.singleToken(token.LBrace, "{")
	case '}':
		return l.singleToken(token.RBrace, "}")
	case ']':
		return l.singleToken(token.RBracket, "]")
	case '[':
		if l.peekChar() == '[' {
			l.readChar()
			l.readChar()
			lit := l.readLongString()
			return token.Token{Type: token.String, Literal: lit, Line: line}
		}
		return l.singleToken(token.LBracket, "[")
	case '"', '\'':
		lit := l.readString(l.ch)
		return token.Token{Type: token.String, Literal: lit, Line: line}
	case 0:
		return token.Token{Type: token.EOF, Literal: "", Line: line}
	default:
		if isLetter(l.ch) {
			lit := string(l.readIdentifier())
			typ := token.LookupIdent(lit)
			return token.Token{Type: typ, Literal: lit, Line: line}
		}
		if isDigit(l.ch) {
			return l.readNumberToken(line)
		}
		tok := token.Token{Type: token.Illegal, Literal: string(l.ch), Line: line}
		l.readChar()
		return tok
	}
}

// readNumberToken handles integers, floats, and hex. Called when l.ch is a
// digit; an integer becomes a float if a '.' is encountered mid-read.
func (l *Lexer) readNumberToken(line int) token.Token {
	// Hex: 0x...
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar()
		l.readChar()
		start := l.position
		for isHexDigit(l.ch) {
			l.readChar()
		}
		return token.Token{Type: token.Int, Literal: "0x" + string(l.input[start:l.position]), Line: line}
	}

	start := l.position
	for isDigit(l.ch) {
		l.readChar()
	}

	// Integer becomes float on '.'
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
		l.readExponent()
		return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line}
	}

	return token.Token{Type: token.Int, Literal: string(l.input[start:l.position]), Line: line}
}

// readDotFloat handles floats starting with '.' (e.g. .5). Called when l.ch == '.'.
func (l *Lexer) readDotFloat(line int) token.Token {
	start := l.position
	l.readChar() // consume '.'
	for isDigit(l.ch) {
		l.readChar()
	}
	l.readExponent()
	return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line}
}

// readExponent reads an optional e/E exponent from the current position.
func (l *Lexer) readExponent() {
	if l.ch == 'e' || l.ch == 'E' {
		l.readChar()
		if l.ch == '+' || l.ch == '-' {
			l.readChar()
		}
		for isDigit(l.ch) {
			l.readChar()
		}
	}
}

// absorbComment skips a Lua comment. Called when l.ch is on the first '-'
// and peekChar() == '-'. Handles both short and --[[ long comment styles.
//
// A Luau-style mode directive (`--!strict`, `--!nonstrict`, `--!nocheck`)
// appearing in a comment that comes BEFORE any real token has been emitted
// is captured into l.ModeDirective. First directive wins.
func (l *Lexer) absorbComment() {
	l.readChar() // consume second '-'
	l.readChar() // move past --

	if l.ch == '[' && l.peekChar() == '[' {
		l.readChar()
		l.readChar()
		l.readLongString()
		return
	}

	// Mode-directive recognition: only valid in leading comments before any
	// non-comment token has been produced. We scan to the end of the line
	// in any case, so directive parsing is purely a side effect.
	if !l.hasYielded && l.ch == '!' && l.ModeDirective == "" {
		l.readChar() // consume '!'
		start := l.position
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
		word := strings.TrimSpace(string(l.input[start:l.position]))
		switch word {
		case "strict", "nonstrict", "nocheck":
			l.ModeDirective = word
		}
		return
	}

	// Short comment: skip to end of line
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

// readLongString reads a [[...]] long string. Called after [[ has been consumed.
func (l *Lexer) readLongString() string {
	var b strings.Builder

	for {
		if l.ch == 0 {
			break
		}

		if l.ch == ']' && l.peekChar() == ']' {
			l.readChar()
			l.readChar()
			break
		}

		if l.ch == '\n' {
			l.line++
		}
		b.WriteRune(l.ch)
		l.readChar()
	}

	return b.String()
}

func (l *Lexer) readIdentifier() []rune {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString(ch rune) string {
	l.readChar() // skip opening quote

	if l.ch == ch {
		l.readChar()
		return ""
	}

	var b strings.Builder

	for {
		if isEscapedChar(l.ch) {
			b.WriteString(escapedCharResult(l.peekChar()))
			l.readChar()
		} else {
			b.WriteRune(l.ch)
		}
		l.readChar()
		if l.ch == ch || l.peekChar() == 0 {
			break
		}
	}

	l.readChar() // move past closing quote

	return b.String()
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
		if l.ch == '\n' {
			l.line++
		}
		l.readChar()
	}
}

func (l *Lexer) singleToken(t token.Type, lit string) token.Token {
	tok := token.Token{Type: t, Literal: lit, Line: l.line}
	l.readChar()
	return tok
}

func (l *Lexer) makeToken(t token.Type, lit string, line int) token.Token {
	tok := token.Token{Type: t, Literal: lit, Line: line}
	l.readChar()
	return tok
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		// ascii code's null
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}

	return l.input[l.readPosition]
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func isLetter(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ch == '_'
}

func isHexDigit(ch rune) bool {
	return isDigit(ch) || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
}

func isEscapedChar(ch rune) bool {
	return ch == '\\'
}

func escapedCharResult(peeked rune) string {
	switch peeked {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case 'v':
		return "\v"
	case 'f':
		return "\f"
	case '\\':
		return "\\"
	case '"':
		return "\""
	case '\'':
		return "'"
	default:
		return "\\" + string(peeked)
	}
}
