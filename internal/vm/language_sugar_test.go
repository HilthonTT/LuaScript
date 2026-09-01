package vm

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func compileErr(t *testing.T, src string) string {
	t.Helper()
	_, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err == nil {
		t.Fatalf("expected compile error; got success\nsource:\n%s", src)
	}
	return err.Error()
}

func TestContinueNumericFor(t *testing.T) {
	v := run(t, `
		r = ""
		for i = 1, 6 do
			if i % 2 == 0 then continue end
			r = r .. i
		end`)
	assertGlobalEqual(t, v, "r", "135")
}

func TestContinueWhile(t *testing.T) {
	v := run(t, `
		n = 0
		local i = 0
		while i < 10 do
			i = i + 1
			if i % 3 ~= 0 then continue end
			n = n + 1
		end`)
	assertGlobalEqual(t, v, "n", int64(3))
}

func TestContinueRepeat(t *testing.T) {
	v := run(t, `
		r = 0
		local c = 0
		repeat
			c = c + 1
			if c % 2 == 0 then continue end
			r = r + c
		until c >= 5`)
	assertGlobalEqual(t, v, "r", int64(9))
}

func TestContinueGenericFor(t *testing.T) {
	v := run(t, `
		r = ""
		for _, s in ipairs({"a", "b", "c", "d"}) do
			if s == "b" then continue end
			r = r .. s
		end`)
	assertGlobalEqual(t, v, "r", "acd")
}

func TestContinueTargetsInnermostLoop(t *testing.T) {
	v := run(t, `
		r = 0
		for i = 1, 3 do
			for j = 1, 3 do
				if j == 2 then continue end
				r = r + 1
			end
		end`)
	assertGlobalEqual(t, v, "r", int64(6))
}

func TestContinueClosesLoopUpvalues(t *testing.T) {
	v := run(t, `
		local fns = {}
		for i = 1, 4 do
			if i == 2 then continue end
			fns[#fns + 1] = function() return i end
		end
		r = fns[1]() * 100 + fns[2]() * 10 + fns[3]()`)
	assertGlobalEqual(t, v, "r", int64(134))
}

func TestContinueStillAnIdentifier(t *testing.T) {
	v := run(t, `
		local continue = 40
		r = continue + 2`)
	assertGlobalEqual(t, v, "r", int64(42))
}

func TestIfExpressionBasic(t *testing.T) {
	v := run(t, `
		local x = 7
		r = if x > 0 then "pos" elseif x < 0 then "neg" else "zero"`)
	assertGlobalEqual(t, v, "r", "pos")
}

func TestIfExpressionElseArm(t *testing.T) {
	v := run(t, `r = if 1 > 2 then "a" elseif 2 > 3 then "b" else "c"`)
	assertGlobalEqual(t, v, "r", "c")
}

func TestIfExpressionLazyBranches(t *testing.T) {
	v := run(t, `
		hits = ""
		local function mark(s, v) hits = hits .. s return v end
		r = if true then mark("t", 1) else mark("e", 2)`)
	assertGlobalEqual(t, v, "r", int64(1))
	assertGlobalEqual(t, v, "hits", "t")
}

func TestIfExpressionSingleValueAdjustment(t *testing.T) {
	v := run(t, `
		local function two() return 1, 2 end
		local a, b = (if true then two() else 0), 9
		r = b`)
	assertGlobalEqual(t, v, "r", int64(9))
}

func TestIfExpressionInCallArgs(t *testing.T) {
	v := run(t, `
		local function pick(a, b) return a .. b end
		r = pick(if false then "x" else "y", if true then "z" else "w")`)
	assertGlobalEqual(t, v, "r", "yz")
}

func TestDefaultParamOmittedAndNil(t *testing.T) {
	v := run(t, `
		local function greet(name, greeting = "hello")
			return greeting .. " " .. name
		end
		a = greet("w")
		b = greet("w", nil)
		c = greet("w", "hi")`)
	assertGlobalEqual(t, v, "a", "hello w")
	assertGlobalEqual(t, v, "b", "hello w")
	assertGlobalEqual(t, v, "c", "hi w")
}

func TestDefaultParamFalseIsNotNil(t *testing.T) {
	v := run(t, `
		local function f(flag = true)
			return flag
		end
		r = f(false)`)
	assertGlobalEqual(t, v, "r", false)
}

