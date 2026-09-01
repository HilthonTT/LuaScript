package parser

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

type Mode int

const (
	NormalMode Mode = iota + 1
	REPLMode
)

type Parser struct {
	Lexer *lexer.Lexer
	Mode  Mode

	error *errors.Error

	curToken  token.Token
	peekToken token.Token
	peek2     *token.Token

	compoundCounter int

	loopDepth int

	structNames  map[string]bool
	enumVariants map[string]bool

	depth int
}

const maxParseDepth = 4000

func New(l *lexer.Lexer) *Parser {
	return &Parser{Lexer: l, Mode: NormalMode}
}

func (p *Parser) enterDepth(construct string) bool {
	if p.depth >= maxParseDepth {
		if p.error == nil {
			p.errorAt(p.curToken, errors.SyntaxError, construct,
				"input nests too deeply", "reduce the nesting depth of this "+construct)
		}
		return true
	}
	p.depth++
	return false
}

func (p *Parser) leaveDepth() { p.depth-- }

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

func (p *Parser) parseBlock() *ast.Block {
	block := &ast.Block{
		BaseNode:   ast.BaseNode{Token: p.curToken},
		Statements: []ast.Statement{},
	}
	if p.enterDepth("block") {
		return block
	}
	defer p.leaveDepth()
	for !p.endOfBlock() {
		if p.curTokenIs(token.Return) {
			block.Return = p.parseReturnStatement()
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

func (p *Parser) endOfBlock() bool {
	switch p.curToken.Type {
	case token.EOF, token.End, token.Else, token.ElseIf, token.Until, token.Catch:
		return true
	}
	return false
}

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

func (p *Parser) peek2Token() token.Token {
	if p.peek2 == nil {
		t := p.Lexer.NextToken()
		p.peek2 = &t
	}
	return *p.peek2
}

func (p *Parser) curTokenIs(t token.Type) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.Type) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t token.Type) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

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
		if utf8.RuneCountInString(t.Literal) > 1 {
			return t.Literal
		}
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
	case token.Try:
		return "'try'"
	case token.Catch:
		return "'catch'"
	case token.Throw:
		return "'throw'"
	case token.And:
		return "'and'"
	case token.Or:
		return "'or'"
	case token.Not:
		return "'not'"
	}
	return "'" + string(t) + "'"
}

func baseAt(tok token.Token) ast.BaseNode {
	return ast.BaseNode{Token: tok}
}
