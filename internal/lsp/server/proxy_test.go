package server

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

func TestComputeDiagnostics_SyntaxError(t *testing.T) {
	src := "local function f(\n"
	diags := computeDiagnostics(src)
	if len(diags) == 0 {
		t.Fatal("expected a syntax diagnostic, got none")
	}
	d := diags[0]
	if d.Severity != protocol.DiagnosticSeverityError {
		t.Errorf("severity = %v, want Error", d.Severity)
	}
	if d.Source != "luascript" || d.Code != "syntax" {
		t.Errorf("source/code = %q/%v, want luascript/syntax", d.Source, d.Code)
	}
}

func TestComputeDiagnostics_TypeError(t *testing.T) {
	src := "local x: number = \"hello\"\n"
	diags := computeDiagnostics(src)
	found := false
	for _, d := range diags {
		if d.Source == "luascript-types" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a type diagnostic, got %+v", diags)
	}
}

func TestComputeDiagnostics_Clean(t *testing.T) {
	src := "local x = 1\nprint(x)\n"
	for _, d := range computeDiagnostics(src) {
		if d.Severity == protocol.DiagnosticSeverityError {
			t.Errorf("unexpected error diagnostic on clean source: %q", d.Message)
		}
	}
}

func TestComputeDiagnostics_NoCheckDirective(t *testing.T) {
	src := "--!nocheck\nlocal x: number = \"hello\"\n"
	for _, d := range computeDiagnostics(src) {
		if d.Source == "luascript-types" {
			t.Errorf("--!nocheck should suppress type diagnostics, got %q", d.Message)
		}
	}
}

func TestDocumentSymbols(t *testing.T) {
	src := "local a = 1\nlocal function f() end\nfunction M.g() end\n"
	syms := documentSymbols("file:///t.lsc", src)
	got := map[string]bool{}
	for _, s := range syms {
		if s.SymbolInformation != nil {
			got[s.SymbolInformation.Name] = true
		}
	}
	for _, want := range []string{"a", "f", "M.g"} {
		if !got[want] {
			t.Errorf("missing symbol %q (got %v)", want, got)
		}
	}
}

func TestWordAt(t *testing.T) {
	src := "print(x)"
	w, start, end := wordAt(src, 2)
	if w != "print" || start != 0 || end != 5 {
		t.Errorf("wordAt = %q [%d,%d], want print [0,5]", w, start, end)
	}
	if w, _, _ := wordAt("123", 1); w != "" {
		t.Errorf("wordAt on number = %q, want empty", w)
	}
}

func TestCompletionItems(t *testing.T) {
	items := completionItems()
	labels := map[string]bool{}
	for _, it := range items {
		labels[it.Label] = true
	}
	for _, want := range []string{"function", "local", "print", "require", "math", "json"} {
		if !labels[want] {
			t.Errorf("completion missing %q", want)
		}
	}
}

func TestExtractLineCol(t *testing.T) {
	line, col := extractLineCol("function: expected foo got bar at line 3, column 7.")
	if line != 3 || col != 7 {
		t.Errorf("extractLineCol = (%d,%d), want (3,7)", line, col)
	}
	line, col = extractLineCol("some error at line 5.")
	if line != 5 || col != 0 {
		t.Errorf("extractLineCol = (%d,%d), want (5,0)", line, col)
	}
}

func TestNamespaceBefore(t *testing.T) {
	cases := []struct {
		src  string
		at   int
		want string
	}{
		{"math.", 5, "math"},
		{"math.fl", 5, "math"},
		{"str:", 4, "str"},
		{"print", 0, ""},
		{"a.b.c", 4, ""},
		{"1.5", 2, ""},
	}
	for _, c := range cases {
		if got := namespaceBefore(c.src, c.at); got != c.want {
			t.Errorf("namespaceBefore(%q, %d) = %q, want %q", c.src, c.at, got, c.want)
		}
	}
}

func TestMemberCompletion(t *testing.T) {
	items := memberCompletionItems("math")
	labels := map[string]bool{}
	for _, it := range items {
		labels[it.Label] = true
	}
	for _, want := range []string{"floor", "ceil", "sqrt", "random", "pi"} {
		if !labels[want] {
			t.Errorf("math member completion missing %q", want)
		}
	}
	if memberCompletionItems("print") != nil {
		t.Error("expected nil member completion for non-namespace 'print'")
	}
}

func TestQualifiedHoverDocs(t *testing.T) {
	if !strings.Contains(hoverDocs["math.floor"], "largest integer") {
		t.Errorf("qualified hover for math.floor missing, got %q", hoverDocs["math.floor"])
	}
	if !strings.Contains(hoverDocs["string.format"], "printf") {
		t.Errorf("qualified hover for string.format missing, got %q", hoverDocs["string.format"])
	}
}

func TestHoverDocsCoverGlobals(t *testing.T) {
	if !strings.Contains(hoverDocs["print"], "stdout") {
		t.Errorf("hover doc for print missing, got %q", hoverDocs["print"])
	}
	if hoverDocs["function"] == "" {
		t.Error("hover doc for keyword 'function' missing")
	}
}
