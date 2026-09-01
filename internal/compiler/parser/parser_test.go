package parser

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
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
		{"local a = 1 + 2 * 3", "(1 + (2 * 3))"},
		{"local a = (1 + 2) * 3", "(((1 + 2)) * 3)"},
		{"local a = 1 - 2 - 3", "((1 - 2) - 3)"},
		{"local a = 2 ^ 3 ^ 2", "(2 ^ (3 ^ 2))"},
		{"local a = -2 ^ 2", "(-(2 ^ 2))"},
		{"local a = 1 .. 2 .. 3", "(1 .. (2 .. 3))"},
		{"local a = a or b and c", "(a or (b and c))"},
		{"local a = a < b == c", "((a < b) == c)"},
		{"local a = a | b ~ c", "(a | (b ~ c))"},
		{"local a = a ~ b & c", "(a ~ (b & c))"},
		{"local a = a & b << c", "(a & (b << c))"},
		{"local a = 1 < 2 < 3", "((1 < 2) < 3)"},
		{"local a = not a and b", "((not a) and b)"},
		{"local a = #t + 1", "((#t) + 1)"},
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
	if tc.Fields[0].Key != nil {
		t.Errorf("field[0] should be array-positional (Key=nil)")
	}
	if _, ok := tc.Fields[2].Key.(*ast.Identifier); !ok || tc.Fields[2].IsBracketed {
		t.Errorf("field[2] should be record (Ident key, not bracketed)")
	}
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

func matchStmt(t *testing.T, src string) *ast.MatchStatement {
	t.Helper()
	prog := parse(t, src)
	if len(prog.Block.Statements) == 0 {
		t.Fatalf("no statements parsed for %q", src)
	}
	stmt := prog.Block.Statements[len(prog.Block.Statements)-1]
	ms, ok := stmt.(*ast.MatchStatement)
	if !ok {
		t.Fatalf("match did not parse to *ast.MatchStatement, got %T", stmt)
	}
	return ms
}

func armPattern(t *testing.T, ms *ast.MatchStatement, i int) *ast.MatchPattern {
	t.Helper()
	if i >= len(ms.Arms) {
		t.Fatalf("arm %d out of range (%d arms)", i, len(ms.Arms))
	}
	return &ms.Arms[i].Pattern
}

func TestMatchSingleArm(t *testing.T) {
	ms := matchStmt(t, "match x do 1 -> print(\"one\") end")
	if ms.Subject.String() != "x" {
		t.Errorf("subject = %q, want x", ms.Subject.String())
	}
	if len(ms.Arms) != 1 {
		t.Fatalf("arms = %d, want 1", len(ms.Arms))
	}
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchValue {
		t.Fatalf("kind = %v, want MatchValue", p.Kind)
	}
	if len(p.Values) != 1 || p.Values[0].String() != "1" {
		t.Errorf("values = %v, want [1]", p.Values)
	}
}

func TestMatchMultipleArms(t *testing.T) {
	ms := matchStmt(t, `match x do
1 -> print("one")
2 -> print("two")
3 -> print("three")
end`)
	if len(ms.Arms) != 3 {
		t.Fatalf("arms = %d, want 3", len(ms.Arms))
	}
}

func TestMatchMultiPatternArm(t *testing.T) {
	ms := matchStmt(t, `match x do
1, 2, 3 -> print("small")
end`)
	if len(ms.Arms) != 1 {
		t.Fatalf("arms = %d, want 1", len(ms.Arms))
	}
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchValue {
		t.Fatalf("kind = %v, want MatchValue", p.Kind)
	}
	if len(p.Values) != 3 {
		t.Fatalf("values = %d, want 3", len(p.Values))
	}
	for i, want := range []string{"1", "2", "3"} {
		if got := p.Values[i].String(); got != want {
			t.Errorf("value %d = %q, want %q", i, got, want)
		}
	}
}

func TestMatchWildcard(t *testing.T) {
	ms := matchStmt(t, `match x do
1 -> print("one")
_ -> print("other")
end`)
	if len(ms.Arms) != 2 {
		t.Fatalf("arms = %d, want 2", len(ms.Arms))
	}
	if k := armPattern(t, ms, 1).Kind; k != ast.MatchWildcard {
		t.Errorf("second arm kind = %v, want MatchWildcard", k)
	}
}

