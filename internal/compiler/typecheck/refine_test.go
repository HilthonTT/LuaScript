package typecheck

import "testing"

// These exercise type refinement (narrowing) inside conditional branches.
// The pattern: a value of a wide type is used in a position that only its
// narrowed form satisfies. Without narrowing the use is an error; with it,
// the branch type-checks.

func TestRefine_NilGuard_NarrowsInThenBranch(t *testing.T) {
	// `s` is string?; the `~= nil` then-branch must see it as `string` so
	// passing it where a `string` is required is legal.
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
if s ~= nil then
    need(s)
end
`)
}

func TestRefine_NilGuard_StillOptionalOutsideBranch(t *testing.T) {
	// Outside the guard the optional is unchanged, so the call is rejected.
	expectErrContains(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
need(s)
`, "could not be converted")
}

func TestRefine_NilGuard_ElseBranchIsNil(t *testing.T) {
	// In the else-branch of `s ~= nil`, s is nil — assigning it to a plain
	// string slot must fail.
	expectErrContains(t, `
--!strict
local s: string? = nil
if s ~= nil then
else
    local t: string = s
end
`, "could not be converted")
}

func TestRefine_EqNil_ThenBranchIsNil(t *testing.T) {
	expectErrContains(t, `
--!strict
local s: string? = nil
if s == nil then
    local t: string = s
end
`, "could not be converted")
}

func TestRefine_TypeGuard_NarrowsUnion(t *testing.T) {
	// number|string narrowed to number inside `type(x) == "number"`.
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
if type(x) == "number" then
    double(x)
end
`)
}

func TestRefine_TypeGuard_WrongBranchRejected(t *testing.T) {
	// Inside the string branch, x is string, so a numeric call is wrong.
	expectErrContains(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
if type(x) == "string" then
    double(x)
end
`, "could not be converted")
}

func TestRefine_TypeGuard_NotEqRemovesMember(t *testing.T) {
	// `type(x) ~= "string"` leaves number as the only member.
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
if type(x) ~= "string" then
    double(x)
end
`)
}

func TestRefine_TypeofBuiltinAlsoNarrows(t *testing.T) {
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
if typeof(x) == "number" then
    double(x)
end
`)
}

func TestRefine_NotNegatesPolarity(t *testing.T) {
	// `if not (s == nil)` ≡ `if s ~= nil` — then-branch is non-nil.
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
if not (s == nil) then
    need(s)
end
`)
}

func TestRefine_Truthiness_NonNilInThenBranch(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
if s then
    need(s)
end
`)
}

func TestRefine_And_PropagatesBothToThen(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local function dbl(n: number): number return n * 2 end
local s: string? = nil
local n: number? = nil
if s ~= nil and n ~= nil then
    need(s)
    dbl(n)
end
`)
}

func TestRefine_Or_PropagatesBothToElse(t *testing.T) {
	// `if s == nil or n == nil then ... else <both non-nil> end`
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local function dbl(n: number): number return n * 2 end
local s: string? = nil
local n: number? = nil
if s == nil or n == nil then
else
    need(s)
    dbl(n)
end
`)
}

func TestRefine_ElseifAccumulatesNegation(t *testing.T) {
	// number|string|boolean: after the `number` and `string` clauses fail,
	// the else must see boolean as the only remaining member.
	expectOK(t, `
--!strict
local function flip(b: boolean): boolean return not b end
local x: number | string | boolean = true
if type(x) == "number" then
elseif type(x) == "string" then
else
    flip(x)
end
`)
}

func TestRefine_GradualAnyRefinedByGuard(t *testing.T) {
	// The headline gradual win: an untyped local is `any`; a type guard
	// refines it to the guarded primitive so a typed call accepts it.
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: any = 1
if type(x) == "number" then
    double(x)
end
`)
}

func TestRefine_WhileConditionNarrowsBody(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
while s ~= nil do
    need(s)
end
`)
}

func TestRefine_DoesNotLeakPastBranch(t *testing.T) {
	// After the if-statement, the narrowing is gone and the optional is
	// optional again.
	expectErrContains(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
if s ~= nil then
    need(s)
end
need(s)
`, "could not be converted")
}
