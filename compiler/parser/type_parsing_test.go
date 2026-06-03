package parser

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/compiler/ast"
	"github.com/hilthontt/luascript/compiler/lexer"
)

func newWithSource(src string) *Parser {
	return New(lexer.New(src))
}

// ---------------------------------------------------------------------------
// Type-syntax parsing — Phase 1 of the Luau-style type system.
//
// These tests cover the parser surface only: type annotations on locals,
// parameters, return types, type aliases, type assertions, and the type
// expression grammar (primitives, named, optional, union, function, table).
// They do not exercise the type checker (Phase 2+).
// ---------------------------------------------------------------------------

// firstLocalType plucks the parsed type of the only `local` in src.
func firstLocalType(t *testing.T, src string) ast.TypeNode {
	t.Helper()
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	if len(ls.Names) == 0 {
		t.Fatalf("no names in local: %q", src)
	}
	return ls.Names[0].Type
}

func TestLocalAnnotationPrimitive(t *testing.T) {
	cases := []struct {
		src      string
		wantKind string // expected reflect-y "kind name" via String()
		wantStr  string
	}{
		{"local x: number = 1", "primitive", "number"},
		{"local x: string = \"hi\"", "primitive", "string"},
		{"local x: boolean = true", "primitive", "boolean"},
		{"local x: nil = nil", "primitive", "nil"},
		{"local x: any = 1", "primitive", "any"},
		{"local x: unknown = 1", "primitive", "unknown"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			tn := firstLocalType(t, c.src)
			if tn == nil {
				t.Fatalf("annotation missing for %q", c.src)
			}
			if _, ok := tn.(*ast.TypePrimitive); !ok {
				t.Fatalf("%q: want TypePrimitive, got %T", c.src, tn)
			}
			if got := tn.String(); got != c.wantStr {
				t.Errorf("%q: String() = %q, want %q", c.src, got, c.wantStr)
			}
		})
	}
}

func TestLocalAnnotationNamedAlias(t *testing.T) {
	tn := firstLocalType(t, "local p: Point = nil")
	tname, ok := tn.(*ast.TypeName)
	if !ok {
		t.Fatalf("want TypeName, got %T", tn)
	}
	if tname.Name != "Point" {
		t.Errorf("name = %q, want Point", tname.Name)
	}
}

func TestLocalAnnotationOptional(t *testing.T) {
	tn := firstLocalType(t, "local x: string? = nil")
	opt, ok := tn.(*ast.TypeOptional)
	if !ok {
		t.Fatalf("want TypeOptional, got %T", tn)
	}
	if got := opt.String(); got != "string?" {
		t.Errorf("String() = %q, want %q", got, "string?")
	}
}

func TestLocalAnnotationUnion(t *testing.T) {
	tn := firstLocalType(t, "local id: number | string = 1")
	uni, ok := tn.(*ast.TypeUnion)
	if !ok {
		t.Fatalf("want TypeUnion, got %T", tn)
	}
	if len(uni.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(uni.Members))
	}
	if got := uni.String(); got != "number | string" {
		t.Errorf("String() = %q, want %q", got, "number | string")
	}
}

func TestLocalAnnotationUnionThreeWay(t *testing.T) {
	tn := firstLocalType(t, "local x: number | string | boolean = 1")
	uni, ok := tn.(*ast.TypeUnion)
	if !ok {
		t.Fatalf("want TypeUnion, got %T", tn)
	}
	if len(uni.Members) != 3 {
		t.Fatalf("members = %d, want 3", len(uni.Members))
	}
}

func TestLocalAnnotationOptionalUnionInteraction(t *testing.T) {
	// `T?` is right-associated as a postfix on the *atom*, not the whole
	// union, matching Luau. So `number | string?` ≡ `number | (string?)`.
	tn := firstLocalType(t, "local x: number | string? = nil")
	uni, ok := tn.(*ast.TypeUnion)
	if !ok {
		t.Fatalf("want TypeUnion, got %T", tn)
	}
	if _, ok := uni.Members[1].(*ast.TypeOptional); !ok {
		t.Errorf("second member should be TypeOptional, got %T", uni.Members[1])
	}
}

func TestLocalAnnotationFunctionType(t *testing.T) {
	tn := firstLocalType(t, "local f: (number, string) -> boolean = nil")
	fn, ok := tn.(*ast.TypeFunction)
	if !ok {
		t.Fatalf("want TypeFunction, got %T", tn)
	}
	if len(fn.Params) != 2 {
		t.Errorf("params = %d, want 2", len(fn.Params))
	}
	if len(fn.Returns) != 1 {
		t.Errorf("returns = %d, want 1", len(fn.Returns))
	}
	if got := fn.String(); got != "(number, string) -> boolean" {
		t.Errorf("String() = %q", got)
	}
}

