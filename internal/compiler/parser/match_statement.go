package parser

// `match` is a pure parser-level desugar — no new persistent AST node, no
// codegen change, no typecheck rule. It rewrites to a `do` block that binds
// the scrutinee once and then runs a sequence of guarded `if` blocks driven
// by a `__matched` flag:
//
//	match <expr> do
//	  <pattern> [if <guard>] -> <stmt>
//	  ...
//	  _ -> <stmt>
//	end
//
// lowers (conceptually) to
//
//	do
//	  local __match_N   = <expr>
//	  local __matched_N = false
//	  if not __matched_N and <test-1> then <binds-1>; if <guard-1> then __matched_N = true; <body-1> end end
//	  if not __matched_N and <test-2> then ... end
//	  if not __matched_N then <body-wildcard> end        -- the `_` arm
//	end
//
// The `__matched` flag (rather than an `if/elseif` chain) is what makes
// *guards* correct: a matched-but-guard-failed arm leaves the flag false and
// control falls through to later arms. It also lets pattern bindings live in
// an ordinary local scope visible to both the guard and the body.
//
// Pattern forms:
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
// Value patterns keep `match`'s original single-evaluation semantics and are
// fully backward compatible; the binding/destructure/guard forms are the v2
// additions.

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

// matchSyntax is the canonical `match` syntax shown in user-facing error
// hints. Kept in one place so we don't drift if the form changes.
const matchSyntax = "match <expr> do <pattern> [if <guard>] -> <stmt> ... [_ -> <stmt>] end"

// mpKind tags the parsed shape of a single pattern.
type mpKind int

const (
	mpValue            mpKind = iota // scrutinee == expr
	mpWildcard                       // _
	mpTyped                          // name : Type   (name may be "_" / "" = no bind)
	mpDestructurePos                 // Path(a, b, _)
	mpDestructureNamed               // Path{ f = a }
)

// mNamed is one `field = binder` entry of a named (struct) destructure.
type mNamed struct {
	field string
	bind  string // "_" means "match but don't bind"
}

// mpattern is the parser's internal representation of one arm pattern. It is
// consumed immediately to build the arm's test + bindings and never reaches
// the AST.
type mpattern struct {
	kind      mpKind
	value     ast.Expression // mpValue
	bind      string         // mpTyped: binder name ("" / "_" = none)
	typ       ast.TypeNode   // mpTyped
	tag       string         // destructure: last path segment (variant/struct name)
	posBind   []string       // mpDestructurePos: positional binders ("_" = skip)
	namedBind []mNamed       // mpDestructureNamed
}

// parseMatchStatement consumes a `match ... end` block and returns the
// equivalent `do` block described in the file comment.
func (p *Parser) parseMatchStatement() ast.Statement {
	matchTok := p.curToken
	p.nextToken() // consume 'match'

	if p.curTokenIs(token.Do) || p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"expected an expression to scrutinise after 'match', got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}

	scrutinee := p.parseExpression()
	if scrutinee == nil {
		return nil
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected 'do' after the scrutinee expression, got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}
	p.nextToken() // consume 'do'

	// Fresh binding names per `match` keep nested matches unambiguous.
	p.matchCounter++
	subject := fmt.Sprintf("__match_%d", p.matchCounter)
	matched := fmt.Sprintf("__matched_%d", p.matchCounter)

	var armStmts []ast.Statement
	sawWildcard := false

	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(matchTok, errors.EndOfFileError, "match",
				fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
				"each arm is `<pattern> [if <guard>] -> <stmt>`; close the whole construct with `end`")
			return nil
		}

		armTok := p.curToken

		patterns, ok := p.parseArmPatterns()
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

		isWildcardArm := len(patterns) == 1 && patterns[0].kind == mpWildcard && guard == nil
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

		armStmts = append(armStmts, p.buildArm(armTok, subject, matched, patterns, guard, body))
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(matchTok, errors.UnexpectedTokenError, "match",
			fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'

	stmts := []ast.Statement{buildLocal(subject, matchTok, scrutinee)}
	if len(armStmts) > 0 {
		// Only introduce the flag when there is at least one arm to gate.
		stmts = append(stmts, buildLocal(matched, matchTok, &ast.BooleanLiteral{BaseNode: baseAt(matchTok), Value: false}))
		stmts = append(stmts, armStmts...)
	}
	return &ast.DoStatement{
		BaseNode: baseAt(matchTok),
		Body: &ast.Block{
			BaseNode:   ast.BaseNode{Token: matchTok},
			Statements: stmts,
		},
	}
}

