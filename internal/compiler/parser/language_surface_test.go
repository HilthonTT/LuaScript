package parser

import (
	"testing"

	"github.com/hilthontt/luascript/v2/internal/compiler/ast"
)

func TestParseNamedDestructure(t *testing.T) {
	ds := parseExpect1(t, "local { host, port } = cfg").(*ast.LocalDestructureStatement)
	if ds.IsArray {
		t.Fatalf("IsArray = true, want false")
	}
	if len(ds.Binds) != 2 {
		t.Fatalf("binds = %d, want 2", len(ds.Binds))
	}
	if ds.Binds[0].Key != "host" || ds.Binds[0].Bind != "host" {
		t.Errorf("bind 0 = %+v, want key/bind host", ds.Binds[0])
	}
}

func TestParseDestructureRenameDefaultAndRest(t *testing.T) {
	ds := parseExpect1(t, "local { host = h, timeout or 30, ...others } = cfg").(*ast.LocalDestructureStatement)
	if len(ds.Binds) != 3 {
		t.Fatalf("binds = %d, want 3", len(ds.Binds))
	}
	if ds.Binds[0].Key != "host" || ds.Binds[0].Bind != "h" {
		t.Errorf("rename = %+v, want host -> h", ds.Binds[0])
	}
	if ds.Binds[1].Default == nil {
		t.Errorf("bind 1 has no default")
	}
	if !ds.Binds[2].Rest || ds.Binds[2].Bind != "others" {
		t.Errorf("rest = %+v, want ...others", ds.Binds[2])
	}
}

func TestParsePositionalDestructure(t *testing.T) {
	ds := parseExpect1(t, "local [ a, b, ...tail ] = list").(*ast.LocalDestructureStatement)
	if !ds.IsArray {
		t.Fatalf("IsArray = false, want true")
	}
	if ds.Binds[0].Index != 1 || ds.Binds[1].Index != 2 {
		t.Errorf("indices = %d, %d, want 1, 2", ds.Binds[0].Index, ds.Binds[1].Index)
	}
	if !ds.Binds[2].Rest || ds.Binds[2].Index != 3 {
		t.Errorf("rest = %+v, want index 3", ds.Binds[2])
	}
}

func TestParseDestructureErrors(t *testing.T) {
	cases := []string{
		"local { ...rest, a } = t",
		"local { } = t",
		"local { a, a } = t",
		"local { a } t",
	}
	for _, src := range cases {
		parseError(t, src)
	}
}

func TestParseTableSpread(t *testing.T) {
	ls := parseExpect1(t, "local m = { ...a, x = 1, ...b }").(*ast.LocalStatement)
	tc := ls.Values[0].(*ast.TableConstructor)
	if !tc.Fields[0].IsSpread || tc.Fields[1].IsSpread || !tc.Fields[2].IsSpread {
		t.Errorf("spread flags = %v %v %v, want true false true",
			tc.Fields[0].IsSpread, tc.Fields[1].IsSpread, tc.Fields[2].IsSpread)
	}
}

func TestParseVarargInTableIsNotSpread(t *testing.T) {
	ls := parseExpect1(t, "local m = { ... }").(*ast.LocalStatement)
	tc := ls.Values[0].(*ast.TableConstructor)
	if tc.Fields[0].IsSpread {
		t.Errorf("`{ ... }` parsed as a spread, want a vararg expansion")
	}
}

func TestParseOptionalChain(t *testing.T) {
	ls := parseExpect1(t, "local v = a?.b.c").(*ast.LocalStatement)
	outer := ls.Values[0].(*ast.IndexExpression)
	if outer.Optional {
		t.Errorf("outer link marked optional, want plain")
	}
	inner := outer.Object.(*ast.IndexExpression)
	if !inner.Optional {
		t.Errorf("inner link not marked optional")
	}
}

func TestParseOptionalBracketAndMethod(t *testing.T) {
	ls := parseExpect1(t, "local v = a?[k]").(*ast.LocalStatement)
	if ix := ls.Values[0].(*ast.IndexExpression); !ix.Optional || ix.IsDot {
		t.Errorf("bracket form = %+v, want optional non-dot", ix)
	}
	ls = parseExpect1(t, "local v = a?:m(1)").(*ast.LocalStatement)
	if mc := ls.Values[0].(*ast.MethodCallExpression); !mc.Optional || mc.Method != "m" {
		t.Errorf("method form = %+v, want optional call to m", mc)
	}
}

