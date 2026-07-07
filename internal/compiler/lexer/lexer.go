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
	// column is the 1-based column of l.ch within the current line.
	// Maintained by readChar; reset to 1 each time we advance past '\n'.
	column int
	// tokenCol snapshots l.column at the start of the in-progress token
	// (set by nextToken after skipWhitespace) so token-construction
	// helpers can stamp the column where the lexeme *began*, not where
	// the lexer happens to be sitting when it builds the Token struct.
	tokenCol int

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
		lit, ok := l.readString(quote)
		if !ok {
			return token.Token{Type: token.Illegal, Literal: "unfinished string", Line: line, Column: l.tokenCol}
		}
		typ := token.String
		if quote == '`' {
			typ = token.InterpString
		}
		return token.Token{Type: typ, Literal: lit, Line: line, Column: l.tokenCol}
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

// readNumberToken handles integers, floats, and hex. Called when l.ch is a
// digit; an integer becomes a float if a '.' is encountered mid-read.
func (l *Lexer) readNumberToken(line int) token.Token {
	// Hex: 0x... (integer, or a float with a '.' fraction and/or 'p' exponent).
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		start := l.position // include the leading "0x"
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

	// Integer becomes float on '.'
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
		l.readExponent()
		return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
	}

	// An exponent with no radix point still denotes a float (Lua 5.4 §3.1):
	// `1e10`, `2E3` are floats, not an integer followed by an identifier.
	if l.ch == 'e' || l.ch == 'E' {
		l.readExponent()
		return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
	}

	return token.Token{Type: token.Int, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
}

// readDotFloat handles floats starting with '.' (e.g. .5). Called when l.ch == '.'.
func (l *Lexer) readDotFloat(line int) token.Token {
	start := l.position
	l.readChar() // consume '.'
	for isDigit(l.ch) {
		l.readChar()
	}
	l.readExponent()
	return token.Token{Type: token.Float, Literal: string(l.input[start:l.position]), Line: line, Column: l.tokenCol}
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
// absorbComment skips a comment. ok is false when a long comment hit EOF
// before its closing bracket; the caller should surface errTok instead of
// silently treating the swallowed remainder of the file as comment text.
func (l *Lexer) absorbComment(line int) (errTok token.Token, ok bool) {
	l.readChar() // consume second '-'
	l.readChar() // move past --

	if lvl := l.longOpenLevel(); lvl >= 0 {
		l.consumeLongOpen(lvl)
		if _, terminated := l.readLongString(lvl); !terminated {
			return token.Token{Type: token.Illegal, Literal: "unfinished long comment", Line: line, Column: l.tokenCol}, false
		}
		return token.Token{}, true
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
		return token.Token{}, true
	}

	// Short comment: skip to end of line
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	return token.Token{}, true
}

// longOpenLevel reports the level of a long-bracket opener at the current '['
// (`[` followed by N '=' then '['), where N is the level, or -1 if the cursor
// is not on a long-bracket opener. It does not consume input.
func (l *Lexer) longOpenLevel() int {
	if l.ch != '[' {
		return -1
	}
	i := l.readPosition // index just past l.ch
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

// consumeLongOpen advances past a long-bracket opener of the given level
// (`[` + '='*level + `[`).
func (l *Lexer) consumeLongOpen(level int) {
	l.readChar() // opening '['
	for k := 0; k < level; k++ {
		l.readChar()
	}
	l.readChar() // second '['
}

// matchLongClose, with the cursor on a ']', reports whether it begins a long
// close bracket of the given level (`]` + '='*level + `]`). On a match it
// consumes the whole close bracket; otherwise it leaves the cursor untouched.
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
	l.readChar() // first ']'
	for k := 0; k < level; k++ {
		l.readChar()
	}
	l.readChar() // closing ']'
	return true
}

// readLongString reads a long-bracket string of the given level. Called after
// the opener has been consumed; stops at the matching `]=*level]` so inner
// brackets of a different level are kept as content.
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

// readString reads a quoted string. terminated is false when EOF arrived
// before the closing quote — silently accepting that would swallow the rest
// of the file into the literal and compile a truncated program.
func (l *Lexer) readString(ch rune) (lit string, terminated bool) {
	l.readChar() // skip opening quote

	var b strings.Builder
	for l.ch != ch {
		if l.ch == 0 {
			return b.String(), false
		}
		if l.ch == '\\' {
			l.readChar() // consume the backslash
			l.readEscape(&b)
			continue
		}
		if l.ch == '\n' {
			l.line++ // keep diagnostics' line numbers honest across multi-line literals
		}
		b.WriteRune(l.ch)
		l.readChar()
	}

	l.readChar() // move past closing quote

	return b.String(), true
}

// readEscape consumes one escape sequence (the cursor is on the char after the
// backslash) and appends the decoded bytes to b, advancing past the sequence.
// Covers Lua 5.4 §3.1: \a \b \f \n \r \t \v, \\ \" \' \`, \ddd (decimal),
// \xHH (hex), \u{XXX} (UTF-8), \z (skip following whitespace), and a
// backslash-newline line continuation.
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
		// Line continuation: \<newline> yields a single '\n'.
		first := l.ch
		l.readChar()
		if (first == '\r' && l.ch == '\n') || (first == '\n' && l.ch == '\r') {
			l.readChar() // swallow the paired CR/LF
		}
		b.WriteByte('\n')
	case 'x':
		l.readChar() // consume 'x'
		v := 0
		for i := 0; i < 2 && isHexDigit(l.ch); i++ {
			v = v*16 + hexVal(l.ch)
			l.readChar()
		}
		b.WriteByte(byte(v))
	case 'z':
		l.readChar() // consume 'z'
		for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
			l.readChar()
		}
	case 'u':
		l.readChar() // consume 'u'
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
			// \ddd: up to three decimal digits.
			v := 0
			for i := 0; i < 3 && isDigit(l.ch); i++ {
				v = v*10 + int(l.ch-'0')
				l.readChar()
			}
			b.WriteByte(byte(v))
		} else {
			// Unknown escape: keep it verbatim rather than dropping data.
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
	// Advance column tracking. A newline rolls us onto a new line at col 1.
	if l.ch == '\n' {
		l.column = 0
	}
	l.column++

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