func TestLocalAnnotationFunctionTypeMultiReturn(t *testing.T) {
	tn := firstLocalType(t, "local f: (number) -> (number, string) = nil")
	fn := tn.(*ast.TypeFunction)
	if len(fn.Returns) != 2 {
		t.Errorf("returns = %d, want 2", len(fn.Returns))
	}
	if got := fn.String(); !strings.Contains(got, "-> (number, string)") {
		t.Errorf("String() = %q, want multi-return form", got)
	}
}

func TestLocalAnnotationFunctionTypeNoReturns(t *testing.T) {
	tn := firstLocalType(t, "local f: () -> () = nil")
	fn := tn.(*ast.TypeFunction)
	if len(fn.Params) != 0 {
		t.Errorf("params = %d, want 0", len(fn.Params))
	}
	if len(fn.Returns) != 0 {
		t.Errorf("returns = %d, want 0", len(fn.Returns))
	}
}

func TestLocalAnnotationFunctionTypeNamedParams(t *testing.T) {
	tn := firstLocalType(t, "local f: (x: number, y: number) -> number = nil")
	fn := tn.(*ast.TypeFunction)
	if fn.ParamNames[0] != "x" || fn.ParamNames[1] != "y" {
		t.Errorf("param names = %v, want [x y]", fn.ParamNames)
	}
}

func TestLocalAnnotationFunctionTypeVararg(t *testing.T) {
	tn := firstLocalType(t, "local f: (string, ...: number) -> () = nil")
	fn := tn.(*ast.TypeFunction)
	if !fn.IsVararg {
		t.Errorf("expected vararg")
	}
	if fn.VarargType == nil || fn.VarargType.String() != "number" {
		t.Errorf("vararg type = %v, want number", fn.VarargType)
	}
}

func TestLocalAnnotationTableTypeRecord(t *testing.T) {
	tn := firstLocalType(t, "local p: { x: number, y: number } = nil")
	tt, ok := tn.(*ast.TypeTable)
	if !ok {
		t.Fatalf("want TypeTable, got %T", tn)
	}
	if len(tt.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(tt.Fields))
	}
	if tt.Fields[0].Key != "x" || tt.Fields[1].Key != "y" {
		t.Errorf("field keys = [%s %s]", tt.Fields[0].Key, tt.Fields[1].Key)
	}
}

func TestLocalAnnotationTableTypeIndexer(t *testing.T) {
	tn := firstLocalType(t, "local m: {[string]: number} = nil")
	tt := tn.(*ast.TypeTable)
	if tt.Indexer == nil {
		t.Fatalf("expected indexer")
	}
	if tt.Indexer.Key.String() != "string" || tt.Indexer.Value.String() != "number" {
		t.Errorf("indexer = [%s]: %s", tt.Indexer.Key, tt.Indexer.Value)
	}
}

func TestLocalAnnotationTableTypeArrayShorthand(t *testing.T) {
	// `{T}` desugars to `{[number]: T}`.
	tn := firstLocalType(t, "local xs: {string} = nil")
	tt := tn.(*ast.TypeTable)
	if tt.Indexer == nil {
		t.Fatalf("array shorthand should produce indexer")
	}
	if tt.Indexer.Key.String() != "number" || tt.Indexer.Value.String() != "string" {
		t.Errorf("array shorthand = [%s]: %s, want [number]: string",
			tt.Indexer.Key, tt.Indexer.Value)
	}
}

func TestLocalAnnotationTableTypeEmpty(t *testing.T) {
	tn := firstLocalType(t, "local p: {} = nil")
	tt := tn.(*ast.TypeTable)
	if len(tt.Fields) != 0 || tt.Indexer != nil {
		t.Errorf("empty table type should have no fields and no indexer")
	}
}

func TestLocalAnnotationTableTypeNested(t *testing.T) {
	tn := firstLocalType(t, "local p: { inner: {x: number} } = nil")
	outer := tn.(*ast.TypeTable)
	inner, ok := outer.Fields[0].Value.(*ast.TypeTable)
	if !ok {
		t.Fatalf("nested value should be TypeTable, got %T", outer.Fields[0].Value)
	}
	if inner.Fields[0].Key != "x" {
		t.Errorf("inner field = %q", inner.Fields[0].Key)
	}
}

