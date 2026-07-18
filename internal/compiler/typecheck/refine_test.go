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

func TestRefine_AssignDeclaredTypeInsideGuardIsLegal(t *testing.T) {
	// The narrowing shadow must not make `s = nil` illegal: the assignment
	// is checked against the declared `string?`, not the branch's `string`.
	expectOK(t, `
--!strict
local s: string? = "x"
if s ~= nil then
    s = nil
end
`)
}

func TestRefine_AssignmentInvalidatesNarrowing(t *testing.T) {
	// After `s = nil` the branch's `string` narrowing is stale; using s as
	// a plain string must fail again.
	expectErrContains(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
if s ~= nil then
    s = nil
    need(s)
end
`, "could not be converted")
}

func TestRefine_AssignmentStillChecksDeclaredType(t *testing.T) {
	// Seeing through the shadow must not mean seeing `any`: a number is
	// still not assignable to the declared `string?`.
	expectErrContains(t, `
--!strict
local s: string? = "x"
if s ~= nil then
    s = 5
end
`, "could not be converted")
}

func TestRefine_EarlyReturnPersistsNegation(t *testing.T) {
	// The guard clause always returns, so past the `end` s is `string`.
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local function f(s: string?): number
    if s == nil then
        return 0
    end
    return need(s)
end
`)
}

func TestRefine_EarlyErrorCallPersistsNegation(t *testing.T) {
	// error() never returns, so it terminates a clause like `return` does.
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
if type(x) ~= "number" then
    error("expected a number")
end
double(x)
`)
}

func TestRefine_EarlyBreakPersistsNegationInLoop(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local function get(): string? return nil end
while true do
    local s = get()
    if s == nil then break end
    need(s)
end
`)
}

func TestRefine_NonTerminatingThenDoesNotPersist(t *testing.T) {
	// The clause can fall through, so nothing may leak past the `end`.
	expectErrContains(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
if s == nil then
    print("still nil")
end
need(s)
`, "could not be converted")
}

func TestRefine_TerminatingPrefixStopsAtFallthroughClause(t *testing.T) {
	// Clause 1 terminates but clause 2 doesn't — only clause 1's negation
	// may persist, so n stays optional after the statement.
	expectErrContains(t, `
--!strict
local function dbl(n: number): number return n * 2 end
local s: string? = nil
local n: number? = nil
if s == nil then
    return
elseif n == nil then
    print("n is nil")
end
dbl(n)
`, "could not be converted")
}

func TestRefine_AssertNilGuardPersists(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
assert(s ~= nil)
need(s)
`)
}

func TestRefine_AssertTypeGuardPersists(t *testing.T) {
	expectOK(t, `
--!strict
local function double(n: number): number return n * 2 end
local x: number | string = 1
assert(type(x) == "number", "x must be a number")
double(x)
`)
}

func TestRefine_AssertTruthinessPersists(t *testing.T) {
	// Bare `assert(s)` drops nil like `if s then` does.
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
assert(s)
need(s)
`)
}

func TestRefine_AndRhsSeesLhsNarrowing(t *testing.T) {
	// The RHS of `and` only evaluates when the LHS held.
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
local n = s ~= nil and need(s) or 0
print(n)
`)
}

func TestRefine_OrResultDropsNil(t *testing.T) {
	// `s or default` can never yield nil, so it satisfies a plain string.
	expectOK(t, `
--!strict
local s: string? = nil
local t: string = s or "default"
print(t)
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
