// Package ast defines the abstract syntax tree for luascript, a language
// with the syntax of Lua 5.4. The node shapes here mirror the productions in
// the Lua 5.4 reference manual, §9 (The Complete Syntax of Lua).
package ast

import (
	"bytes"

	"github.com/hilthontt/luascript/compiler/token"
)

// BaseNode carries the location and lexeme that every AST node tracks.
type BaseNode struct {
	Token token.Token
}

// Line returns the source line where this node begins.
func (b *BaseNode) Line() int {
	return b.Token.Line
}

// node is the common interface every AST element satisfies.
type node interface {
	TokenLiteral() string
	String() string
	Line() int
}

// Statement is any node that appears in a statement position.
type Statement interface {
	node
	statementNode()
}

// Expression is any node that appears in an expression position.
type Expression interface {
	node
	expressionNode()
}

// Program is the root node — a Lua chunk is a single block.
type Program struct {
	Block *Block
}

// TokenLiteral returns the literal of the first statement, or "" if empty.
func (p *Program) TokenLiteral() string {
	if p.Block == nil {
		return ""
	}
	return p.Block.TokenLiteral()
}

// String renders the program as valid Lua source (best-effort, for debugging).
func (p *Program) String() string {
	var out bytes.Buffer
	if p.Block != nil {
		out.WriteString(p.Block.String())
	}
	return out.String()
}

// Line returns the line of the first statement, or 0 if empty.
func (p *Program) Line() int {
	if p.Block == nil {
		return 0
	}
	return p.Block.Line()
}

type Pattern interface {
	node
	patternNode()
}