func TestMatchWildcardOnlyArm(t *testing.T) {
	ms := matchStmt(t, "match x do _ -> print(\"any\") end")
	if len(ms.Arms) != 1 {
		t.Fatalf("arms = %d, want 1", len(ms.Arms))
	}
	if k := armPattern(t, ms, 0).Kind; k != ast.MatchWildcard {
		t.Errorf("kind = %v, want MatchWildcard", k)
	}
}

func TestMatchEmpty(t *testing.T) {
	ms := matchStmt(t, "match f() do end")
	if ms.Subject.String() != "f()" {
		t.Errorf("subject = %q, want f()", ms.Subject.String())
	}
	if len(ms.Arms) != 0 {
		t.Errorf("expected no arms for empty match, got %d", len(ms.Arms))
	}
}

func TestMatchNests(t *testing.T) {
	ms := matchStmt(t, `match x do
1 -> match y do
  10 -> print("a")
end
end`)
	inner, ok := ms.Arms[0].Body.(*ast.MatchStatement)
	if !ok {
		t.Fatalf("outer arm body = %T, want *ast.MatchStatement", ms.Arms[0].Body)
	}
	if inner.Subject.String() != "y" {
		t.Errorf("inner subject = %q, want y", inner.Subject.String())
	}
}

func TestMatchStringPattern(t *testing.T) {
	ms := matchStmt(t, `match s do "hi" -> print(1) end`)
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchValue || len(p.Values) != 1 || p.Values[0].String() != `"hi"` {
		t.Errorf("pattern = %s, want value pattern \"hi\"", p.String())
	}
}

func TestMatchWildcardNotLastErrors(t *testing.T) {
	msg := parseError(t, `match x do
_ -> print("any")
1 -> print("one")
end`)
	if !contains(msg, "wildcard") {
		t.Errorf("error = %q, want it to mention `wildcard`", msg)
	}
}

func TestMatchSemicolonBetweenArms(t *testing.T) {
	ms := matchStmt(t, `match s do
"hi" -> print("greeting");
"bye" -> print("farewell")
end`)
	if len(ms.Arms) != 2 {
		t.Errorf("arms = %d, want 2", len(ms.Arms))
	}
}

func TestMatchMissingDo(t *testing.T) {
	msg := parseError(t, `match x 1 -> print("one") end`)
	if msg == "" {
		t.Errorf("expected error for missing `do`")
	}
}

func TestBreakInsideLoopParses(t *testing.T) {
	cases := []string{
		"while true do break end",
		"repeat break until true",
		"for i = 1, 10 do break end",
		"for k, v in pairs(t) do break end",
		"while true do if x then break end end",
		"for i = 1, 3 do for j = 1, 3 do break end end",
	}
	for _, src := range cases {
		p := New(lexer.New(src))
		_, err := p.ParseProgram()
		if err != nil {
			t.Errorf("expected %q to parse, got error: %s", src, err.Message)
		}
	}
}

func TestBreakOutsideLoopErrors(t *testing.T) {
	cases := []string{
		"break",
		"do break end",
		"if x then break end",
		"for i = 1, 10 do local f = function() break end end",
	}
	for _, src := range cases {
		msg := parseError(t, src)
		if !contains(msg, "break") || !contains(msg, "outside a loop") {
			t.Errorf("expected break-outside-loop error for %q, got: %s", src, msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseStructBasic(t *testing.T) {
	stmt := parseExpect1(t, `struct Point {
		x: number,
		y: number,
	}`)
	ss, ok := stmt.(*ast.StructStatement)
	if !ok {
		t.Fatalf("expected *ast.StructStatement, got %T", stmt)
	}
	if ss.Name.Name != "Point" {
		t.Errorf("name = %q, want Point", ss.Name.Name)
	}
	if len(ss.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(ss.Fields))
	}
	if ss.Fields[0].Name != "x" || ss.Fields[1].Name != "y" {
		t.Errorf("field names = %q,%q, want x,y", ss.Fields[0].Name, ss.Fields[1].Name)
	}
	if ss.Fields[0].Type.String() != "number" {
		t.Errorf("field x type = %q, want number", ss.Fields[0].Type.String())
	}
}

func TestParseStructSemicolonSeparators(t *testing.T) {
	stmt := parseExpect1(t, `struct P { a: number; b: string }`)
	ss := stmt.(*ast.StructStatement)
	if len(ss.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(ss.Fields))
	}
}

func TestParseStructIsSoftKeyword(t *testing.T) {
	prog := parse(t, `local struct = 5 print(struct)`)
	if len(prog.Block.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Block.Statements))
	}
}

func TestParseStructErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`struct P { }`, "no fields"},
		{`struct P { x number }`, "expected ':'"},
		{`struct P { x: number, x: string }`, "duplicate field"},
		{`struct P { x: number `, "missing '}'"},
	}
	for _, tc := range cases {
		msg := parseError(t, tc.src)
		if !contains(msg, tc.want) {
			t.Errorf("for %q: got %q, want substring %q", tc.src, msg, tc.want)
		}
	}
}

func TestParseTaggedEnum(t *testing.T) {
	stmt := parseExpect1(t, `enum Shape
		Circle(number),
		Rect(number, number),
		Unit,
	end`)
	es, ok := stmt.(*ast.EnumStatement)
	if !ok {
		t.Fatalf("expected *ast.EnumStatement, got %T", stmt)
	}
	if !es.IsTagged() {
		t.Fatalf("expected IsTagged() to be true")
	}
	if len(es.Variants) != 3 {
		t.Fatalf("variants = %d, want 3", len(es.Variants))
	}
	if len(es.Variants[0].Payload) != 1 {
		t.Errorf("Circle payload = %d, want 1", len(es.Variants[0].Payload))
	}
	if len(es.Variants[1].Payload) != 2 {
		t.Errorf("Rect payload = %d, want 2", len(es.Variants[1].Payload))
	}
	if len(es.Variants[2].Payload) != 0 {
		t.Errorf("Unit payload = %d, want 0", len(es.Variants[2].Payload))
	}
}

func TestParsePlainEnumIsNotTagged(t *testing.T) {
	stmt := parseExpect1(t, `enum Color RED, GREEN, BLUE end`)
	es := stmt.(*ast.EnumStatement)
	if es.IsTagged() {
		t.Errorf("plain enum reported as tagged")
	}
}

func TestParseTaggedEnumErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`enum E A() end`, "empty payload"},
		{`enum E A(number end`, "expected ')'"},
	}
	for _, tc := range cases {
		msg := parseError(t, tc.src)
		if !contains(msg, tc.want) {
			t.Errorf("for %q: got %q, want substring %q", tc.src, msg, tc.want)
		}
	}
}

func TestMatchTypedBinding(t *testing.T) {
	ms := matchStmt(t, `match v do
n: number -> print(n)
end`)
	if len(ms.Arms) != 1 {
		t.Fatalf("arms = %d, want 1", len(ms.Arms))
	}
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchTyped {
		t.Fatalf("kind = %v, want MatchTyped", p.Kind)
	}
	if p.Bind != "n" {
		t.Errorf("bind = %q, want n", p.Bind)
	}
	if _, ok := p.Type.(*ast.TypePrimitive); !ok {
		t.Errorf("type = %T, want *ast.TypePrimitive", p.Type)
	}
	if p.Type.String() != "number" {
		t.Errorf("type = %q, want number", p.Type.String())
	}
}

func TestMatchNominalTypedBindingKeepsTypeName(t *testing.T) {
	ms := matchStmt(t, `match v do
p: Point -> print(p)
end`)
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchTyped {
		t.Fatalf("kind = %v, want MatchTyped", p.Kind)
	}
	if _, ok := p.Type.(*ast.TypeName); !ok {
		t.Errorf("type = %T, want *ast.TypeName", p.Type)
	}
	if p.Type.String() != "Point" {
		t.Errorf("type = %q, want Point", p.Type.String())
	}
}

func TestMatchPositionalDestructure(t *testing.T) {
	ms := matchStmt(t, `enum Shape Circle(number), Rect(number, number) end
match s do
Shape.Circle(r) -> print(r)
end`)
	p := armPattern(t, ms, 0)
	if p.Kind != ast.MatchDestructurePos {
		t.Fatalf("kind = %v, want MatchDestructurePos", p.Kind)
	}
	if p.Tag != "Circle" {
		t.Errorf("tag = %q, want Circle", p.Tag)
	}
	if len(p.PosBinds) != 1 || p.PosBinds[0] != "r" {
		t.Errorf("posBinds = %v, want [r]", p.PosBinds)
	}
}

func TestMatchNamedDestructure(t *testing.T) {
	ms := matchStmt(t, `struct Point { x: number, y: number }
match p do
Point{ x = px, y = py } -> print(px, py)
end`)
	pat := armPattern(t, ms, 0)
	if pat.Kind != ast.MatchDestructureNamed {
		t.Fatalf("kind = %v, want MatchDestructureNamed", pat.Kind)
	}
	if pat.Tag != "Point" {
		t.Errorf("tag = %q, want Point", pat.Tag)
	}
	want := []ast.MatchFieldBind{{Field: "x", Bind: "px"}, {Field: "y", Bind: "py"}}
	if len(pat.NamedBinds) != len(want) {
		t.Fatalf("namedBinds = %v, want %v", pat.NamedBinds, want)
	}
	for i, w := range want {
		if pat.NamedBinds[i] != w {
			t.Errorf("namedBind %d = %+v, want %+v", i, pat.NamedBinds[i], w)
		}
	}
}

