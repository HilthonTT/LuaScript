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

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/lexer"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/compiler/token"
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

		p.errorf(errors.UnexpectedTokenError,
			"unexpected token %s(%q) after chunk. Line: %d",
			p.curToken.Type, p.curToken.Literal, p.curToken.Line)

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
	p.peekToken = p.Lexer.NextToken()
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
// error and returns false on mismatch.
func (p *Parser) expectCur(t token.Type) bool {
	if p.curTokenIs(t) {
		return true
	}
	p.errorf(errors.UnexpectedTokenError,
		"expected token %s, got %s(%q). Line: %d",
		t, p.curToken.Type, p.curToken.Literal, p.curToken.Line)
	return false
}

func (p *Parser) peekError(t token.Type) {
	p.errorf(errors.UnexpectedTokenError,
		"expected next token to be %s, got %s(%q) instead. Line: %d",
		t, p.peekToken.Type, p.peekToken.Literal, p.peekToken.Line)
}

// errorf records a parser error with the given category and message. Once
// an error is set, parsing should unwind to ParseProgram (which returns it).
func (p *Parser) errorf(category int, format string, args ...any) {
	if p.error != nil {
		return // keep the first error
	}
	p.error = errors.InitError(fmt.Sprintf(format, args...), category)
}

// baseAt builds a BaseNode anchored to a specific token. Used when an AST
// node's source position should be the token that opened it rather than the
// cursor position at the end of parsing.
func baseAt(tok token.Token) *ast.BaseNode {
	return &ast.BaseNode{Token: tok}
}
