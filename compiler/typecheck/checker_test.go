package typecheck

import (
	"strings"
	"testing"

	"github.com/hilthontt/sakura-lang/compiler/lexer"
	"github.com/hilthontt/sakura-lang/compiler/parser"
)

// runCheck parses src and runs the checker. Returns the error list.
func runCheck(t *testing.T, src string) []TypeError {
	t.Helper()
	return runCheckWith(t, src, Options{})
}

func runCheckWith(t *testing.T, src string, opts Options) []TypeError {
	t.Helper()
	p := parser.New(lexer.New(src))
	prog, perr := p.ParseProgram()
	if perr != nil {
		t.Fatalf("parse error: %s\nsource: %s", perr.Message, src)
	}
	return Check(prog, opts)
}

// expectOK fails if the checker emits any error.
func expectOK(t *testing.T, src string) {
	t.Helper()
	errs := runCheck(t, src)
	if len(errs) > 0 {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Format())
		}
		t.Errorf("expected no type errors, got:\n  %s\nsource: %s", strings.Join(msgs, "\n  "), src)
	}
}

// expectErrContains fails unless at least one error message contains substr.
func expectErrContains(t *testing.T, src, substr string) {
	t.Helper()
	errs := runCheck(t, src)
	for _, e := range errs {
		if strings.Contains(e.Format(), substr) {
			return
		}
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Format())
	}
	t.Errorf("expected error containing %q, got:\n  %s\nsource: %s",
		substr, strings.Join(msgs, "\n  "), src)
}

// ---------------------------------------------------------------------------
// Primitives + locals
// ---------------------------------------------------------------------------

func TestPrimitiveAnnotationOK(t *testing.T) {
	expectOK(t, `local x: number = 1`)
	expectOK(t, `local s: string = "hi"`)
	expectOK(t, `local b: boolean = true`)
	expectOK(t, `local n: nil = nil`)
}

func TestPrimitiveAnnotationMismatch(t *testing.T) {
	expectErrContains(t, `local x: number = "hi"`, `"string"`)
	expectErrContains(t, `local x: string = 1`, `"number"`)
	expectErrContains(t, `local x: boolean = 1`, `"number"`)
}

func TestLocalInferenceFromLiteral(t *testing.T) {
	// `local x = 1` infers number; assigning a string later should error.
	expectErrContains(t, `local x = 1
x = "hi"`, `"string"`)
}

func TestAnyAcceptsEverything(t *testing.T) {
	expectOK(t, `local x: any = 1`)
	expectOK(t, `local x: any = "hi"`)
	expectOK(t, `local x: any = nil`)
}

func TestUnannotatedLocalIsLatent(t *testing.T) {
	// `local f = something_undefined` — the global lookup falls back to any,
	// which then propagates. Type-correctness of the reference itself is
	// not checked at the local-binding level.
	expectOK(t, `local f = some_global`)
}

// ---------------------------------------------------------------------------
// Optionals + unions
// ---------------------------------------------------------------------------

func TestOptionalAcceptsNil(t *testing.T) {
	expectOK(t, `local x: number? = nil`)
	expectOK(t, `local x: number? = 1`)
}

func TestOptionalRejectsWrongType(t *testing.T) {
	expectErrContains(t, `local x: number? = "hi"`, `"string"`)
}

func TestUnionAcceptsAnyMember(t *testing.T) {
	expectOK(t, `local id: number | string = 1`)
	expectOK(t, `local id: number | string = "hello"`)
}

func TestUnionRejectsNonMember(t *testing.T) {
	expectErrContains(t, `local id: number | string = true`, `"boolean"`)
}

// ---------------------------------------------------------------------------
// Type aliases
// ---------------------------------------------------------------------------

func TestTypeAliasResolves(t *testing.T) {
	expectOK(t, `type Id = number
local x: Id = 1`)
}

func TestTypeAliasMismatch(t *testing.T) {
	expectErrContains(t, `type Id = number
local x: Id = "hi"`, `"string"`)
}

func TestUnknownAliasErrors(t *testing.T) {
	expectErrContains(t, `local x: NotDefined = 1`, `unknown type "NotDefined"`)
}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

func TestFunctionParamTypeChecksArgs(t *testing.T) {
	src := `
function add(a: number, b: number): number return a + b end
local r = add(1, "two")
`
	expectErrContains(t, src, `"string"`)
}

func TestFunctionArityChecked(t *testing.T) {
	src := `
function f(a: number, b: number) end
f(1)
`
	expectErrContains(t, src, `expects at least 2`)
}

func TestFunctionAcceptsOptionalTrailingArg(t *testing.T) {
	src := `
function greet(name: string, suffix: string?) end
greet("Ada")
greet("Ada", "!")
`
	expectOK(t, src)
}

