package parser

import (
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/compiler/ast"
	"github.com/hilthontt/luascript/compiler/lexer"
	"github.com/hilthontt/luascript/compiler/parser/errors"
	"github.com/hilthontt/luascript/compiler/token"
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

func (p *Parser) parseFalseLiteral() ast.Expression {
	exp := &ast.BooleanLiteral{BaseNode: baseAt(p.curToken), Value: false}
	p.nextToken()
	return exp
}

func (p *Parser) parseTrueLiteral() ast.Expression {
	exp := &ast.BooleanLiteral{BaseNode: baseAt(p.curToken), Value: true}
	p.nextToken()
	return exp
}

func (p *Parser) parseVarArg() ast.Expression {
	exp := &ast.VarargExpression{BaseNode: baseAt(p.curToken)}
	p.nextToken()
	return exp

}

func (p *Parser) parseIdent() ast.Expression {
	exp := &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}
	p.nextToken()
	return exp
}

func (p *Parser) parseStringLiteral() ast.Expression {
	tok := p.curToken
	raw := p.curToken.Literal

	// Only backtick-quoted strings (token.InterpString) participate in
	// `{expr}` interpolation. Plain `"..."` / `'...'` (token.String) are
	// emitted verbatim even if they contain `{` — they may be JSON, format
	// strings, etc.
	if tok.Type != token.InterpString {
		exp := &ast.StringLiteral{BaseNode: baseAt(p.curToken), Value: p.curToken.Literal}
		p.nextToken()
		return exp
	}

	// Fast path: backtick string with no `{` — just a plain string.
	if !strings.Contains(raw, "{") && !strings.Contains(raw, "}") {
		exp := &ast.StringLiteral{BaseNode: baseAt(p.curToken), Value: p.curToken.Literal}
		p.nextToken()
		return exp
	}

	return p.buildInterpolation(tok, raw)
}

func (p *Parser) buildInterpolation(tok token.Token, raw string) ast.Expression {
	type part struct {
		text   string
		isExpr bool
	}

	var parts []part
	s := raw

	for len(s) > 0 {
		idx := strings.Index(s, "{")

		if idx == -1 {
			if len(s) > 0 {
				parts = append(parts, part{s, false})
			}
			break
		}
		if idx > 0 {
			parts = append(parts, part{s[:idx], false})
		}
		s = s[idx+1:]
		depth, end := 1, 0
		for end < len(s) && depth > 0 {
			if s[end] == '{' {
				depth++
			} else if s[end] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
			end++
		}
		if depth != 0 {
			p.error = errors.NewTypeParsingError(raw, "InterpolatedString", tok.Line)
			return &ast.StringLiteral{BaseNode: baseAt(tok), Value: raw}
		}
		parts = append(parts, part{s[:end], true})
		s = s[end+1:]
	}

	var result ast.Expression
	for _, pt := range parts {
		var expr ast.Expression
		if pt.isExpr {
			l := lexer.New(pt.text)
			sub := New(l)
			// Prime cur/peek tokens — New() doesn't do it; ParseProgram does.
			sub.nextToken()
			sub.nextToken()
			inner := sub.parseExpression()
			if inner == nil {
				continue
			}
			// Wrap in tostring(...) so non-string values concat without panic.
			expr = &ast.CallExpression{
				BaseNode: baseAt(tok),
				Func:     &ast.Identifier{BaseNode: baseAt(tok), Name: "tostring"},
				Args:     []ast.Expression{inner},
			}
		} else {
			expr = &ast.StringLiteral{BaseNode: baseAt(tok), Value: pt.text}
		}
		if result == nil {
			result = expr
		} else {
			result = &ast.BinaryExpression{
				BaseNode: baseAt(tok),
				Op:       "..",
				Left:     result,
				Right:    expr,
			}
		}
	}
	if result == nil {
		return &ast.StringLiteral{BaseNode: baseAt(tok), Value: ""}
	}
	p.nextToken()
	return result
}