func TestDefaultParamSeesEarlierParams(t *testing.T) {
	v := run(t, `
		local function f(a, b = a * 2)
			return a + b
		end
		r = f(5)`)
	assertGlobalEqual(t, v, "r", int64(15))
}

func TestDefaultParamEvaluatedPerCall(t *testing.T) {
	v := run(t, `
		local n = 0
		local function next(step = 1)
			n = n + step
			return n
		end
		next()
		next()
		r = next(10)`)
	assertGlobalEqual(t, v, "r", int64(12))
}

func TestDefaultParamOnMethods(t *testing.T) {
	v := run(t, `
		local obj = { base = 10 }
		function obj:add(x = 5)
			return self.base + x
		end
		r = obj:add() + obj:add(1)`)
	assertGlobalEqual(t, v, "r", int64(26))
}

func TestNewSyntaxInREPLMode(t *testing.T) {
	v := New()
	for _, src := range []string{
		`local mode = if 2 > 1 then "up" else "down"`,
		`local function tally(n, step = 2)
			local total = 0
			for i = 1, n do
				if i % 2 == 0 then continue end
				total = total + step
			end
			return total
		end
		result = mode .. ":" .. tally(5)`,
	} {
		chunks, err := compiler.CompileToInstructions(src, parser.REPLMode)
		if err != nil {
			t.Fatalf("REPL-mode compile error: %v\nsource:\n%s", err, src)
		}
		if err := v.Run(chunks[0]); err != nil {
			t.Fatalf("REPL-mode run error: %v\nsource:\n%s", err, src)
		}
	}
	assertGlobalEqual(t, v, "result", "up:6")
}

func TestConstAssignRejected(t *testing.T) {
	msg := compileErr(t, "local x <const> = 1\nx = 2")
	if !strings.Contains(msg, "cannot assign to const variable 'x'") {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestConstCompoundAssignRejected(t *testing.T) {
	msg := compileErr(t, "local x <const> = 1\nx += 1")
	if !strings.Contains(msg, "cannot assign to const variable 'x'") {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestConstUpvalueAssignRejected(t *testing.T) {
	msg := compileErr(t, `
		local x <const> = 1
		local function f() x = 2 end`)
	if !strings.Contains(msg, "cannot assign to const variable 'x'") {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestCloseAssignRejected(t *testing.T) {
	msg := compileErr(t, "local x <close> = nil\nx = 2")
	if !strings.Contains(msg, "cannot assign to close variable 'x'") {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestConstEnforcedUnderNocheck(t *testing.T) {
	msg := compileErr(t, "--!nocheck\nlocal x <const> = 1\nx = 2")
	if !strings.Contains(msg, "cannot assign to const variable 'x'") {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestConstShadowingIsAssignable(t *testing.T) {
	v := run(t, `
		local x <const> = 1
		do
			local x = 2
			x = 3
			r = x
		end`)
	assertGlobalEqual(t, v, "r", int64(3))
}

func TestConstFieldWriteAllowed(t *testing.T) {
	v := run(t, `
		local t <const> = {}
		t.x = 42
		r = t.x`)
	assertGlobalEqual(t, v, "r", int64(42))
}

func TestInterpolationEscapes(t *testing.T) {
	v := run(t, "\n"+`
		local a = 5
		brace   = `+"`"+`\u{7B}x\u{7D}`+"`"+`
		quoted  = `+"`"+`v={"a\"b"}`+"`"+`
		json    = `+"`"+`{'{"k":1}'}`+"`"+`
		mixed   = `+"`"+`tab\tv={a}\u{7D}`+"`"+`
		esc     = `+"`"+`back\slash {a}`+"`"+`
		tick    = `+"`"+`\`+"`"+` {a}`+"`"+`
		plain   = `+"`"+`no interpolation here`+"`"+`
	`)
	assertGlobalEqual(t, v, "brace", "{x}")
	assertGlobalEqual(t, v, "quoted", `v=a"b`)
	assertGlobalEqual(t, v, "json", `{"k":1}`)
	assertGlobalEqual(t, v, "mixed", "tab\tv=5}")
	assertGlobalEqual(t, v, "esc", `back\slash 5`)
	assertGlobalEqual(t, v, "tick", "` 5")
	assertGlobalEqual(t, v, "plain", "no interpolation here")
}
