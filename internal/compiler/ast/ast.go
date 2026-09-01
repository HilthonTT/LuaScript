package ast

import (
	"bytes"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

type BaseNode struct {
	Token token.Token
}

func (b *BaseNode) Line() int {
	return b.Token.Line
}

type node interface {
	TokenLiteral() string
	String() string
	Line() int
}

type Statement interface {
	node
	statementNode()
}

type Expression interface {
	node
	expressionNode()
}

type Program struct {
	Block *Block
}

func (p *Program) TokenLiteral() string {
	if p.Block == nil {
		return ""
	}
	return p.Block.TokenLiteral()
}

func (p *Program) String() string {
	var out bytes.Buffer
	if p.Block != nil {
		out.WriteString(p.Block.String())
	}
	return out.String()
}

func (p *Program) Line() int {
	if p.Block == nil {
		return 0
	}
	return p.Block.Line()
}