func TestMatchGuardIsOnTheArm(t *testing.T) {
	ms := matchStmt(t, `match n do
x: number if x > 0 -> print("pos")
end`)
	arm := &ms.Arms[0]
	if arm.Guard == nil {
		t.Fatalf("arm has no guard")
	}
	if got := arm.Guard.String(); got != "(x > 0)" {
		t.Errorf("guard = %q, want (x > 0)", got)
	}
}

func TestMatchUnderscoreIsNotABinder(t *testing.T) {
	ms := matchStmt(t, `enum Shape Circle(number), Rect(number, number) end
match s do
Shape.Rect(_, h) -> print(h)
end`)
	p := armPattern(t, ms, 0)
	if len(p.PosBinds) != 2 || p.PosBinds[0] != "_" || p.PosBinds[1] != "h" {
		t.Fatalf("posBinds = %v, want [_ h]", p.PosBinds)
	}
	if got := p.Binders(); len(got) != 1 || got[0] != "h" {
		t.Errorf("binders = %v, want [h]", got)
	}
}

func TestMatchCommaWithBindingErrors(t *testing.T) {
	msg := parseError(t, `match v do
n: number, s: string -> print(n)
end`)
	if !contains(msg, "value patterns") {
		t.Errorf("error = %q, want it to mention value patterns", msg)
	}
}

func TestParseGenericFunction(t *testing.T) {
	lf := parseExpect1(t, "local function id<T>(x: T): T return x end").(*ast.LocalFunctionStatement)
	if len(lf.Func.TypeParams) != 1 || lf.Func.TypeParams[0] != "T" {
		t.Errorf("type params = %v, want [T]", lf.Func.TypeParams)
	}
}

func TestParseGenericFunctionMultiParam(t *testing.T) {
	fd := parseExpect1(t, "function f<K, V>(k: K, v: V) end").(*ast.FunctionDeclaration)
	if len(fd.Func.TypeParams) != 2 {
		t.Fatalf("type params = %d, want 2", len(fd.Func.TypeParams))
	}
}

func TestParseGenericTypeAlias(t *testing.T) {
	ta := parseExpect1(t, "type Box<T> = { value: T }").(*ast.TypeAliasStatement)
	if len(ta.TypeParams) != 1 || ta.TypeParams[0] != "T" {
		t.Errorf("type params = %v, want [T]", ta.TypeParams)
	}
}

func TestParseGenericApplicationInAnnotation(t *testing.T) {
	ls := parseExpect1(t, "local b: Box<number> = x").(*ast.LocalStatement)
	app, ok := ls.Names[0].Type.(*ast.TypeApplication)
	if !ok {
		t.Fatalf("annotation type = %T, want *ast.TypeApplication", ls.Names[0].Type)
	}
	if app.Name != "Box" || len(app.Args) != 1 {
		t.Errorf("application = %s, want Box<number>", app.String())
	}
}

func TestParseNestedGenericApplication(t *testing.T) {
	ls := parseExpect1(t, "local m: Box<Box<number>> = x").(*ast.LocalStatement)
	if got := ls.Names[0].Type.String(); got != "Box<Box<number>>" {
		t.Errorf("nested application = %q, want Box<Box<number>>", got)
	}
}

func TestParseGenericStruct(t *testing.T) {
	ss := parseExpect1(t, "struct Pair<A, B> { first: A, second: B }").(*ast.StructStatement)
	if len(ss.TypeParams) != 2 || ss.TypeParams[0] != "A" || ss.TypeParams[1] != "B" {
		t.Errorf("type params = %v, want [A B]", ss.TypeParams)
	}
}

func TestParseGenericErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"local function f<>(x) end", "type-parameter name"},
		{"local function f<T, T>(x) end", "duplicate type parameter"},
		{"local b: Box<number = x", "'>'"},
	}
	for _, tc := range cases {
		msg := parseError(t, tc.src)
		if !contains(msg, tc.want) {
			t.Errorf("for %q: got %q, want substring %q", tc.src, msg, tc.want)
		}
	}
}
