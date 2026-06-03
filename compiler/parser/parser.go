// Package parser builds an *ast.Program from a token stream produced by the
// lexer. The grammar implemented here is Lua 5.4 (reference manual §9).
//
// The parser is a hand-written, single-pass recursive-descent parser with a
// Pratt-style operator-precedence layer for expressions. There is no state
// machine and no lookahead beyond a single peek token — Lua 5.4 is LL(2) at
// most (see the Name vs. funcname disambiguation in `parseStatement`).
package parser

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/compiler/ast"
	"github.com/hilthontt/luascript/compiler/lexer"
	"github.com/hilthontt/luascript/compiler/parser/errors"
	"github.com/hilthontt/luascript/compiler/token"
)

// Mode controls REPL-friendly error reporting (e.g. distinguishing
// "still typing — give me more input" from a real syntax error).
type Mode int

// Recognised parser modes.
const (
	NormalMode Mode = iota + 1
	REPLMode
)

// Parser owns the token cursor and parser state.
type Parser struct {
	Lexer *lexer.Lexer
	Mode  Mode

	error *errors.Error

	curToken  token.Token
	peekToken token.Token
	// peek2 holds a one-token-ahead-of-peek lookahead, populated lazily by
	// peek2Token() and consumed by the next nextToken() call. The Lua 5.4
	// grammar is mostly LL(2)-or-less, but the Luau-style type-assertion
	// vs goto-label disambiguation needs to look two tokens past `::`.
	peek2 *token.Token

	// matchCounter generates unique scrutinee-binding names for the
	// parser-level `match` desugar (compiler/parser/match_statement.go).
	// Each `match` rewrites its scrutinee to a fresh `__match_N` local so
	// nested matches don't shadow each other in a confusing way.
	matchCounter int

	// loopDepth tracks how many for/while/repeat loops enclose the
	// current cursor. parseBreakStatement rejects `break` when it's 0.
	// Function bodies save and zero this — break does not escape into
	// the enclosing loop across a function boundary, matching Lua.
	loopDepth int
}

// New constructs a parser ready to consume the supplied lexer's tokens.
// Call ParseProgram to drive it.
func New(l *lexer.Lexer) *Parser {
	return &Parser{Lexer: l, Mode: NormalMode}
}

// ParseProgram drives the top-level block until EOF and returns the AST.
// Returns the partial AST and an error if one occurred. The parser is
// single-shot: do not call ParseProgram more than once on the same Parser.
func (p *Parser) ParseProgram() (program *ast.Program, err *errors.Error) {
	defer func() {
		if r := recover(); r != nil {
			if p.error != nil {
				err = p.error
				return
			}
			err = errors.InitError(
				fmt.Sprintf("panic during parse near token %q (line %d): %v",
					p.curToken.Literal, p.curToken.Line, r),
				errors.SyntaxError,
			)
		}
	}()

	// Prime the cursor: read two tokens so curToken/peekToken are both set.
	p.nextToken()
	p.nextToken()

	block := p.parseBlock()
	if p.error != nil {
		return &ast.Program{Block: block}, p.error
	}
	if !p.curTokenIs(token.EOF) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"unexpected "+describeToken(p.curToken)+" after end of chunk",
			"if this is meant to be a statement, check the previous block — an `end`, `then`, or `do` may be missing")
		return &ast.Program{Block: block}, p.error
	}

	return &ast.Program{Block: block}, nil
}

// parseBlock reads statements until it hits a block-terminating token
// (`end`, `else`, `elseif`, `until`, EOF). A leading or trailing return
// statement is recognised here because Lua restricts `return` to the last
// statement of a block.
func (p *Parser) parseBlock() *ast.Block {
	block := &ast.Block{
		BaseNode:   &ast.BaseNode{Token: p.curToken},
		Statements: []ast.Statement{},
	}
	for !p.endOfBlock() {
		if p.curTokenIs(token.Return) {
			block.Return = p.parseReturnStatement()
			// `return` must be the last statement in a block; an
			// optional `;` may follow but no further statements.
			p.skipSemicolons()
			break
		}
		stmt := p.parseStatement()
		if p.error != nil {
			return block
		}
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.skipSemicolons()
	}
	return block
}

// endOfBlock reports whether the current token closes the surrounding block
// in the Lua grammar.
func (p *Parser) endOfBlock() bool {
	switch p.curToken.Type {
	case token.EOF, token.End, token.Else, token.ElseIf, token.Until:
		return true
	}
	return false
}

// skipSemicolons consumes any number of stray `;` separators. Lua allows
// `;` as a no-op statement separator (and after a `return`).
func (p *Parser) skipSemicolons() {
	for p.curTokenIs(token.Semicolon) {
		p.nextToken()
	}
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	if p.peek2 != nil {
		p.peekToken = *p.peek2
		p.peek2 = nil
	} else {
		p.peekToken = p.Lexer.NextToken()
	}
}

// peek2Token returns the token immediately after peekToken without
// advancing the cursor. The first call consumes one token from the lexer
// and buffers it; subsequent calls return the same buffered token until a
// nextToken() call drains the buffer.
func (p *Parser) peek2Token() token.Token {
	if p.peek2 == nil {
		t := p.Lexer.NextToken()
		p.peek2 = &t
	}
	return *p.peek2
}

