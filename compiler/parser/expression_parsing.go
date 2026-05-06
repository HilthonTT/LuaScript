package parser

import "github.com/hilthontt/sakura-lang/compiler/ast"

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)