func TestFunctionVarargAcceptsExtras(t *testing.T) {
	src := `
function fmt(prefix: string, ...: number) end
fmt("x")
fmt("x", 1)
fmt("x", 1, 2, 3)
`
	expectOK(t, src)
}

func TestFunctionReturnTypeChecked(t *testing.T) {
	src := `
function name(): string
    return 1
end
`
	expectErrContains(t, src, `"number"`)
}

func TestFunctionReturnSpreadingFromCall(t *testing.T) {
	// Multi-return spreading: an unannotated function's return is `any`,
	// which fills both targets in `g, i = make()`.
	src := `
function make()
    return 1, 2
end
local a, b = make()
b()
`
	// `b` is `any`, calling it is allowed.
	expectOK(t, src)
}

// ---------------------------------------------------------------------------
// Type assertions
// ---------------------------------------------------------------------------

func TestTypeAssertionOverride(t *testing.T) {
	src := `local n: number = something :: number`
	expectOK(t, src)
}

func TestTypeAssertionFlowsAsAsserted(t *testing.T) {
	// After `:: number`, the value is treated as number for downstream
	// flow. Here we feed it into a number-typed function param.
	src := `
function takes(x: number) end
takes(payload :: number)
`
	expectOK(t, src)
}

// ---------------------------------------------------------------------------
// Operators
// ---------------------------------------------------------------------------

func TestArithmeticRequiresNumbers(t *testing.T) {
	expectErrContains(t, `local n: number = "x" + 1`, `"string"`)
}

func TestConcatRequiresStringOrNumber(t *testing.T) {
	src := `
local s: string = true .. "x"
`
	expectErrContains(t, src, `..`)
}

func TestCompareRequiresMatchingOrderable(t *testing.T) {
	expectErrContains(t, `local b = "x" < 1`, "compare")
}

func TestLengthOpAcceptsStringOrTable(t *testing.T) {
	expectOK(t, `local n = #"hi"`)
	expectOK(t, `local n = #{1,2,3}`)
}

// ---------------------------------------------------------------------------
// Stdlib integration
// ---------------------------------------------------------------------------

func TestStdlibPrintAcceptsAnyArgs(t *testing.T) {
	expectOK(t, `print(1, "x", true, nil)`)
}

func TestStdlibTostringReturnsString(t *testing.T) {
	src := `local s: string = tostring(42)`
	expectOK(t, src)
}

func TestStdlibStringFormatTyped(t *testing.T) {
	expectOK(t, `local s: string = string.format("%d", 7)`)
}

func TestStdlibMathPiIsNumber(t *testing.T) {
	expectOK(t, `local pi: number = math.pi`)
}

func TestStdlibUnknownFieldErrors(t *testing.T) {
	expectErrContains(t, `local x = math.notReal`, `no field`)
}

// ---------------------------------------------------------------------------
// Mode directives
// ---------------------------------------------------------------------------

func TestNocheckBypassesAllChecks(t *testing.T) {
	// Compiler glue handles the actual bypass — but the directive must be
	// captured so the glue can read it.
	p := parser.New(lexer.New("--!nocheck\nlocal x: number = \"hi\""))
	prog, perr := p.ParseProgram()
	if perr != nil {
		t.Fatalf("parse error: %s", perr.Message)
	}
	// We invoke Check directly here for unit isolation; the compiler
	// glue is what skips the call entirely under --!nocheck.
	errs := Check(prog, Options{})
	if len(errs) == 0 {
		t.Errorf("expected errors when bypass not honored — directive itself doesn't suppress within Check()")
	}
	if got := p.Lexer.ModeDirective; got != "nocheck" {
		t.Errorf("ModeDirective = %q, want nocheck", got)
	}
}

func TestStrictModeFlagsImplicitAny(t *testing.T) {
	src := `function f(a, b) return a end`
	errs := runCheckWith(t, src, Options{Strict: true})
	if len(errs) == 0 {
		t.Errorf("expected implicit-any errors in strict mode")
	}
}

func TestNonstrictModeAcceptsImplicitAny(t *testing.T) {
	src := `function f(a, b) return a end`
	errs := runCheckWith(t, src, Options{Strict: false})
	if len(errs) > 0 {
		t.Errorf("nonstrict mode should accept implicit any; got %d errors", len(errs))
	}
}

// ---------------------------------------------------------------------------
// Lua programs without annotations remain unaffected
// ---------------------------------------------------------------------------

func TestUntypedLuaProgramTypechecks(t *testing.T) {
	// Realistic Lua program with no annotations — checker must accept.
	src := `
local function fact(n)
    if n <= 1 then return 1 end
    return n * fact(n - 1)
end
local result = fact(5)
print(result)
`
	expectOK(t, src)
}

func TestClosureCapturingUpvalues(t *testing.T) {
	src := `
local function counter()
    local n = 0
    return function()
        n = n + 1
        return n
    end
end
local next = counter()
print(next())
`
	expectOK(t, src)
}
