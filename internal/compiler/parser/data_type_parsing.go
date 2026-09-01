package parser

import (
	"math"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func int64FromUint64Bits(u uint64) int64 {
	if u > math.MaxInt64 {
		return -int64(^u) - 1
	}
	return int64(u)
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	tok := p.curToken
	lit := tok.Literal
	if strings.HasPrefix(lit, "0x") || strings.HasPrefix(lit, "0X") {
		if u, err := strconv.ParseUint(lit[2:], 16, 64); err == nil {
			p.nextToken()
			return &ast.IntegerLiteral{BaseNode: baseAt(tok), Value: int64FromUint64Bits(u)}
		}
		p.error = errors.NewTypeParsingError(lit, "Integer", tok.Line)
		return nil
	}
	if v, err := strconv.ParseInt(lit, 10, 64); err == nil {
		p.nextToken()
		return &ast.IntegerLiteral{BaseNode: baseAt(tok), Value: v}
	}
	if f, err := strconv.ParseFloat(lit, 64); err == nil {
		p.nextToken()
		return &ast.FloatLiteral{BaseNode: baseAt(tok), Value: f}
	}
	p.error = errors.NewTypeParsingError(lit, "Integer", tok.Line)
	return nil
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	tok := p.curToken
	lit := tok.Literal
	if (strings.HasPrefix(lit, "0x") || strings.HasPrefix(lit, "0X")) &&
		!strings.ContainsAny(lit, "pP") {
		lit += "p0"
	}
	v, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			p.nextToken()
			return &ast.FloatLiteral{BaseNode: baseAt(tok), Value: v}
		}
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

	if tok.Type != token.InterpString {
		exp := &ast.StringLiteral{BaseNode: baseAt(p.curToken), Value: p.curToken.Literal}
		p.nextToken()
		return exp
	}

	raw = tok.Raw
	if raw == "" {
		raw = tok.Literal
	}

	if !strings.Contains(raw, "{") && !strings.Contains(raw, "}") {
		exp := &ast.StringLiteral{BaseNode: baseAt(p.curToken), Value: p.curToken.Literal}
		p.nextToken()
		return exp
	}

	return p.buildInterpolation(tok, raw)
}

func skipEscape(s string, i int) int {
	if i+2 < len(s) && s[i+1] == 'u' && s[i+2] == '{' {
		if end := strings.IndexByte(s[i+2:], '}'); end >= 0 {
			return i + 2 + end + 1
		}
	}
	return i + 2
}

func unescapedIndexOfBrace(s string) int {
	for i := 0; i < len(s); {
		switch s[i] {
		case '\\':
			i = skipEscape(s, i)
		case '{':
			return i
		default:
			i++
		}
	}
	return -1
}

func (p *Parser) buildInterpolation(tok token.Token, raw string) ast.Expression {
	p.nextToken()

	type part struct {
		text   string
		isExpr bool
	}

	var parts []part
	s := raw

	for len(s) > 0 {
		idx := unescapedIndexOfBrace(s)

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
			switch s[end] {
			case '\\':
				end = skipEscape(s, end)
				continue
			case '{':
				depth++
			case '}':
				depth--
			case '"', '\'':
				q := s[end]
				end++
				for end < len(s) && s[end] != q {
					if s[end] == '\\' && end+1 < len(s) {
						end++
					}
					end++
				}
			}
			if depth == 0 {
				break
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
			sub.nextToken()
			sub.nextToken()
			inner := sub.parseExpression()
			if inner == nil || sub.error != nil {
				if sub.error != nil {
					p.error = sub.error
				} else {
					p.error = errors.NewTypeParsingError(raw, "InterpolatedString", tok.Line)
				}
				return &ast.StringLiteral{BaseNode: baseAt(tok), Value: raw}
			}
			if !sub.curTokenIs(token.EOF) {
				p.error = errors.NewTypeParsingError(raw, "InterpolatedString", tok.Line)
				return &ast.StringLiteral{BaseNode: baseAt(tok), Value: raw}
			}
			expr = &ast.CallExpression{
				BaseNode: baseAt(tok),
				Func:     &ast.Identifier{BaseNode: baseAt(tok), Name: "tostring"},
				Args:     []ast.Expression{inner},
			}
		} else {
			expr = &ast.StringLiteral{BaseNode: baseAt(tok), Value: lexer.Unescape(pt.text)}
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
	return result
}
