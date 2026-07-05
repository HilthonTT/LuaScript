package proxy

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/lsp/protocol"
)

func TestComputeDiagnostics_SyntaxError(t *testing.T) {
	// Unterminated function parameter list — a parse error with a column.
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
	// Assigning a string to a number-typed local is a type error, and the
	// file parses cleanly so the type checker runs.
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
	// --!nocheck must suppress type errors, matching the compiler pipeline.
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
	w, start, end := wordAt(src, 2) // inside "print"
	if w != "print" || start != 0 || end != 5 {
		t.Errorf("wordAt = %q [%d,%d], want print [0,5]", w, start, end)
	}
	// A digit run is not an identifier.
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

func TestHoverDocsCoverGlobals(t *testing.T) {
	if !strings.Contains(hoverDocs["print"], "stdout") {
		t.Errorf("hover doc for print missing, got %q", hoverDocs["print"])
	}
	if hoverDocs["function"] == "" {
		t.Error("hover doc for keyword 'function' missing")
	}
}
