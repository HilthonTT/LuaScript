package vm

import "testing"

// Execution-level tests for the `match` statement. `match` is lowered by the
// bytecode generator into a test-and-branch chain (see
// bytecode/match_generation.go), so these cover the semantics that lowering
// has to preserve: single evaluation, first-match-wins ordering, guards
// falling through, binder scoping, and control flow escaping an arm.

func TestMatchValueArmSelected(t *testing.T) {
	v := run(t, `
		match 2 do
			1 -> r = "one"
			2 -> r = "two"
			3 -> r = "three"
		end
	`)
	assertGlobalEqual(t, v, "r", "two")
}

func TestMatchAlternativesInOneArm(t *testing.T) {
	v := run(t, `
		match 3 do
			1, 2, 3 -> r = "small"
			_ -> r = "big"
		end
	`)
	assertGlobalEqual(t, v, "r", "small")
}

func TestMatchFallsThroughToWildcard(t *testing.T) {
	v := run(t, `
		match 99 do
			1 -> r = "one"
			_ -> r = "other"
		end
	`)
	assertGlobalEqual(t, v, "r", "other")
}

// A match that matches nothing is a no-op, not an error — `match` is not
// exhaustive and has no fallback.
func TestMatchNoArmMatchesIsNoOp(t *testing.T) {
	v := run(t, `
		r = "untouched"
		match 42 do
			1 -> r = "one"
			2 -> r = "two"
		end
	`)
	assertGlobalEqual(t, v, "r", "untouched")
}

// The scrutinee must be evaluated exactly once no matter how many arms are
// tested before one matches.
func TestMatchEvaluatesSubjectOnce(t *testing.T) {
	v := run(t, `
		calls = 0
		function subject()
			calls = calls + 1
			return 4
		end
		match subject() do
			1 -> r = "one"
			2 -> r = "two"
			3 -> r = "three"
			4 -> r = "four"
		end
	`)
	assertGlobalEqual(t, v, "calls", int64(1))
	assertGlobalEqual(t, v, "r", "four")
}

// The behaviour the old `__matched` flag existed to protect: an arm whose
// pattern matches but whose guard fails must NOT swallow the value — control
// continues to later arms.
func TestMatchFailedGuardFallsThroughToLaterArm(t *testing.T) {
	v := run(t, `
		match 5 do
			n: number if n > 100 -> r = "huge"
			n: number if n > 10  -> r = "big"
			n: number            -> r = "small"
		end
	`)
	assertGlobalEqual(t, v, "r", "small")
}

func TestMatchGuardSeesPatternBinding(t *testing.T) {
	v := run(t, `
		match 7 do
			n: number if n % 2 == 1 -> r = n * 2
			_ -> r = 0
		end
	`)
	assertGlobalEqual(t, v, "r", int64(14))
}

func TestMatchTypedPatternDispatchesOnRuntimeType(t *testing.T) {
	src := `
		function describe(v)
			match v do
				n: number -> return "number"
				s: string -> return "string"
				b: boolean -> return "boolean"
				_ -> return "other"
			end
		end
		a = describe(1)
		b = describe("hi")
		c = describe(true)
		d = describe(nil)
	`
	v := run(t, src)
	assertGlobalEqual(t, v, "a", "number")
	assertGlobalEqual(t, v, "b", "string")
	assertGlobalEqual(t, v, "c", "boolean")
	assertGlobalEqual(t, v, "d", "other")
}

// `x: any` always matches, so it must behave exactly like a wildcard that
// also binds.
func TestMatchAnyPatternAlwaysMatches(t *testing.T) {
	v := run(t, `
		match false do
			x: any -> r = "bound"
		end
	`)
	assertGlobalEqual(t, v, "r", "bound")
}

// `false` is a legitimate value to match on — it must not read as "no match".
func TestMatchOnFalseSubject(t *testing.T) {
	v := run(t, `
		match false do
			true -> r = "yes"
			false -> r = "no"
		end
	`)
	assertGlobalEqual(t, v, "r", "no")
}

