package lexer

import (
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

type Lexer struct {
	input        []rune
	position     int
	readPosition int
	ch           rune
	line         int
	column       int
	tokenCol     int

	ModeDirective string
	hasYielded    bool
}

func New(input string) *Lexer {
	runes := []rune(input)
	if len(runes) > 0 && runes[0] == 0xFEFF {
		runes = runes[1:]
	}
	l := &Lexer{
		input: runes,
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
	l.tokenCol = l.column

	switch l.ch {
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.PlusAssign, "+=", line)
		}

		return l.singleToken(token.Plus, "+")
	case '-':
		if l.peekChar() == '-' {
			if errTok, ok := l.absorbComment(line); !ok {
				return errTok
			}
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
			return l.makeToken(token.BandAssign, "&=", line)
		}
		return l.singleToken(token.Ampersand, "&")
	case '|':
		if l.peekChar() == '=' {
			l.readChar()
			return l.makeToken(token.BorAssign, "|=", line)
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
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
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
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
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
		if lvl := l.longOpenLevel(); lvl >= 0 {
			l.consumeLongOpen(lvl)
			lit, ok := l.readLongString(lvl)
			if !ok {
				return token.Token{Type: token.Illegal, Literal: "unfinished long string", Line: line, Column: l.tokenCol}
			}
			return token.Token{Type: token.String, Literal: lit, Line: line, Column: l.tokenCol}
		}
		return l.singleToken(token.LBracket, "[")
	case '"', '\'', '`':
		quote := l.ch
		lit, raw, ok := l.readString(quote)
		if !ok {
			return token.Token{Type: token.Illegal, Literal: "unfinished string", Line: line, Column: l.tokenCol}
		}
		if quote == '`' {
			return token.Token{Type: token.InterpString, Literal: lit, Raw: raw, Line: line, Column: l.tokenCol}
		}
		return token.Token{Type: token.String, Literal: lit, Line: line, Column: l.tokenCol}
	case 0:
		return token.Token{Type: token.EOF, Literal: "", Line: line, Column: l.tokenCol}
	default:
		if isLetter(l.ch) {
			lit := string(l.readIdentifier())
			typ := token.LookupIdent(lit)
			return token.Token{Type: typ, Literal: lit, Line: line, Column: l.tokenCol}
		}
		if isDigit(l.ch) {
			return l.readNumberToken(line)
		}
		tok := token.Token{Type: token.Illegal, Literal: string(l.ch), Line: line, Column: l.tokenCol}
		l.readChar()
		return tok
	}
}

func (l *Lexer) readNumberToken(line int) token.Token {
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		start := l.position
		l.readChar()
		l.readChar()
		for isHexDigit(l.ch) {
			l.readChar()
		}
		isFloat := false
		if l.ch == '.' {
			isFloat = true
			l.readChar()
			for isHexDigit(l.ch) {
				l.readChar()
			}
		}
		if l.ch == 'p' || l.ch == 'P' {
			isFloat = true
			l.readChar()
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			for isDigit(l.ch) {
				l.readChar()
			}
		}
		lit := string(l.input[start:l.position])
		if isFloat {
			return token.Token{Type: token.Float, Literal: lit, Line: line, Column: l.tokenCol}
		}
		return token.Token{Type: token.Int, Literal: lit, Line: line, Column: l.tokenCol}
	}

	start := l.position
	for isDigit(l.ch) {
		l.readChar()
	}

	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
		l.readExponent()
		return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
	}

	if l.ch == 'e' || l.ch == 'E' {
		l.readExponent()
		return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
	}

	return token.Token{Type: token.Int, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
}

func (l *Lexer) readDotFloat(line int) token.Token {
	start := l.position
	l.readChar()
	for isDigit(l.ch) {
		l.readChar()
	}
	l.readExponent()
	return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
}

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

func (l *Lexer) absorbComment(line int) (errTok token.Token, ok bool) {
	l.readChar()
	l.readChar()

	if lvl := l.longOpenLevel(); lvl >= 0 {
		l.consumeLongOpen(lvl)
		if _, terminated := l.readLongString(lvl); !terminated {
			return token.Token{Type: token.Illegal, Literal: "unfinished long comment", Line: line, Column: l.tokenCol}, false
		}
		return token.Token{}, true
	}

	if !l.hasYielded && l.ch == '!' && l.ModeDirective == "" {
		l.readChar()
		start := l.position
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
		word := strings.TrimSpace(string(l.input[start:l.position]))
		switch word {
		case "strict", "nonstrict", "nocheck":
			l.ModeDirective = word
		}
		return token.Token{}, true
	}

	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	return token.Token{}, true
}

