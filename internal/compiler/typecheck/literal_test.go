package typecheck

import "testing"

func TestLiteralAnnotationAccepted(t *testing.T) {
	expectOK(t, `local m: "read" = "read"`)
	expectOK(t, `local n: 42 = 42`)
	expectOK(t, `local b: true = true`)
	expectOK(t, `local neg: -1 = -1`)
	expectOK(t, `local f: 1 = 1.0`)
}

func TestLiteralAnnotationRejectsOtherValue(t *testing.T) {
	expectErrContains(t, `local m: "read" = "write"`, `'"write"'`)
	expectErrContains(t, `local n: 42 = 41`, `"41"`)
	expectErrContains(t, `local b: true = false`, `"false"`)
}

func TestLiteralUnion(t *testing.T) {
	src := `type Mode = "read" | "write"
	local function f(m: Mode): string return m end
	`
	expectOK(t, src+`f("read")`)
	expectOK(t, src+`f("write")`)
	expectErrContains(t, src+`f("append")`, `'"append"'`)
}

func TestLiteralWidensToPrimitive(t *testing.T) {
	expectOK(t, `local s: string = "read"`)
	expectOK(t, `local n: number = 42`)
	expectErrContains(t, `local s: string = "x"
	local m: "x" = s`, `"string"`)
}

func TestInferredLocalWidens(t *testing.T) {
	expectOK(t, `local mode = "read"
	mode = "write"`)
	expectOK(t, `local n = 1
	n = 2`)
	expectErrContains(t, `local mode: "read" = "read"
	mode = "write"`, `'"write"'`)
}

func TestGenericInferenceWidensLiteral(t *testing.T) {
	expectOK(t, `local function identity<T>(v: T): T return v end
	local s: string = identity("hi")`)
}

func TestLiteralNarrowing(t *testing.T) {
	expectOK(t, `type Mode = "read" | "write"
	local function f(m: Mode): string
		if m == "read" then
			local r: "read" = m
			return r
		end
		local w: "write" = m
		return w
	end`)
}

func TestTypeGuardOnLiteralUnion(t *testing.T) {
	expectOK(t, `type Tag = "a" | 1
	local function f(v: Tag): string
		if type(v) == "string" then
			local s: "a" = v
			return s
		end
		return "n"
	end`)
}

func TestClassicEnumIsLiteralUnion(t *testing.T) {
	src := `enum Color RED, GREEN, BLUE end
	local function paint(c: Color): number return c end
	`
	expectOK(t, src+`paint(Color.RED)`)
	expectOK(t, src+`paint(2)`)
	expectErrContains(t, src+`paint(99)`, `"99"`)
	expectErrContains(t, src+`paint("red")`, `'"red"'`)
}

func TestClassicEnumMemberIsSingleton(t *testing.T) {
	expectOK(t, `enum Color RED, GREEN end
	local r: 1 = Color.RED
	local g: 2 = Color.GREEN`)
	expectErrContains(t, `enum Color RED, GREEN end
	local x = Color.REDD`, `no field "REDD"`)
}

func TestErrorMessageWidensWhenTargetIsPrimitive(t *testing.T) {
	expectErrContains(t, `local n: number = "two"`, `Type "string"`)
	expectErrContains(t, `type Mode = "read"
	local m: Mode = "two"`, `Type '"two"'`)
}