// parseArmPatterns reads the pattern portion of an arm: either a single
// binding/destructure/wildcard pattern, or a comma-separated list of value
// patterns. Returns the patterns and whether parsing succeeded.
func (p *Parser) parseArmPatterns() ([]mpattern, bool) {
	first, ok := p.parseOnePattern()
	if !ok {
		return nil, false
	}
	patterns := []mpattern{first}

	// Only value patterns may be joined with commas: alternatives with
	// bindings would leave the body unsure which binder is live.
	for p.curTokenIs(token.Comma) {
		if first.kind != mpValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return nil, false
		}
		p.nextToken() // consume ','
		next, ok := p.parseOnePattern()
		if !ok {
			return nil, false
		}
		if next.kind != mpValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return nil, false
		}
		patterns = append(patterns, next)
	}
	return patterns, true
}

// parseOnePattern parses a single pattern, dispatching on the leading tokens.
func (p *Parser) parseOnePattern() (mpattern, bool) {
	// `_` — wildcard or typed no-bind (`_ : T`).
	if p.curTokenIs(token.Ident) && p.curToken.Literal == "_" {
		if p.peekTokenIs(token.Colon) {
			return p.parseTypedPattern("_")
		}
		p.nextToken() // consume '_'
		return mpattern{kind: mpWildcard}, true
	}

	// `name : Type` — typed binding.
	if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon) {
		name := p.curToken.Literal
		return p.parseTypedPattern(name)
	}

	// Otherwise parse a full expression and classify it. A call-shaped
	// expression whose arguments are plain binders is a destructure; anything
	// else is a value pattern compared with `==`.
	expr := p.parseExpression()
	if expr == nil {
		return mpattern{}, false
	}
	return classifyPattern(expr), true
}

// parseTypedPattern reads the `Type` half of a `name : Type` pattern. The
// cursor is on `name` at entry.
func (p *Parser) parseTypedPattern(name string) (mpattern, bool) {
	p.nextToken() // consume name/underscore
	p.nextToken() // consume ':'
	ty := p.parseType()
	if ty == nil {
		return mpattern{}, false
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
		return mpattern{}, false
	}
	return mpattern{kind: mpTyped, bind: name, typ: ty}, true
}