func (p *Parser) curTokenIs(t token.Type) bool  { return p.curToken.Type == t }
func (p *Parser) peekTokenIs(t token.Type) bool { return p.peekToken.Type == t }

// expectPeek advances if the peek token matches `t`; otherwise records an
// error and returns false. The caller MUST check the boolean.
func (p *Parser) expectPeek(t token.Type) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

// expectCur asserts the current token type without advancing. Records an
// error and returns false on mismatch. EOF promotion to EndOfFileError is
// handled centrally in errorAt.
func (p *Parser) expectCur(t token.Type) bool {
	if p.curTokenIs(t) {
		return true
	}
	p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
		fmt.Sprintf("expected %s, got %s", describeTokenType(t), describeToken(p.curToken)),
		"")
	return false
}

func (p *Parser) peekError(t token.Type) {
	cat := errors.UnexpectedTokenError
	if p.peekTokenIs(token.EOF) {
		cat = errors.EndOfFileError
	}
	p.errorAt(p.peekToken, cat, "",
		fmt.Sprintf("expected %s next, got %s", describeTokenType(t), describeToken(p.peekToken)),
		"")
}

// errorAt records a structured parse error anchored to a specific token.
// Format:
//
//	<construct>: <msg> at line N, column M.
//	       hint: <hint>           (only when hint != "")
//
// If `construct` is empty the prefix is dropped. If the offending token
// predates column tracking (Column == 0) the column suffix is omitted so
// older lexer paths don't print "column 0". When the cursor sits on EOF,
// generic SyntaxError / UnexpectedTokenError categories are promoted to
// EndOfFileError so the REPL's IsEOF() check fires for truncated input.
func (p *Parser) errorAt(tok token.Token, category int, construct, msg, hint string) {
	if p.error != nil {
		return
	}
	if p.curTokenIs(token.EOF) {
		switch category {
		case errors.SyntaxError, errors.UnexpectedTokenError:
			category = errors.EndOfFileError
		}
	}
	var b strings.Builder
	if construct != "" {
		b.WriteString(construct)
		b.WriteString(": ")
	}
	b.WriteString(msg)
	if tok.Column > 0 {
		fmt.Fprintf(&b, " at line %d, column %d.", tok.Line, tok.Column)
	} else {
		fmt.Fprintf(&b, " at line %d.", tok.Line)
	}
	if hint != "" {
		b.WriteString("\n       hint: ")
		b.WriteString(hint)
	}
	p.error = errors.InitError(b.String(), category)
}

// describeToken returns a human-friendly description of a token suitable
// for embedding in an error message. Keywords and punctuation are quoted
// by their literal; numbers/strings/identifiers get a kind word.
func describeToken(t token.Token) string {
	switch t.Type {
	case token.EOF:
		return "end of file"
	case token.Int, token.Float:
		if t.Literal != "" {
			return "number " + t.Literal
		}
		return "number"
	case token.String:
		return "string literal"
	case token.InterpString:
		return "interpolated string"
	case token.Ident:
		return "identifier '" + t.Literal + "'"
	case token.Illegal:
		if t.Literal != "" {
			return "illegal character '" + t.Literal + "'"
		}
		return "illegal character"
	}
	if t.Literal != "" {
		return "'" + t.Literal + "'"
	}
	return string(t.Type)
}

// describeTokenType describes an *expected* token type — symmetric with
// describeToken but takes a Type alone (no literal available). Keywords
// are rendered in their source form (lower-case) and punctuation by its
// operator form.
func describeTokenType(t token.Type) string {
	switch t {
	case token.EOF:
		return "end of file"
	case token.Int:
		return "integer literal"
	case token.Float:
		return "float literal"
	case token.String:
		return "string literal"
	case token.InterpString:
		return "interpolated string"
	case token.Ident:
		return "identifier"
	case token.True:
		return "'true'"
	case token.False:
		return "'false'"
	case token.Nil:
		return "'nil'"
	case token.If:
		return "'if'"
	case token.ElseIf:
		return "'elseif'"
	case token.Else:
		return "'else'"
	case token.Then:
		return "'then'"
	case token.End:
		return "'end'"
	case token.Do:
		return "'do'"
	case token.While:
		return "'while'"
	case token.Repeat:
		return "'repeat'"
	case token.Until:
		return "'until'"
	case token.For:
		return "'for'"
	case token.In:
		return "'in'"
	case token.Function:
		return "'function'"
	case token.Local:
		return "'local'"
	case token.Return:
		return "'return'"
	case token.Break:
		return "'break'"
	case token.Goto:
		return "'goto'"
	case token.Match:
		return "'match'"
	case token.And:
		return "'and'"
	case token.Or:
		return "'or'"
	case token.Not:
		return "'not'"
	}
	// Operators and punctuation have a literal-shaped Type string already.
	return "'" + string(t) + "'"
}

// baseAt builds a BaseNode anchored to a specific token. Used when an AST
// node's source position should be the token that opened it rather than the
// cursor position at the end of parsing.
func baseAt(tok token.Token) *ast.BaseNode {
	return &ast.BaseNode{Token: tok}
}
