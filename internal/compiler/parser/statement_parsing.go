package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.Semicolon:
		p.nextToken()
		return nil

	case token.Local:
		return p.parseLocalStatement()
	case token.Function:
		return p.parseFunctionDeclaration()
	case token.If:
		return p.parseIfStatement()
	case token.While:
		return p.parseWhileStatement()
	case token.Repeat:
		return p.parseRepeatStatement()
	case token.For:
		return p.parseForStatement()
	case token.Do:
		return p.parseDoStatement()
	case token.Break:
		return p.parseBreakStatement()
	case token.Goto:
		return p.parseGotoStatement()
	case token.Label:
		return p.parseLabelStatement()
	case token.Match:
		return p.parseMatchStatement()
	case token.Enum:
		return p.parseEnumStatement()
	case token.Defer:
		return p.parseDeferStatement()
	case token.Try:
		return p.parseTryCatchStatement()
	case token.Throw:
		return p.parseThrowStatement()
	case token.End, token.Else, token.ElseIf, token.Until, token.Catch:
		p.errorAt(p.curToken, errors.UnexpectedEndError, "",
			"unexpected "+describeToken(p.curToken)+" with no matching block to close",
			"this keyword closes a block — make sure every `if`/`for`/`while`/`do`/`function`/`match` is properly opened first")
		return nil
	}

	if p.curTokenIs(token.Ident) && p.curToken.Literal == "type" && p.peekTokenIs(token.Ident) {
		return p.parseTypeAliasStatement()
	}

	if p.curTokenIs(token.Ident) && p.curToken.Literal == "struct" && p.peekTokenIs(token.Ident) {
		return p.parseStructStatement()
	}

	if p.curTokenIs(token.Ident) && p.curToken.Literal == "continue" && !p.peekStartsSuffix() {
		return p.parseContinueStatement()
	}

	return p.parseExprOrAssignStatement()
}

func (p *Parser) parseTypeAliasStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

	if !p.expectCur(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	p.nextToken()

	var typeParams []string
	if p.curTokenIs(token.LT) {
		typeParams = p.parseTypeParams()
		if p.error != nil {
			return nil
		}
	}

	if !p.expectCur(token.Assign) {
		return nil
	}
	p.nextToken()

	target := p.parseType()
	if target == nil {
		return nil
	}
	return &ast.TypeAliasStatement{
		BaseNode:   baseAt(tok),
		Name:       name,
		TypeParams: typeParams,
		Target:     target,
	}
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	tok := p.curToken
	p.nextToken()

	stmt := &ast.ReturnStatement{BaseNode: baseAt(tok)}
	if p.endOfBlock() || p.curTokenIs(token.Semicolon) {
		return stmt
	}
	stmt.Values = p.parseExpressionList()
	return stmt
}

func (p *Parser) parseBreakStatement() ast.Statement {
	tok := p.curToken
	if p.loopDepth == 0 {
		p.errorAt(tok, errors.SyntaxError, "break",
			"'break' outside a loop",
			"break is only valid inside a for, while, or repeat loop")
		return nil
	}
	p.nextToken()
	return &ast.BreakStatement{BaseNode: baseAt(tok)}
}

func (p *Parser) peekStartsSuffix() bool {
	switch p.peekToken.Type {
	case token.Assign, token.Comma, token.Dot, token.Colon, token.Label,
		token.LParen, token.LBracket, token.LBrace,
		token.String, token.InterpString:
		return true
	}
	_, isCompound := compoundOps[p.peekToken.Type]
	return isCompound
}

func (p *Parser) parseContinueStatement() ast.Statement {
	tok := p.curToken
	if p.loopDepth == 0 {
		p.errorAt(tok, errors.SyntaxError, "continue",
			"'continue' outside a loop",
			"continue is only valid inside a for, while, or repeat loop")
		return nil
	}
	p.nextToken()
	return &ast.ContinueStatement{BaseNode: baseAt(tok)}
}

func (p *Parser) parseGotoStatement() ast.Statement {
	tok := p.curToken
	if !p.expectPeek(token.Ident) {
		return nil
	}
	label := p.curToken.Literal
	p.nextToken()
	return &ast.GotoStatement{BaseNode: baseAt(tok), Label: label}
}

func (p *Parser) parseLabelStatement() ast.Statement {
	tok := p.curToken
	if !p.expectPeek(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.Label) {
		return nil
	}
	p.nextToken()
	return &ast.LabelStatement{BaseNode: baseAt(tok), Name: name}
}

func (p *Parser) parseLocalStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

	if p.curTokenIs(token.Function) {
		return p.parseLocalFunctionStatement(tok)
	}

	stmt := &ast.LocalStatement{BaseNode: baseAt(tok)}
	for {
		if !p.expectCur(token.Ident) {
			return nil
		}
		ln := ast.LocalName{Name: p.curToken.Literal}
		p.nextToken()
		if p.curTokenIs(token.Colon) {
			p.nextToken()
			ln.Type = p.parseType()
			if ln.Type == nil {
				return nil
			}
		}
		if p.curTokenIs(token.LT) {
			p.nextToken()
			if !p.expectCur(token.Ident) {
				return nil
			}
			ln.Attrib = p.curToken.Literal
			if ln.Attrib != "const" && ln.Attrib != "close" {
				p.errorAt(p.curToken, errors.SyntaxError, "local",
					fmt.Sprintf("unknown attribute '%s'", ln.Attrib),
					"Lua 5.4 supports `<const>` and `<close>`")
				return nil
			}
			p.nextToken()
			if !p.expectCur(token.GT) {
				return nil
			}
			p.nextToken()
		}
		stmt.Names = append(stmt.Names, ln)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken()
	}

	if p.curTokenIs(token.Assign) {
		p.nextToken()
		stmt.Values = p.parseExpressionList()
	}
	return stmt
}

func (p *Parser) parseLocalFunctionStatement(localTok token.Token) ast.Statement {
	fnTok := p.curToken
	p.nextToken()
	if !p.expectCur(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	p.nextToken()
	body := p.parseFunctionBody(fnTok)
	if body == nil {
		return nil
	}
	return &ast.LocalFunctionStatement{
		BaseNode: baseAt(localTok),
		Name:     name,
		Func:     body,
	}
}

func (p *Parser) parseFunctionDeclaration() ast.Statement {
	tok := p.curToken
	p.nextToken()

	if !p.expectCur(token.Ident) {
		return nil
	}
	name := &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}
	p.nextToken()

	var dotted []string
	var method string
	for p.curTokenIs(token.Dot) {
		p.nextToken()
		if !p.expectCur(token.Ident) {
			return nil
		}
		dotted = append(dotted, p.curToken.Literal)
		p.nextToken()
	}
	if p.curTokenIs(token.Colon) {
		p.nextToken()
		if !p.expectCur(token.Ident) {
			return nil
		}
		method = p.curToken.Literal
		p.nextToken()
	}

	body := p.parseFunctionBody(tok)
	if body == nil {
		return nil
	}
	return &ast.FunctionDeclaration{
		BaseNode:     baseAt(tok),
		Name:         name,
		DottedFields: dotted,
		MethodName:   method,
		Func:         body,
	}
}
