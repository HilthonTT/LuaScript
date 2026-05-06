package parser

import (
	"testing"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/lexer"
)

func parse(t *testing.T, src string) *ast.Program {
	t.Helper()
	p := New(lexer.New(src))
	prog, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse error for %q: %s", src, err.Message)
	}
	if prog == nil || prog.Block == nil {
		t.Fatalf("parser returned nil program for %q", src)
	}
	return prog
}

func parseExpect1(t *testing.T, src string) ast.Statement {
	t.Helper()
	prog := parse(t, src)
	if len(prog.Block.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d for %q\nstatements: %#v",
			len(prog.Block.Statements), src, prog.Block.Statements)
	}
	return prog.Block.Statements[0]
}

func parseError(t *testing.T, src string) string {
	t.Helper()
	p := New(lexer.New(src))
	_, err := p.ParseProgram()
	if err == nil {
		t.Fatalf("expected parse error for %q, got none", src)
	}
	return err.Message
}

func TestParseAtomicLiterals(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"local a = nil", "nil"},
		{"local a = true", "true"},
		{"local a = false", "false"},
		{"local a = 42", "42"},
		{"local a = 3.14", "3.14"},
		{`local a = "hi"`, `"hi"`},
		{"local a = ...", "..."},
	}
	// `...` is only valid in vararg-functions, but parseExpression doesn't
	// enforce that — semantic checks happen later.
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ls := parseExpect1(t, c.src).(*ast.LocalStatement)
			if got := ls.Values[0].String(); got != c.want {
				t.Errorf("expr = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseIntegerHex(t *testing.T) {
	ls := parseExpect1(t, "local a = 0xFF").(*ast.LocalStatement)
	il, ok := ls.Values[0].(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("got %T, want IntegerLiteral", ls.Values[0])
	}
	if il.Value != 255 {
		t.Errorf("value = %d, want 255", il.Value)
	}
}

func TestOperatorPrecedence(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		// Arithmetic associativity & precedence
		{"local a = 1 + 2 * 3", "(1 + (2 * 3))"},
		{"local a = (1 + 2) * 3", "(((1 + 2)) * 3)"}, // ParenExpression is preserved (multi→one adjustment); double-paren in render is expected
		{"local a = 1 - 2 - 3", "((1 - 2) - 3)"},     // left-assoc
		{"local a = 2 ^ 3 ^ 2", "(2 ^ (3 ^ 2))"},     // right-assoc
		{"local a = -2 ^ 2", "(-(2 ^ 2))"},           // ^ tighter than unary -
		{"local a = 1 .. 2 .. 3", "(1 .. (2 .. 3))"}, // right-assoc
		// or < and < compare
		{"local a = a or b and c", "(a or (b and c))"},
		{"local a = a < b == c", "((a < b) == c)"},
		// Bitwise ladder: | < ~ < & < << = >>
		{"local a = a | b ~ c", "(a | (b ~ c))"},
		{"local a = a ~ b & c", "(a ~ (b & c))"},
		{"local a = a & b << c", "(a & (b << c))"},
		// Comparison chain (left-assoc, but Lua has no chained comparison
		// semantics — this is just associativity).
		{"local a = 1 < 2 < 3", "((1 < 2) < 3)"},
		// not / # / unary -
		{"local a = not a and b", "((not a) and b)"},
		{"local a = #t + 1", "((#t) + 1)"},
		// .. binds tighter than comparison, looser than +
		{"local a = a .. b == c", "((a .. b) == c)"},
		{"local a = 1 + 2 .. 3", "((1 + 2) .. 3)"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ls := parseExpect1(t, c.src).(*ast.LocalStatement)
			if got := ls.Values[0].String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestLocalMultiNameMultiValue(t *testing.T) {
	ls := parseExpect1(t, "local a, b, c = 1, 2, 3").(*ast.LocalStatement)
	if len(ls.Names) != 3 {
		t.Fatalf("names = %d, want 3", len(ls.Names))
	}
	if len(ls.Values) != 3 {
		t.Fatalf("values = %d, want 3", len(ls.Values))
	}
}

func TestLocalConstAttrib(t *testing.T) {
	ls := parseExpect1(t, "local x <const> = 1").(*ast.LocalStatement)
	if ls.Names[0].Attrib != "const" {
		t.Errorf("attrib = %q, want %q", ls.Names[0].Attrib, "const")
	}
}

func TestLocalCloseAttrib(t *testing.T) {
	ls := parseExpect1(t, "local x <close> = io.open()").(*ast.LocalStatement)
	if ls.Names[0].Attrib != "close" {
		t.Errorf("attrib = %q, want %q", ls.Names[0].Attrib, "close")
	}
}

func TestLocalFunctionStatement(t *testing.T) {
	lf := parseExpect1(t, "local function f(x, y) return x + y end").(*ast.LocalFunctionStatement)
	if lf.Name != "f" {
		t.Errorf("name = %q, want %q", lf.Name, "f")
	}
	if len(lf.Func.Params) != 2 {
		t.Errorf("params = %d, want 2", len(lf.Func.Params))
	}
	if lf.Func.Body.Return == nil {
		t.Errorf("expected return statement in body")
	}
}

func TestFunctionDeclarationPlain(t *testing.T) {
	fd := parseExpect1(t, "function f() end").(*ast.FunctionDeclaration)
	if fd.Name.Name != "f" {
		t.Errorf("name = %q, want f", fd.Name.Name)
	}
	if len(fd.DottedFields) != 0 || fd.MethodName != "" {
		t.Errorf("expected no dotted/method, got %v / %q", fd.DottedFields, fd.MethodName)
	}
}

func TestFunctionDeclarationDotted(t *testing.T) {
	fd := parseExpect1(t, "function t.a.b() end").(*ast.FunctionDeclaration)
	if fd.Name.Name != "t" {
		t.Errorf("base = %q, want t", fd.Name.Name)
	}
	if len(fd.DottedFields) != 2 || fd.DottedFields[0] != "a" || fd.DottedFields[1] != "b" {
		t.Errorf("dotted = %v, want [a b]", fd.DottedFields)
	}
	if fd.MethodName != "" {
		t.Errorf("method = %q, want empty", fd.MethodName)
	}
}

func TestFunctionDeclarationMethod(t *testing.T) {
	fd := parseExpect1(t, "function obj:greet(s) return s end").(*ast.FunctionDeclaration)
	if fd.MethodName != "greet" {
		t.Errorf("method = %q, want greet", fd.MethodName)
	}
}

func TestFunctionVararg(t *testing.T) {
	fd := parseExpect1(t, "function f(a, ...) end").(*ast.FunctionDeclaration)
	if !fd.Func.IsVararg {
		t.Errorf("IsVararg = false, want true")
	}
	if len(fd.Func.Params) != 1 {
		t.Errorf("params = %d, want 1 (a; ... is the vararg flag)", len(fd.Func.Params))
	}
}

func TestIfElseifElse(t *testing.T) {
	src := `if a then x = 1 elseif b then x = 2 elseif c then x = 3 else x = 4 end`
	is := parseExpect1(t, src).(*ast.IfStatement)
	if len(is.Clauses) != 3 {
		t.Errorf("clauses = %d, want 3", len(is.Clauses))
	}
	if is.Else == nil {
		t.Errorf("expected else block")
	}
}

func TestWhileWithBreak(t *testing.T) {
	ws := parseExpect1(t, "while true do break end").(*ast.WhileStatement)
	if len(ws.Body.Statements) != 1 {
		t.Fatalf("body stmts = %d, want 1", len(ws.Body.Statements))
	}
	if _, ok := ws.Body.Statements[0].(*ast.BreakStatement); !ok {
		t.Errorf("body[0] = %T, want BreakStatement", ws.Body.Statements[0])
	}
}

func TestRepeatUntil(t *testing.T) {
	rs := parseExpect1(t, "repeat x = x + 1 until x > 10").(*ast.RepeatStatement)
	if rs.Condition == nil {
		t.Fatalf("nil condition")
	}
	if got := rs.Condition.String(); got != "(x > 10)" {
		t.Errorf("condition = %q, want %q", got, "(x > 10)")
	}
}

func TestNumericFor(t *testing.T) {
	fs := parseExpect1(t, "for i = 1, 10 do end").(*ast.NumericForStatement)
	if fs.Name != "i" {
		t.Errorf("name = %q, want i", fs.Name)
	}
	if fs.Step != nil {
		t.Errorf("expected nil step (omitted)")
	}
}

func TestNumericForWithStep(t *testing.T) {
	fs := parseExpect1(t, "for i = 10, 1, -1 do end").(*ast.NumericForStatement)
	if fs.Step == nil {
		t.Fatalf("expected step expression")
	}
	if got := fs.Step.String(); got != "(-1)" {
		t.Errorf("step = %q, want (-1)", got)
	}
}

func TestGenericFor(t *testing.T) {
	fs := parseExpect1(t, "for k, v in pairs(t) do end").(*ast.GenericForStatement)
	if len(fs.Names) != 2 || fs.Names[0] != "k" || fs.Names[1] != "v" {
		t.Errorf("names = %v, want [k v]", fs.Names)
	}
	if len(fs.Exprs) != 1 {
		t.Errorf("exprs = %d, want 1", len(fs.Exprs))
	}
	if _, ok := fs.Exprs[0].(*ast.CallExpression); !ok {
		t.Errorf("expr[0] = %T, want CallExpression", fs.Exprs[0])
	}
}

func TestDoBlock(t *testing.T) {
	ds := parseExpect1(t, "do local x = 1 end").(*ast.DoStatement)
	if len(ds.Body.Statements) != 1 {
		t.Errorf("body stmts = %d, want 1", len(ds.Body.Statements))
	}
}

func TestGotoAndLabel(t *testing.T) {
	prog := parse(t, "::start:: goto start")
	stmts := prog.Block.Statements
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
	lab, ok := stmts[0].(*ast.LabelStatement)
	if !ok || lab.Name != "start" {
		t.Errorf("stmt[0] = %#v, want label `start`", stmts[0])
	}
	g, ok := stmts[1].(*ast.GotoStatement)
	if !ok || g.Label != "start" {
		t.Errorf("stmt[1] = %#v, want goto `start`", stmts[1])
	}
}

func TestSimpleAssignment(t *testing.T) {
	as := parseExpect1(t, "x = 1").(*ast.AssignStatement)
	if len(as.Targets) != 1 || len(as.Values) != 1 {
		t.Errorf("got %d targets, %d values", len(as.Targets), len(as.Values))
	}
}

func TestMultipleAssignment(t *testing.T) {
	as := parseExpect1(t, "a, b, c = 1, 2, 3").(*ast.AssignStatement)
	if len(as.Targets) != 3 || len(as.Values) != 3 {
		t.Errorf("got %d targets, %d values", len(as.Targets), len(as.Values))
	}
}

func TestIndexAssignmentDot(t *testing.T) {
	as := parseExpect1(t, "t.x = 1").(*ast.AssignStatement)
	idx, ok := as.Targets[0].(*ast.IndexExpression)
	if !ok {
		t.Fatalf("target = %T, want IndexExpression", as.Targets[0])
	}
	if !idx.IsDot {
		t.Errorf("expected IsDot=true")
	}
}

func TestIndexAssignmentBracket(t *testing.T) {
	as := parseExpect1(t, "t[1] = 1").(*ast.AssignStatement)
	idx, ok := as.Targets[0].(*ast.IndexExpression)
	if !ok || idx.IsDot {
		t.Fatalf("target = %#v, want bracket IndexExpression", as.Targets[0])
	}
}

func TestInvalidAssignmentTarget(t *testing.T) {
	msg := parseError(t, "1 = 2")
	if msg == "" {
		t.Errorf("expected non-empty error message")
	}
}

func TestCallExpressionStatement(t *testing.T) {
	es := parseExpect1(t, "print(1, 2)").(*ast.ExpressionStatement)
	ce, ok := es.Expression.(*ast.CallExpression)
	if !ok {
		t.Fatalf("expr = %T, want CallExpression", es.Expression)
	}
	if len(ce.Args) != 2 {
		t.Errorf("args = %d, want 2", len(ce.Args))
	}
}

func TestMethodCallStatement(t *testing.T) {
	es := parseExpect1(t, "obj:do_thing(1)").(*ast.ExpressionStatement)
	mc, ok := es.Expression.(*ast.MethodCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want MethodCallExpression", es.Expression)
	}
	if mc.Method != "do_thing" {
		t.Errorf("method = %q, want do_thing", mc.Method)
	}
}

func TestCallSugarString(t *testing.T) {
	es := parseExpect1(t, `print "hello"`).(*ast.ExpressionStatement)
	ce, ok := es.Expression.(*ast.CallExpression)
	if !ok {
		t.Fatalf("expr = %T, want CallExpression", es.Expression)
	}
	if len(ce.Args) != 1 {
		t.Fatalf("args = %d, want 1", len(ce.Args))
	}
	if _, ok := ce.Args[0].(*ast.StringLiteral); !ok {
		t.Errorf("arg = %T, want StringLiteral", ce.Args[0])
	}
}

func TestCallSugarTable(t *testing.T) {
	es := parseExpect1(t, "f { 1, 2 }").(*ast.ExpressionStatement)
	ce, ok := es.Expression.(*ast.CallExpression)
	if !ok {
		t.Fatalf("expr = %T, want CallExpression", es.Expression)
	}
	if len(ce.Args) != 1 {
		t.Fatalf("args = %d, want 1", len(ce.Args))
	}
	if _, ok := ce.Args[0].(*ast.TableConstructor); !ok {
		t.Errorf("arg = %T, want TableConstructor", ce.Args[0])
	}
}

func TestTableConstructorAllFieldForms(t *testing.T) {
	src := `local t = { 1, 2, x = 10, [99] = "k", 3 }`
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	tc, ok := ls.Values[0].(*ast.TableConstructor)
	if !ok {
		t.Fatalf("value = %T, want TableConstructor", ls.Values[0])
	}
	if len(tc.Fields) != 5 {
		t.Fatalf("fields = %d, want 5", len(tc.Fields))
	}
	// Field 0: positional
	if tc.Fields[0].Key != nil {
		t.Errorf("field[0] should be array-positional (Key=nil)")
	}
	// Field 2: record
	if _, ok := tc.Fields[2].Key.(*ast.Identifier); !ok || tc.Fields[2].IsBracketed {
		t.Errorf("field[2] should be record (Ident key, not bracketed)")
	}
	// Field 3: bracketed
	if !tc.Fields[3].IsBracketed {
		t.Errorf("field[3] should be bracketed")
	}
}

func TestTableConstructorEmpty(t *testing.T) {
	ls := parseExpect1(t, "local t = {}").(*ast.LocalStatement)
	tc := ls.Values[0].(*ast.TableConstructor)
	if len(tc.Fields) != 0 {
		t.Errorf("fields = %d, want 0", len(tc.Fields))
	}
}

func TestTableConstructorTrailingSeparator(t *testing.T) {
	ls := parseExpect1(t, "local t = { 1, 2, 3, }").(*ast.LocalStatement)
	tc := ls.Values[0].(*ast.TableConstructor)
	if len(tc.Fields) != 3 {
		t.Errorf("fields = %d, want 3", len(tc.Fields))
	}
}

func TestTableConstructorSemicolonSeparator(t *testing.T) {
	ls := parseExpect1(t, "local t = { 1; 2; x = 3 }").(*ast.LocalStatement)
	tc := ls.Values[0].(*ast.TableConstructor)
	if len(tc.Fields) != 3 {
		t.Errorf("fields = %d, want 3", len(tc.Fields))
	}
}

func TestReturnEmpty(t *testing.T) {
	prog := parse(t, "return")
	if prog.Block.Return == nil {
		t.Fatalf("nil return")
	}
	if len(prog.Block.Return.Values) != 0 {
		t.Errorf("values = %d, want 0", len(prog.Block.Return.Values))
	}
}

func TestReturnMultiple(t *testing.T) {
	prog := parse(t, "return 1, 2, 3")
	if len(prog.Block.Return.Values) != 3 {
		t.Errorf("values = %d, want 3", len(prog.Block.Return.Values))
	}
}

func TestReturnSemicolon(t *testing.T) {
	prog := parse(t, "return 1, 2;")
	if len(prog.Block.Return.Values) != 2 {
		t.Errorf("values = %d, want 2", len(prog.Block.Return.Values))
	}
}

func TestParenExpressionPreserved(t *testing.T) {
	ls := parseExpect1(t, "local a = (f())").(*ast.LocalStatement)
	if _, ok := ls.Values[0].(*ast.ParenExpression); !ok {
		t.Errorf("value = %T, want ParenExpression (preserves multi→one adjustment)", ls.Values[0])
	}
}

func TestPostfixChain(t *testing.T) {
	// a.b[1]:m(2).c
	es := parseExpect1(t, "a.b[1]:m(2).c = 3").(*ast.AssignStatement)
	idx, ok := es.Targets[0].(*ast.IndexExpression)
	if !ok || !idx.IsDot {
		t.Fatalf("target = %#v, want dot IndexExpression", es.Targets[0])
	}
	mc, ok := idx.Object.(*ast.MethodCallExpression)
	if !ok {
		t.Fatalf("inner = %T, want MethodCallExpression", idx.Object)
	}
	if mc.Method != "m" {
		t.Errorf("method = %q, want m", mc.Method)
	}
}

func TestUnclosedBlockError(t *testing.T) {
	msg := parseError(t, "if true then x = 1")
	if msg == "" {
		t.Errorf("expected error for unclosed if")
	}
}

func TestUnexpectedTokenAtChunkStart(t *testing.T) {
	msg := parseError(t, ")")
	if msg == "" {
		t.Errorf("expected error for stray `)`")
	}
}

func TestSmokeProgramParses(t *testing.T) {
	src := `
local function fib(n)
  if n < 2 then return n end
  return fib(n - 1) + fib(n - 2)
end

local t = { 1, 2, 3 }
for i, v in ipairs(t) do
  print(i, v, fib(v))
end

local s = "hello" .. " " .. "world"
local x <const> = 42
return fib(10)
`
	prog := parse(t, src)
	if len(prog.Block.Statements) < 4 {
		t.Errorf("expected at least 4 top-level statements, got %d", len(prog.Block.Statements))
	}
	if prog.Block.Return == nil {
		t.Errorf("expected trailing return")
	}
}
