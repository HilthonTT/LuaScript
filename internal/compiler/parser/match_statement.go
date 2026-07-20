package parser

// `match` is a first-class statement: the parser produces an
// `ast.MatchStatement` and the bytecode generator lowers it directly to a
// jump chain (see bytecode/statement_generation.go::compileMatch).
//
//	match <expr> do
//	  <pattern> [if <guard>] -> <stmt>
//	  ...
//	  _ -> <stmt>
//	end
//
// It used to desugar here into a `do` block driven by a `__matched` flag.
// Keeping it as a real node means the type checker can bind and narrow the
// arm binders, the formatter can round-trip `match` source, the analyzer can
// see arms as arms, and codegen emits one test-and-branch per arm instead of
// re-reading a flag before every one.
//
// Pattern forms (classified here, evaluated by codegen):
//
//	<expr>                 value / literal — matches when `scrutinee == expr`.
//	                       Comma-separated alternatives allowed: `1, 2, 3`.
//	_                      wildcard — matches anything, binds nothing.
//	name : Type            typed binding — matches when the runtime type
//	                       matches, binds `name` to the scrutinee. `_ : Type`
//	                       tests without binding; `x : any` binds anything.
//	Path(a, b, _)          positional destructure of a tagged-enum variant:
//	                       matches when `scrutinee.__tag == "Path-last-seg"`,
//	                       binds each named position from `scrutinee[i]`.
//	Path{ f = a, g = _ }   named destructure of a struct: matches when
//	                       `typeof(scrutinee) == "Path-last-seg"`, binds each
//	                       `a` from `scrutinee.f`.
//
// The scrutinee is evaluated exactly once. Arms are tried in order and the
// statement is NOT exhaustive: when nothing matches it is a no-op.

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

// matchSyntax is the canonical `match` syntax shown in user-facing error
// hints. Kept in one place so we don't drift if the form changes.
const matchSyntax = "match <expr> do <pattern> [if <guard>] -> <stmt> ... [_ -> <stmt>] end"

// parseMatchStatement consumes a `match ... end` block.
func (p *Parser) parseMatchStatement() ast.Statement {
	matchTok := p.curToken
	p.nextToken() // consume 'match'

	if p.curTokenIs(token.Do) || p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"expected an expression to scrutinise after 'match', got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}

	subject := p.parseExpression()
	if subject == nil {
		return nil
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected 'do' after the scrutinee expression, got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}
	p.nextToken() // consume 'do'

	stmt := &ast.MatchStatement{BaseNode: baseAt(matchTok), Subject: subject}
	sawWildcard := false

	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(matchTok, errors.EndOfFileError, "match",
				fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
				"each arm is `<pattern> [if <guard>] -> <stmt>`; close the whole construct with `end`")
			return nil
		}

		armTok := p.curToken

		pattern, ok := p.parseArmPattern()
		if !ok {
			return nil
		}

		// Optional guard: `if <expr>` between the pattern and `->`.
		var guard ast.Expression
		if p.curTokenIs(token.If) {
			p.nextToken() // consume 'if'
			guard = p.parseExpression()
			if guard == nil {
				return nil
			}
		}

		if !p.curTokenIs(token.Arrow) {
			p.reportArrowError()
			return nil
		}
		p.nextToken() // consume '->'

		if p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"arm body cannot be empty",
				"write one statement after '->', or wrap multiple statements in `do ... end`")
			return nil
		}

		// `return` is normally a block terminator (Block.Return), but a match
		// arm body is a single statement — and returning from an arm is a
		// very common pattern. The bytecode generator already compiles a
		// ReturnStatement wherever it appears, so accept it here directly.
		var body ast.Statement
		if p.curTokenIs(token.Return) {
			body = p.parseReturnStatement()
		} else {
			body = p.parseStatement()
		}
		if body == nil {
			return nil
		}
		p.skipSemicolons()

		isWildcardArm := pattern.Kind == ast.MatchWildcard && guard == nil
		if isWildcardArm {
			if sawWildcard {
				p.errorAt(armTok, errors.SyntaxError, "match",
					"duplicate wildcard '_' arm",
					"`match` allows at most one unguarded '_' arm")
				return nil
			}
			sawWildcard = true
			if !p.curTokenIs(token.End) {
				p.errorAt(p.curToken, errors.SyntaxError, "match",
					"wildcard '_' arm must be the last arm",
					"the '_' arm matches anything, so arms following it are unreachable; move '_' to the bottom")
				return nil
			}
		}

		stmt.Arms = append(stmt.Arms, ast.MatchStmtArm{
			BaseNode: baseAt(armTok),
			Pattern:  pattern,
			Guard:    guard,
			Body:     body,
		})
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(matchTok, errors.UnexpectedTokenError, "match",
			fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'

	return stmt
}

