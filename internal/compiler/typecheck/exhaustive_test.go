package typecheck

import "testing"

// `match` exhaustiveness. Note the `;` after an arm body ending in a string
// literal: in Lua `f"x"` chains as a call, so a following string pattern
// needs the separator.

const taggedShape = `enum Shape Circle(number), Rect(number, number), Unit, end
`

func TestMatchTaggedEnumExhaustive(t *testing.T) {
	expectOK(t, taggedShape+`local function area(s: Shape): number
		match s do
			Shape.Circle(r)  -> return r
			Shape.Rect(w, h) -> return w * h
			Shape.Unit       -> return 0
		end
		return 0
	end`)
}

func TestMatchTaggedEnumMissingVariant(t *testing.T) {
	expectErrContains(t, taggedShape+`local function area(s: Shape): number
		match s do
			Shape.Circle(r)  -> return r
			Shape.Rect(w, h) -> return w * h
		end
		return 0
	end`, "does not handle 1 case: Shape.Unit")
}

func TestMatchWildcardIsAlwaysExhaustive(t *testing.T) {
	expectOK(t, taggedShape+`local function area(s: Shape): number
		match s do
			Shape.Circle(r) -> return r
			_               -> return 0
		end
		return 0
	end`)
}

// `x: any` matches anything, so it closes the match just like `_`.
func TestMatchTypedAnyArmIsExhaustive(t *testing.T) {
	expectOK(t, taggedShape+`local function area(s: Shape): number
		match s do
			Shape.Circle(r) -> return r
			x: any          -> return 0
		end
		return 0
	end`)
}

// A guard can fail, so a guarded arm never counts as covering its case.
func TestMatchGuardedArmDoesNotCover(t *testing.T) {
	expectErrContains(t, taggedShape+`local function area(s: Shape): number
		match s do
			Shape.Circle(r) if r > 0 -> return r
			Shape.Rect(w, h)         -> return w * h
			Shape.Unit               -> return 0
		end
		return 0
	end`, "does not handle 1 case: Shape.Circle")
}

func TestMatchLiteralUnionExhaustive(t *testing.T) {
	expectOK(t, `type Mode = "read" | "write"
	local function verb(m: Mode): string
		match m do
			"read"  -> return "r";
			"write" -> return "w"
		end
		return "?"
	end`)
}

func TestMatchLiteralUnionMissingCase(t *testing.T) {
	expectErrContains(t, `type Mode = "read" | "write" | "append"
	local function verb(m: Mode): string
		match m do
			"read"  -> return "r";
			"write" -> return "w"
		end
		return "?"
	end`, `does not handle 1 case: "append"`)
}

// Comma-joined alternatives cover every value they list.
func TestMatchAlternativesCoverAll(t *testing.T) {
	expectOK(t, `type Mode = "read" | "write" | "append"
	local function verb(m: Mode): string
		match m do
			"read", "write" -> return "rw";
			"append"        -> return "a"
		end
		return "?"
	end`)
}

// A classic enum's members are named in the diagnostic, not numbered.
func TestMatchClassicEnumNamesMissingMembers(t *testing.T) {
	expectErrContains(t, `enum Color RED, GREEN, BLUE end
	local function hex(c: Color): string
		match c do
			Color.RED -> return "#f00"
		end
		return "?"
	end`, "does not handle 2 cases: Color.GREEN, Color.BLUE")
}

// The subject flows into `number`, so the typed arm covers the whole enum.
func TestMatchTypedArmCoveringSubjectIsExhaustive(t *testing.T) {
	expectOK(t, `enum Color RED, GREEN, BLUE end
	local function hex(c: Color): string
		match c do
			n: number -> return "c" .. n
		end
		return "?"
	end`)
}

func TestMatchBooleanExhaustiveness(t *testing.T) {
	expectOK(t, `local function label(b: boolean): string
		match b do
			true  -> return "yes";
			false -> return "no"
		end
		return "?"
	end`)
	expectErrContains(t, `local function label(b: boolean): string
		match b do
			true -> return "yes"
		end
		return "?"
	end`, "does not handle 1 case: false")
}

// Untyped code has no finite domain to check, so nothing is reported —
// this is what keeps the diagnostic opt-in via types.
func TestMatchUntypedSubjectIsNotChecked(t *testing.T) {
	expectOK(t, `local function classify(n)
		match n do
			0 -> return "zero";
			1 -> return "one"
		end
		return "other"
	end`)
	expectOK(t, `local function f(s: string): string
		match s do
			"a" -> return "A"
		end
		return "?"
	end`)
}

// An inline literal scrutinee is a singleton type, but not a domain anyone
// declared — demanding an arm for it would be noise.
func TestMatchLiteralScrutineeIsNotChecked(t *testing.T) {
	expectOK(t, `local r = ""
	match 42 do
		1 -> r = "one";
		2 -> r = "two"
	end`)
}