func TestParseCoalesce(t *testing.T) {
	ls := parseExpect1(t, "local v = a ?? b").(*ast.LocalStatement)
	if be, ok := ls.Values[0].(*ast.BinaryExpression); !ok || be.Op != "??" {
		t.Errorf("value = %s, want a ?? b", ls.Values[0].String())
	}
}

func TestParseCoalesceBindsTighterThanOr(t *testing.T) {
	ls := parseExpect1(t, "local v = a ?? b or c").(*ast.LocalStatement)
	if got := ls.Values[0].String(); got != "((a ?? b) or c)" {
		t.Errorf("value = %s, want ((a ?? b) or c)", got)
	}
}

func TestParsePipeline(t *testing.T) {
	cases := map[string]string{
		"local v = x |> f":       "f(x)",
		"local v = x |> f(1)":    "f(x, 1)",
		"local v = x |> o:m(1)":  "o:m(x, 1)",
		"local v = x |> f |> g":  "g(f(x))",
		"local v = a + b |> f":   "f((a + b))",
		"local v = x |> f or y":  "(f(x) or y)",
		"local v = x |> a.b.c()": "a.b.c(x)",
	}
	for src, want := range cases {
		ls := parseExpect1(t, src).(*ast.LocalStatement)
		if got := ls.Values[0].String(); got != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestParsePipelineRejectsNonCallable(t *testing.T) {
	parseError(t, "local v = x |> 42")
}

func TestParseImplStatement(t *testing.T) {
	is := parseExpect1(t, "impl Point function m(self) end function n() end end").(*ast.ImplStatement)
	if is.Target.Name != "Point" {
		t.Errorf("target = %q, want Point", is.Target.Name)
	}
	if len(is.Members) != 2 || is.Members[0].Name != "m" || is.Members[1].Name != "n" {
		t.Errorf("members = %+v, want m and n", is.Members)
	}
}

func TestParseImplErrors(t *testing.T) {
	cases := []string{
		"impl Point end",
		"impl Point local x = 1 end",
		"impl Point function m() end function m() end end",
		"impl Point function m() end",
	}
	for _, src := range cases {
		parseError(t, src)
	}
}

func TestImplIsContextual(t *testing.T) {
	parse(t, "local impl = 1 impl = impl + 1")
}

func TestParseInterfaceStatement(t *testing.T) {
	ta := parseExpect1(t, "interface Named { name: string }").(*ast.TypeAliasStatement)
	if !ta.IsInterface || ta.Name != "Named" {
		t.Errorf("alias = %+v, want interface Named", ta)
	}
	if _, ok := ta.Target.(*ast.TypeTable); !ok {
		t.Errorf("target = %T, want *ast.TypeTable", ta.Target)
	}
}

func TestInterfaceIsContextual(t *testing.T) {
	parse(t, "local interface = 1 print(interface)")
}

func TestParseTypeParamConstraint(t *testing.T) {
	lf := parseExpect1(t, "local function f<T: Named, U>(x: T): T return x end").(*ast.LocalFunctionStatement)
	if len(lf.Func.TypeParams) != 2 {
		t.Fatalf("type params = %d, want 2", len(lf.Func.TypeParams))
	}
	if lf.Func.TypeParams[0].Constraint == nil || lf.Func.TypeParams[0].Constraint.String() != "Named" {
		t.Errorf("constraint = %v, want Named", lf.Func.TypeParams[0].Constraint)
	}
	if lf.Func.TypeParams[1].Constraint != nil {
		t.Errorf("second param is constrained, want unconstrained")
	}
}

func TestParseIntersectionType(t *testing.T) {
	ta := parseExpect1(t, "type Both = A & B").(*ast.TypeAliasStatement)
	if _, ok := ta.Target.(*ast.TypeIntersection); !ok {
		t.Fatalf("target = %T, want *ast.TypeIntersection", ta.Target)
	}
	if got := ta.Target.String(); got != "A & B" {
		t.Errorf("target = %q, want A & B", got)
	}
}

func TestIntersectionBindsTighterThanUnion(t *testing.T) {
	ta := parseExpect1(t, "type T = A & B | C").(*ast.TypeAliasStatement)
	if got := ta.Target.String(); got != "A & B | C" {
		t.Errorf("target = %q, want A & B | C", got)
	}
	if _, ok := ta.Target.(*ast.TypeUnion); !ok {
		t.Errorf("target = %T, want the union to be outermost", ta.Target)
	}
}