// parseArmPattern reads the pattern portion of an arm: either a single
// binding/destructure/wildcard pattern, or a comma-separated list of value
// patterns (which become the alternatives of one MatchValue pattern).
func (p *Parser) parseArmPattern() (ast.MatchPattern, bool) {
	first, ok := p.parseOnePattern()
	if !ok {
		return ast.MatchPattern{}, false
	}

	// Only value patterns may be joined with commas: alternatives with
	// bindings would leave the body unsure which binder is live.
	for p.curTokenIs(token.Comma) {
		if first.Kind != ast.MatchValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return ast.MatchPattern{}, false
		}
		p.nextToken() // consume ','
		next, ok := p.parseOnePattern()
		if !ok {
			return ast.MatchPattern{}, false
		}
		if next.Kind != ast.MatchValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return ast.MatchPattern{}, false
		}
		first.Values = append(first.Values, next.Values...)
	}
	return first, true
}

// parseOnePattern parses a single pattern, dispatching on the leading tokens.
func (p *Parser) parseOnePattern() (ast.MatchPattern, bool) {
	// `_` — wildcard or typed no-bind (`_ : T`).
	if p.curTokenIs(token.Ident) && p.curToken.Literal == "_" {
		if p.peekTokenIs(token.Colon) {
			return p.parseTypedPattern("_")
		}
		p.nextToken() // consume '_'
		return ast.MatchPattern{Kind: ast.MatchWildcard}, true
	}

	// `name : Type` — typed binding.
	if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon) {
		name := p.curToken.Literal
		return p.parseTypedPattern(name)
	}

	// Otherwise parse a full expression and classify it. A call-shaped
	// expression naming a declared struct whose arguments are plain binders
	// is a destructure; anything else is a value pattern compared with `==`.
	expr := p.parseExpression()
	if expr == nil {
		return ast.MatchPattern{}, false
	}
	return p.classifyPattern(expr), true
}

// parseTypedPattern reads the `Type` half of a `name : Type` pattern. The
// cursor is on `name` at entry.
func (p *Parser) parseTypedPattern(name string) (ast.MatchPattern, bool) {
	p.nextToken() // consume name/underscore
	p.nextToken() // consume ':'
	ty := p.parseType()
	if ty == nil {
		return ast.MatchPattern{}, false
	}
	// Only a primitive or a named type makes a usable runtime test; unions,
	// tables, and function types would need structural probing we don't do
	// in a pattern. Reject them with a clear message.
	switch ty.(type) {
	case *ast.TypePrimitive, *ast.TypeName:
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"a typed pattern must name a primitive or a type/struct/enum name",
			"e.g. `n: number`, `s: string`, `p: Point`; use a guard for structural tests")
		return ast.MatchPattern{}, false
	}
	return ast.MatchPattern{Kind: ast.MatchTyped, Bind: name, Type: ty}, true
}