// Destructuring a REAL enum/struct value is not testable here: constructing
// one needs `__enum_freeze` from internal/native/stdlib/enumrt, and that
// package imports this one. The positive path is covered end-to-end by
// examples/43_tagged_enums.lsc and examples/44_match.lsc. What is testable
// here is the shape check that must run before any projection.

// A positional pattern probes `__tag` on a plain table, so a hand-rolled
// value with the right tag destructures like the real thing.
func TestMatchPositionalDestructureReadsTagAndPayload(t *testing.T) {
	v := run(t, `
		function __enum_freeze(t, n) return t end
		enum Shape Circle(number), Rect(number, number) end
		local fake = { __tag = "Rect", 3, 4 }
		match fake do
			Shape.Rect(w, h) -> r = w * h
			_ -> r = 0
		end
	`)
	assertGlobalEqual(t, v, "r", int64(12))
}

// An `_` position holds its slot but binds no name, so the following binder
// must still read the correct payload index.
func TestMatchPositionalUnderscoreSkipsSlot(t *testing.T) {
	v := run(t, `
		function __enum_freeze(t, n) return t end
		enum Shape Rect(number, number) end
		local fake = { __tag = "Rect", 3, 4 }
		match fake do
			Shape.Rect(_, h) -> r = h
		end
	`)
	assertGlobalEqual(t, v, "r", int64(4))
}

// A tag mismatch must fall through without projecting.
func TestMatchPositionalRejectsWrongTag(t *testing.T) {
	v := run(t, `
		function __enum_freeze(t, n) return t end
		enum Shape Circle(number), Rect(number, number) end
		local fake = { __tag = "Circle", 5 }
		match fake do
			Shape.Rect(w, h) -> r = "rect"
			_ -> r = "not a rect"
		end
	`)
	assertGlobalEqual(t, v, "r", "not a rect")
}

// A destructure pattern tests the tag before projecting, so a value of the
// wrong shape must fall through rather than fault on a missing field.
func TestMatchDestructureRejectsWrongShape(t *testing.T) {
	v := run(t, `
		function __struct_define(n, f) return {} end
		struct Point { x: number, y: number }
		match 42 do
			Point{ x = px, y = py } -> r = "point"
			_ -> r = "not a point"
		end
	`)
	assertGlobalEqual(t, v, "r", "not a point")
}

// Arm binders are scoped to their arm and must not survive the `end`.
func TestMatchBinderDoesNotEscapeArm(t *testing.T) {
	v := run(t, `
		match 5 do
			n: number -> inner = n
		end
		leaked = n
	`)
	assertGlobalEqual(t, v, "inner", int64(5))
	assertGlobalEqual(t, v, "leaked", nil)
}

func TestMatchNested(t *testing.T) {
	v := run(t, `
		function grid(a, b)
			match a do
				1 -> match b do
					1 -> return "1,1"
					2 -> return "1,2"
					_ -> return "1,?"
				end
				_ -> return "?,?"
			end
		end
		p = grid(1, 2)
		q = grid(1, 9)
		s = grid(3, 1)
	`)
	assertGlobalEqual(t, v, "p", "1,2")
	assertGlobalEqual(t, v, "q", "1,?")
	assertGlobalEqual(t, v, "s", "?,?")
}

// `break` inside an arm belongs to the enclosing loop, not the match — one of
// the reasons match is a real statement rather than a closure lowering.
func TestMatchBreakEscapesEnclosingLoop(t *testing.T) {
	v := run(t, `
		count = 0
		for i = 1, 10 do
			count = count + 1
			match i do
				3 -> break
			end
		end
	`)
	assertGlobalEqual(t, v, "count", int64(3))
}

func TestMatchContinueSkipsIteration(t *testing.T) {
	v := run(t, `
		total = 0
		for i = 1, 5 do
			match i do
				2, 4 -> continue
			end
			total = total + i
		end
	`)
	assertGlobalEqual(t, v, "total", int64(9)) // 1 + 3 + 5
}

// A closure created in an arm must capture that arm's binder, and the capture
// must survive the match's scope exit.
func TestMatchArmClosureCapturesBinder(t *testing.T) {
	v := run(t, `
		match 21 do
			n: number -> f = function() return n * 2 end
		end
		r = f()
	`)
	assertGlobalEqual(t, v, "r", int64(42))
}