// --- Params / return on declared functions ----------------------------------

func TestFunctionDeclarationTypedParamsAndReturn(t *testing.T) {
	src := "function add(a: number, b: number): number return a + b end"
	fd := parseExpect1(t, src).(*ast.FunctionDeclaration)
	if len(fd.Func.Params) != 2 {
		t.Fatalf("params = %d, want 2", len(fd.Func.Params))
	}
	for i, want := range []string{"number", "number"} {
		if fd.Func.Params[i].Type == nil || fd.Func.Params[i].Type.String() != want {
			t.Errorf("param[%d] type = %v, want %s", i, fd.Func.Params[i].Type, want)
		}
	}
	if len(fd.Func.ReturnTypes) != 1 || fd.Func.ReturnTypes[0].String() != "number" {
		t.Errorf("return types = %v, want [number]", fd.Func.ReturnTypes)
	}
}

func TestFunctionDeclarationMultiReturn(t *testing.T) {
	src := "function pair(): (number, string) return 1, \"x\" end"
	fd := parseExpect1(t, src).(*ast.FunctionDeclaration)
	if len(fd.Func.ReturnTypes) != 2 {
		t.Fatalf("return types = %d, want 2", len(fd.Func.ReturnTypes))
	}
}

func TestFunctionDeclarationTypedVararg(t *testing.T) {
	src := "function fmt(...: string) end"
	fd := parseExpect1(t, src).(*ast.FunctionDeclaration)
	if !fd.Func.IsVararg {
		t.Fatalf("expected vararg")
	}
	if fd.Func.VarargType == nil || fd.Func.VarargType.String() != "string" {
		t.Errorf("vararg type = %v, want string", fd.Func.VarargType)
	}
}

func TestLocalFunctionTypedSignature(t *testing.T) {
	src := "local function f(x: number): string return \"hi\" end"
	lf := parseExpect1(t, src).(*ast.LocalFunctionStatement)
	if lf.Func.Params[0].Type.String() != "number" {
		t.Errorf("param type = %v", lf.Func.Params[0].Type)
	}
	if lf.Func.ReturnTypes[0].String() != "string" {
		t.Errorf("return type = %v", lf.Func.ReturnTypes[0])
	}
}

func TestUnannotatedParamsHaveNilType(t *testing.T) {
	// Existing Lua code without annotations must still parse, with each
	// TypedParam.Type left nil.
	src := "function f(a, b) end"
	fd := parseExpect1(t, src).(*ast.FunctionDeclaration)
	for i, p := range fd.Func.Params {
		if p.Type != nil {
			t.Errorf("param[%d] type should be nil; got %v", i, p.Type)
		}
	}
}

// --- Type aliases -----------------------------------------------------------

func TestTypeAliasPrimitive(t *testing.T) {
	src := "type Id = number"
	s := parseExpect1(t, src).(*ast.TypeAliasStatement)
	if s.Name != "Id" || s.Target.String() != "number" {
		t.Errorf("alias = %s = %s", s.Name, s.Target)
	}
}

func TestTypeAliasTableShape(t *testing.T) {
	src := "type Point = { x: number, y: number }"
	s := parseExpect1(t, src).(*ast.TypeAliasStatement)
	tt := s.Target.(*ast.TypeTable)
	if len(tt.Fields) != 2 {
		t.Errorf("fields = %d, want 2", len(tt.Fields))
	}
}

func TestTypeAliasFunctionType(t *testing.T) {
	src := "type Cb = (number, string) -> boolean"
	s := parseExpect1(t, src).(*ast.TypeAliasStatement)
	if _, ok := s.Target.(*ast.TypeFunction); !ok {
		t.Errorf("alias target should be TypeFunction, got %T", s.Target)
	}
}

func TestTypeAliasUnion(t *testing.T) {
	src := "type Either = number | string"
	s := parseExpect1(t, src).(*ast.TypeAliasStatement)
	if _, ok := s.Target.(*ast.TypeUnion); !ok {
		t.Errorf("alias target should be TypeUnion, got %T", s.Target)
	}
}

func TestTypeAsIdentifierStillWorks(t *testing.T) {
	// `type` is intentionally not a reserved keyword. Existing code that
	// uses it as a regular identifier must still compile.
	src := "local type = 1"
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	if ls.Names[0].Name != "type" {
		t.Errorf("expected local name 'type'")
	}
}