// classifyPattern decides whether an already-parsed expression is a
// destructure pattern (call-shaped with binder arguments) or a plain value
// pattern. Destructuring requires the callee to name a declaration from
// this chunk — a payload-carrying tagged-enum variant for the positional
// form (`Circle(r)` reads __tag + payload slots) or a struct for the named
// form (`Point{ x = a }` reads typeof + named fields). Without that gate,
// an ordinary value pattern like `double(a)` (call double, compare the
// result) would silently flip into a never-matching `__tag` probe the
// moment its argument is an identifier instead of a literal. Variants and
// structs imported from other modules therefore match as value patterns;
// use a guard for those.
func (p *Parser) classifyPattern(expr ast.Expression) ast.MatchPattern {
	valuePattern := ast.MatchPattern{Kind: ast.MatchValue, Values: []ast.Expression{expr}}

	call, ok := expr.(*ast.CallExpression)
	if !ok {
		return valuePattern
	}
	seg, ok := dottedTail(call.Func)
	if !ok {
		return valuePattern
	}

	// Named form: `Name{ field = binder, ... }` (folded to a single
	// table-constructor argument).
	if len(call.Args) == 1 {
		if tc, ok := call.Args[0].(*ast.TableConstructor); ok {
			if binders, ok := namedBinders(tc); ok && p.structNames[seg] {
				return ast.MatchPattern{Kind: ast.MatchDestructureNamed, Tag: seg, NamedBinds: binders}
			}
			return valuePattern
		}
	}

	if !p.enumVariants[seg] {
		return valuePattern
	}

	// Positional form: `Name(a, b, _)` — every argument must be a plain
	// identifier (a binder) or `_`.
	binders := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		id, ok := a.(*ast.Identifier)
		if !ok {
			return valuePattern
		}
		binders = append(binders, id.Name)
	}
	return ast.MatchPattern{Kind: ast.MatchDestructurePos, Tag: seg, PosBinds: binders}
}

// dottedTail returns the final identifier of a `A.B.C` path expression (or a
// bare identifier), and whether the expression is such a path.
func dottedTail(e ast.Expression) (string, bool) {
	switch n := e.(type) {
	case *ast.Identifier:
		return n.Name, true
	case *ast.IndexExpression:
		if n.IsDot {
			if s, ok := n.Index.(*ast.StringLiteral); ok {
				return s.Value, true
			}
		}
	}
	return "", false
}

// namedBinders extracts `field = binder` pairs from a table constructor,
// requiring every entry to be a record field whose value is a plain
// identifier (or `_`). Returns false if any entry doesn't fit, so the caller
// can fall back to treating the whole thing as a value pattern.
func namedBinders(tc *ast.TableConstructor) ([]ast.MatchFieldBind, bool) {
	if len(tc.Fields) == 0 {
		return nil, false
	}
	out := make([]ast.MatchFieldBind, 0, len(tc.Fields))
	for _, f := range tc.Fields {
		if f.IsBracketed || f.Key == nil {
			return nil, false
		}
		key, ok := f.Key.(*ast.Identifier)
		if !ok {
			return nil, false
		}
		val, ok := f.Value.(*ast.Identifier)
		if !ok {
			return nil, false
		}
		out = append(out, ast.MatchFieldBind{Field: key.Name, Bind: val.Name})
	}
	return out, true
}

// reportArrowError emits a precise message for the common arm-separator
// mistakes (`then`, `=>`, `:`) before the generic "expected ->".
func (p *Parser) reportArrowError() {
	switch {
	case p.curTokenIs(token.Then):
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got 'then'",
			"`match` uses '->' as the arm separator, not 'then'")
	case p.curTokenIs(token.Assign) && p.peekTokenIs(token.GT):
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got '=>'",
			"`match` uses '->' (Lua arrow) as the arm separator, not '=>'")
	default:
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got "+describeToken(p.curToken),
			"each arm has the form `<pattern> [if <guard>] -> <stmt>`")
	}
}
