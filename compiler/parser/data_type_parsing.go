package parser

import (
	"strconv"
	"strings"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
)

func (p *Parser) parseIntegerLiteral() ast.Expression {
	tok := p.curToken
	lit := tok.Literal
	var (
		v   int64
		err error
	)
	if strings.HasPrefix(lit, "0x") || strings.HasPrefix(lit, "0X") {
		v, err = strconv.ParseInt(lit[2:], 16, 64)
	} else {
		v, err = strconv.ParseInt(lit, 10, 64)
	}
	if err != nil {
		p.error = errors.NewTypeParsingError(lit, "Integer", tok.Line)
		return nil
	}
	p.nextToken()
	return &ast.IntegerLiteral{BaseNode: baseAt(tok), Value: v}
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	tok := p.curToken
	v, err := strconv.ParseFloat(tok.Literal, 64)
	if err != nil {
		p.error = errors.NewTypeParsingError(tok.Literal, "Float", tok.Line)
		return nil
	}
	p.nextToken()
	return &ast.FloatLiteral{BaseNode: baseAt(tok), Value: v}
}