func (l *Lexer) longOpenLevel() int {
	if l.ch != '[' {
		return -1
	}
	i := l.readPosition
	level := 0
	for i < len(l.input) && l.input[i] == '=' {
		level++
		i++
	}
	if i < len(l.input) && l.input[i] == '[' {
		return level
	}
	return -1
}

func (l *Lexer) consumeLongOpen(level int) {
	l.readChar()
	for k := 0; k < level; k++ {
		l.readChar()
	}
	l.readChar()
}

func (l *Lexer) matchLongClose(level int) bool {
	i := l.readPosition
	cnt := 0
	for i < len(l.input) && l.input[i] == '=' {
		cnt++
		i++
	}
	if cnt != level || i >= len(l.input) || l.input[i] != ']' {
		return false
	}
	l.readChar()
	for k := 0; k < level; k++ {
		l.readChar()
	}
	l.readChar()
	return true
}

func (l *Lexer) readLongString(level int) (lit string, terminated bool) {
	var b strings.Builder

	for {
		if l.ch == 0 {
			return b.String(), false
		}
		if l.ch == ']' && l.matchLongClose(level) {
			return b.String(), true
		}

		if l.ch == '\n' {
			l.line++
		}
		b.WriteRune(l.ch)
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() []rune {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString(ch rune) (lit, raw string, terminated bool) {
	l.readChar()
	bodyStart := l.position

	var b strings.Builder
	for l.ch != ch {
		if l.ch == 0 {
			return b.String(), string(l.input[bodyStart:l.position]), false
		}
		if l.ch == '\\' {
			l.readChar()
			l.readEscape(&b)
			continue
		}
		if l.ch == '\n' {
			l.line++
		}
		b.WriteRune(l.ch)
		l.readChar()
	}
	raw = string(l.input[bodyStart:l.position])

	l.readChar()

	return b.String(), raw, true
}

func Unescape(body string) string {
	l := New(body)
	var b strings.Builder
	for l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			l.readEscape(&b)
			continue
		}
		b.WriteRune(l.ch)
		l.readChar()
	}
	return b.String()
}

func (l *Lexer) readEscape(b *strings.Builder) {
	switch l.ch {
	case 'n':
		b.WriteByte('\n')
		l.readChar()
	case 't':
		b.WriteByte('\t')
		l.readChar()
	case 'r':
		b.WriteByte('\r')
		l.readChar()
	case 'v':
		b.WriteByte('\v')
		l.readChar()
	case 'f':
		b.WriteByte('\f')
		l.readChar()
	case 'a':
		b.WriteByte('\a')
		l.readChar()
	case 'b':
		b.WriteByte('\b')
		l.readChar()
	case '\\':
		b.WriteByte('\\')
		l.readChar()
	case '"':
		b.WriteByte('"')
		l.readChar()
	case '\'':
		b.WriteByte('\'')
		l.readChar()
	case '`':
		b.WriteByte('`')
		l.readChar()
	case '\n', '\r':
		first := l.ch
		l.readChar()
		if (first == '\r' && l.ch == '\n') || (first == '\n' && l.ch == '\r') {
			l.readChar()
		}
		b.WriteByte('\n')
	case 'x':
		l.readChar()
		v := 0
		for i := 0; i < 2 && isHexDigit(l.ch); i++ {
			v = v*16 + hexVal(l.ch)
			l.readChar()
		}
		b.WriteByte(byte(v))
	case 'z':
		l.readChar()
		for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
			l.readChar()
		}
	case 'u':
		l.readChar()
		if l.ch == '{' {
			l.readChar()
			var r rune
			for isHexDigit(l.ch) {
				r = r*16 + rune(hexVal(l.ch))
				l.readChar()
			}
			if l.ch == '}' {
				l.readChar()
			}
			b.WriteRune(r)
		}
	default:
		if isDigit(l.ch) {
			v := 0
			for i := 0; i < 3 && isDigit(l.ch); i++ {
				v = v*10 + int(l.ch-'0')
				l.readChar()
			}
			b.WriteByte(byte(v))
		} else {
			b.WriteByte('\\')
			b.WriteRune(l.ch)
			l.readChar()
		}
	}
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
	tok := token.Token{Type: t, Literal: lit, Line: l.line, Column: l.tokenCol}
	l.readChar()
	return tok
}

func (l *Lexer) makeToken(t token.Type, lit string, line int) token.Token {
	tok := token.Token{Type: t, Literal: lit, Line: line, Column: l.tokenCol}
	l.readChar()
	return tok
}

func (l *Lexer) readChar() {
	if l.ch == '\n' {
		l.column = 0
	}
	l.column++

	if l.readPosition >= len(l.input) {
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

func hexVal(ch rune) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10
	}
	return 0
}