// classifyPattern decides whether an already-parsed expression is a
// destructure pattern (call-shaped with binder arguments) or a plain value
// pattern.
func classifyPattern(expr ast.Expression) mpattern {
	call, ok := expr.(*ast.CallExpression)
	if !ok {
		return mpattern{kind: mpValue, value: expr}
	}
	seg, ok := dottedTail(call.Func)
	if !ok {
		return mpattern{kind: mpValue, value: expr}
	}

	// Named form: `Name{ field = binder, ... }` (folded to a single
	// table-constructor argument).
	if len(call.Args) == 1 {
		if tc, ok := call.Args[0].(*ast.TableConstructor); ok {
			if binders, ok := namedBinders(tc); ok {
				return mpattern{kind: mpDestructureNamed, tag: seg, namedBind: binders}
			}
			return mpattern{kind: mpValue, value: expr}
		}
	}

	// Positional form: `Name(a, b, _)` — every argument must be a plain
	// identifier (a binder) or `_`.
	binders := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		id, ok := a.(*ast.Identifier)
		if !ok {
			return mpattern{kind: mpValue, value: expr}
		}
		binders = append(binders, id.Name)
	}
	return mpattern{kind: mpDestructurePos, tag: seg, posBind: binders}
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
func namedBinders(tc *ast.TableConstructor) ([]mNamed, bool) {
	if len(tc.Fields) == 0 {
		return nil, false
	}
	out := make([]mNamed, 0, len(tc.Fields))
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
		out = append(out, mNamed{field: key.Name, bind: val.Name})
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

// ---------------------------------------------------------------------------
// Arm lowering
// ---------------------------------------------------------------------------

// buildArm builds one arm's gated `if` block:
//
//	if not <matched> [and <test>] then
//	    <bindings>
//	    [if <guard> then] <matched> = true; <body> [end]
//	end
func (p *Parser) buildArm(tok token.Token, subject, matched string, patterns []mpattern, guard ast.Expression, body ast.Statement) ast.Statement {
	binds := p.patternBindings(tok, subject, patterns)

	// Inner: set matched, run body (optionally gated by the guard).
	inner := []ast.Statement{
		assign(matched, tok, &ast.BooleanLiteral{BaseNode: baseAt(tok), Value: true}),
		body,
	}
	if guard != nil {
		inner = []ast.Statement{&ast.IfStatement{
			BaseNode: baseAt(tok),
			Clauses: []ast.IfClause{{
				Condition: guard,
				Body:      blockOf(tok, inner),
			}},
		}}
	}

	thenStmts := append(binds, inner...)

	// Condition: `not matched` followed by the pattern's conjuncts. Folding
	// LEFT-associatively matches how Lua parses `a and b and c`, so the
	// emitted tree round-trips through the formatter unchanged.
	conjuncts := append([]ast.Expression{
		&ast.UnaryExpression{BaseNode: baseAt(tok), Op: "not", Operand: ident(matched, tok)},
	}, p.patternConjuncts(tok, subject, patterns)...)

	return &ast.IfStatement{
		BaseNode: baseAt(tok),
		Clauses: []ast.IfClause{{
			Condition: foldAnd(tok, conjuncts),
			Body:      blockOf(tok, thenStmts),
		}},
	}
}

// foldAnd combines conjuncts into a left-associative `and` chain.
func foldAnd(tok token.Token, conjuncts []ast.Expression) ast.Expression {
	cond := conjuncts[0]
	for _, c := range conjuncts[1:] {
		cond = &ast.BinaryExpression{BaseNode: baseAt(tok), Op: "and", Left: cond, Right: c}
	}
	return cond
}

// patternConjuncts builds the list of boolean conjuncts an arm's pattern(s)
// contributes to its `if` condition. Empty when the pattern always matches
// (a wildcard, or `: any`). Returning a flat list (rather than a pre-built
// `and` tree) lets buildArm fold everything — including the leading
// `not matched` — into one left-associative chain.
func (p *Parser) patternConjuncts(tok token.Token, subject string, patterns []mpattern) []ast.Expression {
	// Value-pattern alternatives OR together into a single equality conjunct.
	if patterns[0].kind == mpValue {
		var cond ast.Expression
		for _, pat := range patterns {
			eq := &ast.BinaryExpression{
				BaseNode: baseAt(tok),
				Op:       "==",
				Left:     ident(subject, tok),
				Right:    pat.value,
			}
			if cond == nil {
				cond = eq
			} else {
				cond = &ast.BinaryExpression{BaseNode: baseAt(tok), Op: "or", Left: cond, Right: eq}
			}
		}
		return []ast.Expression{cond}
	}

	pat := patterns[0]
	switch pat.kind {
	case mpWildcard:
		return nil
	case mpTyped:
		if t := typeTest(tok, subject, pat.typ); t != nil {
			return []ast.Expression{t}
		}
		return nil
	case mpDestructurePos:
		// type(subject) == "table"  AND  subject.__tag == "<tag>"
		isTable := &ast.BinaryExpression{
			BaseNode: baseAt(tok), Op: "==",
			Left:  callGlobal("type", tok, ident(subject, tok)),
			Right: strLit("table", tok),
		}
		tagEq := &ast.BinaryExpression{
			BaseNode: baseAt(tok), Op: "==",
			Left:  field(subject, "__tag", tok),
			Right: strLit(pat.tag, tok),
		}
		return []ast.Expression{isTable, tagEq}
	case mpDestructureNamed:
		// typeof(subject) == "<tag>"
		return []ast.Expression{&ast.BinaryExpression{
			BaseNode: baseAt(tok), Op: "==",
			Left:  callGlobal("typeof", tok, ident(subject, tok)),
			Right: strLit(pat.tag, tok),
		}}
	}
	return nil
}

// patternBindings builds the `local <name> = <projection>` statements a
// binding/destructure pattern introduces. Value and wildcard patterns bind
// nothing.
func (p *Parser) patternBindings(tok token.Token, subject string, patterns []mpattern) []ast.Statement {
	pat := patterns[0]
	var out []ast.Statement
	switch pat.kind {
	case mpTyped:
		if pat.bind != "" && pat.bind != "_" {
			out = append(out, buildLocal(pat.bind, tok, ident(subject, tok)))
		}
	case mpDestructurePos:
		for i, name := range pat.posBind {
			if name == "_" {
				continue
			}
			out = append(out, buildLocal(name, tok, index(subject, int64(i+1), tok)))
		}
	case mpDestructureNamed:
		for _, nb := range pat.namedBind {
			if nb.bind == "_" {
				continue
			}
			out = append(out, buildLocal(nb.bind, tok, field(subject, nb.field, tok)))
		}
	}
	return out
}

// typeTest builds the runtime type test for a typed pattern. A TypePrimitive
// probes the Lua-level `type()`; a TypeName probes `typeof()` (which reports
// the nominal `__type` of structs and tagged-enum values). `any` always
// matches (nil test).
func typeTest(tok token.Token, subject string, ty ast.TypeNode) ast.Expression {
	switch t := ty.(type) {
	case *ast.TypePrimitive:
		switch t.Name {
		case "any", "unknown":
			return nil // matches anything
		case "nil":
			return &ast.BinaryExpression{
				BaseNode: baseAt(tok), Op: "==",
				Left: ident(subject, tok), Right: &ast.NilLiteral{BaseNode: baseAt(tok)},
			}
		default:
			return &ast.BinaryExpression{
				BaseNode: baseAt(tok), Op: "==",
				Left:  callGlobal("type", tok, ident(subject, tok)),
				Right: strLit(t.Name, tok),
			}
		}
	case *ast.TypeName:
		return &ast.BinaryExpression{
			BaseNode: baseAt(tok), Op: "==",
			Left:  callGlobal("typeof", tok, ident(subject, tok)),
			Right: strLit(t.Name, tok),
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Small AST builders
// ---------------------------------------------------------------------------

func ident(name string, tok token.Token) *ast.Identifier {
	return &ast.Identifier{BaseNode: baseAt(tok), Name: name}
}

func strLit(v string, tok token.Token) *ast.StringLiteral {
	return &ast.StringLiteral{BaseNode: baseAt(tok), Value: v}
}

// field builds `obj.name`.
func field(obj, name string, tok token.Token) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   ident(obj, tok),
		Index:    strLit(name, tok),
		IsDot:    true,
	}
}

// index builds `obj[i]`.
func index(obj string, i int64, tok token.Token) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   ident(obj, tok),
		Index:    &ast.IntegerLiteral{BaseNode: baseAt(tok), Value: i},
	}
}

// callGlobal builds `fn(arg)`.
func callGlobal(fn string, tok token.Token, arg ast.Expression) *ast.CallExpression {
	return &ast.CallExpression{
		BaseNode: baseAt(tok),
		Func:     ident(fn, tok),
		Args:     []ast.Expression{arg},
	}
}

// assign builds `name = value`.
func assign(name string, tok token.Token, value ast.Expression) *ast.AssignStatement {
	return &ast.AssignStatement{
		BaseNode: baseAt(tok),
		Targets:  []ast.Expression{ident(name, tok)},
		Values:   []ast.Expression{value},
	}
}

// buildLocal builds `local name = value`.
func buildLocal(name string, tok token.Token, value ast.Expression) *ast.LocalStatement {
	return &ast.LocalStatement{
		BaseNode: baseAt(tok),
		Names:    []ast.LocalName{{Name: name}},
		Values:   []ast.Expression{value},
	}
}

// blockOf wraps statements in a Block.
func blockOf(tok token.Token, stmts []ast.Statement) *ast.Block {
	return &ast.Block{BaseNode: ast.BaseNode{Token: tok}, Statements: stmts}
}