func TestTypeFunctionCallStillWorks(t *testing.T) {
	// `type(x)` is the global `type()` builtin call. Must not be confused
	// with a type-alias statement (which requires `type Ident =`).
	src := "type(x)"
	stmt := parseExpect1(t, src).(*ast.ExpressionStatement)
	if _, ok := stmt.Expression.(*ast.CallExpression); !ok {
		t.Errorf("expected CallExpression, got %T", stmt.Expression)
	}
}

// --- Type assertions --------------------------------------------------------

func TestTypeAssertionBasic(t *testing.T) {
	src := "local n = x :: number"
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	ta, ok := ls.Values[0].(*ast.TypeAssertionExpression)
	if !ok {
		t.Fatalf("want TypeAssertionExpression, got %T", ls.Values[0])
	}
	if ta.Type.String() != "number" {
		t.Errorf("asserted type = %v", ta.Type)
	}
}

func TestTypeAssertionBindsTighterThanBinaryOp(t *testing.T) {
	// `a + b :: number` should parse as `a + (b :: number)`, matching Luau.
	src := "local n = a + b :: number"
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	bin, ok := ls.Values[0].(*ast.BinaryExpression)
	if !ok {
		t.Fatalf("want BinaryExpression at top, got %T", ls.Values[0])
	}
	if _, ok := bin.Right.(*ast.TypeAssertionExpression); !ok {
		t.Errorf("RHS should be TypeAssertionExpression, got %T", bin.Right)
	}
}

func TestLabelStatementStillWorks(t *testing.T) {
	// Regression: the `::done::` label must not be consumed as a type
	// assertion when it follows a complete expression statement.
	src := "x = 0; goto done; ::done::"
	prog := parse(t, src)
	if len(prog.Block.Statements) != 3 {
		t.Fatalf("statements = %d, want 3", len(prog.Block.Statements))
	}
	if _, ok := prog.Block.Statements[2].(*ast.LabelStatement); !ok {
		t.Errorf("third statement should be LabelStatement, got %T", prog.Block.Statements[2])
	}
}

// --- Round-trip via String() ------------------------------------------------

func TestTypeAnnotationRoundTrip(t *testing.T) {
	// Each case lists distinguishing substrings the rendered output must
	// contain. The assertion is "the annotation didn't disappear silently",
	// not "exact whitespace match".
	cases := []struct {
		src   string
		needs []string
	}{
		{"local x: number = 1\n", []string{"x: number"}},
		{"local p: {x: number, y: number} = nil\n", []string{"x: number", "y: number"}},
		{"function f(a: number, b: string): boolean return true end\n", []string{"a: number", "b: string", ": boolean"}},
		{"type Point = {x: number, y: number}\n", []string{"type Point", "x: number"}},
		{"type Id = number | string\n", []string{"type Id", "number | string"}},
		{"local n = x :: number\n", []string{":: number"}},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog := parse(t, c.src)
			out := prog.String()
			for _, want := range c.needs {
				if !strings.Contains(out, want) {
					t.Errorf("round-trip missing %q\nin:  %q\nout: %q", want, c.src, out)
				}
			}
		})
	}
}

// --- Lexer mode-directive surface ------------------------------------------

func TestLexerModeDirectiveStrict(t *testing.T) {
	// Compile-time integration: the lexer is consumed by the parser; the
	// parser doesn't read ModeDirective itself yet — this test asserts the
	// lexer captured the directive correctly.
	src := "--!strict\nlocal x = 1"
	p := newWithSource(src)
	_, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
	if got := p.Lexer.ModeDirective; got != "strict" {
		t.Errorf("ModeDirective = %q, want strict", got)
	}
}

func TestLexerModeDirectiveNocheck(t *testing.T) {
	src := "--!nocheck\nlocal x = 1"
	p := newWithSource(src)
	if _, err := p.ParseProgram(); err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
	if got := p.Lexer.ModeDirective; got != "nocheck" {
		t.Errorf("ModeDirective = %q, want nocheck", got)
	}
}

func TestLexerModeDirectiveOnlyAtHead(t *testing.T) {
	// Directives in mid-file comments are NOT recognised — only leading
	// comments before any non-comment token has been emitted.
	src := "local x = 1\n--!strict\nlocal y = 2"
	p := newWithSource(src)
	if _, err := p.ParseProgram(); err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
	if got := p.Lexer.ModeDirective; got != "" {
		t.Errorf("ModeDirective = %q, want empty (mid-file directive ignored)", got)
	}
}
