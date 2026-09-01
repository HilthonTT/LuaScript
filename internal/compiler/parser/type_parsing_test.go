package parser

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
)

func newWithSource(src string) *Parser {
	return New(lexer.New(src))
}

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
		wantKind string
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
	src := "function f(a, b) end"
	fd := parseExpect1(t, src).(*ast.FunctionDeclaration)
	for i, p := range fd.Func.Params {
		if p.Type != nil {
			t.Errorf("param[%d] type should be nil; got %v", i, p.Type)
		}
	}
}

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
	src := "local type = 1"
	ls := parseExpect1(t, src).(*ast.LocalStatement)
	if ls.Names[0].Name != "type" {
		t.Errorf("expected local name 'type'")
	}
}

func TestTypeFunctionCallStillWorks(t *testing.T) {
	src := "type(x)"
	stmt := parseExpect1(t, src).(*ast.ExpressionStatement)
	if _, ok := stmt.Expression.(*ast.CallExpression); !ok {
		t.Errorf("expected CallExpression, got %T", stmt.Expression)
	}
}

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
	src := "x = 0; goto done; ::done::"
	prog := parse(t, src)
	if len(prog.Block.Statements) != 3 {
		t.Fatalf("statements = %d, want 3", len(prog.Block.Statements))
	}
	if _, ok := prog.Block.Statements[2].(*ast.LabelStatement); !ok {
		t.Errorf("third statement should be LabelStatement, got %T", prog.Block.Statements[2])
	}
}

func TestTypeAnnotationRoundTrip(t *testing.T) {
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

func TestLexerModeDirectiveStrict(t *testing.T) {
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
	src := "local x = 1\n--!strict\nlocal y = 2"
	p := newWithSource(src)
	if _, err := p.ParseProgram(); err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
	if got := p.Lexer.ModeDirective; got != "" {
		t.Errorf("ModeDirective = %q, want empty (mid-file directive ignored)", got)
	}
}

func TestLocalAnnotationLiteral(t *testing.T) {
	cases := []struct {
		src     string
		wantStr string
		check   func(*testing.T, *ast.TypeLiteral)
	}{
		{`local x: "read" = "read"`, `"read"`, func(t *testing.T, l *ast.TypeLiteral) {
			if l.Kind != ast.LiteralString || l.Str != "read" {
				t.Errorf("got kind=%v str=%q", l.Kind, l.Str)
			}
		}},
		{"local x: 42 = 42", "42", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Kind != ast.LiteralNumber || l.Num != 42 {
				t.Errorf("got kind=%v num=%v", l.Kind, l.Num)
			}
		}},
		{"local x: -1 = -1", "-1", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Kind != ast.LiteralNumber || l.Num != -1 {
				t.Errorf("got kind=%v num=%v", l.Kind, l.Num)
			}
		}},
		{"local x: 0x10 = 16", "0x10", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Num != 16 {
				t.Errorf("hex literal type: got num=%v, want 16", l.Num)
			}
		}},
		{"local x: 1.5 = 1.5", "1.5", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Num != 1.5 {
				t.Errorf("got num=%v", l.Num)
			}
		}},
		{"local x: true = true", "true", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Kind != ast.LiteralBoolean || !l.Bool {
				t.Errorf("got kind=%v bool=%v", l.Kind, l.Bool)
			}
		}},
		{"local x: false = false", "false", func(t *testing.T, l *ast.TypeLiteral) {
			if l.Kind != ast.LiteralBoolean || l.Bool {
				t.Errorf("got kind=%v bool=%v", l.Kind, l.Bool)
			}
		}},
	}
	for _, c := range cases {
		ty := firstLocalType(t, c.src)
		lit, ok := ty.(*ast.TypeLiteral)
		if !ok {
			t.Fatalf("%q: expected *ast.TypeLiteral, got %T", c.src, ty)
		}
		if lit.String() != c.wantStr {
			t.Errorf("%q: String() = %q, want %q", c.src, lit.String(), c.wantStr)
		}
		c.check(t, lit)
	}
}

func TestLiteralUnionType(t *testing.T) {
	ty := firstLocalType(t, `local x: "read" | "write" | 1 = "read"`)
	u, ok := ty.(*ast.TypeUnion)
	if !ok {
		t.Fatalf("expected *ast.TypeUnion, got %T", ty)
	}
	if len(u.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(u.Members))
	}
	if got, want := u.String(), `"read" | "write" | 1`; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestOptionalLiteralType(t *testing.T) {
	ty := firstLocalType(t, `local x: "read"? = nil`)
	if got, want := ty.String(), `"read"?`; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestLiteralTypeInAliasAndSignature(t *testing.T) {
	src := `type Mode = "read" | "write"
	local function f(m: Mode, n: 1): "ok" return "ok" end`
	p := newWithSource(src)
	if _, err := p.ParseProgram(); err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
}

func TestDanglingMinusInTypeIsAnError(t *testing.T) {
	p := newWithSource(`local x: - = 1`)
	_, err := p.ParseProgram()
	if err == nil {
		t.Fatal("expected a parse error for `local x: - = 1`")
	}
	if !strings.Contains(err.Message, "number after '-'") {
		t.Errorf("unexpected message: %s", err.Message)
	}
}
