package typecheck

import "testing"

func TestRefine_NilGuard_NarrowsInThenBranch(t *testing.T) {
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
	expectErrContains(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = nil
need(s)
`, "could not be converted")
}

func TestRefine_NilGuard_ElseBranchIsNil(t *testing.T) {
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
	expectOK(t, `
--!strict
local s: string? = "x"
if s ~= nil then
    s = nil
end
`)
}

func TestRefine_AssignmentInvalidatesNarrowing(t *testing.T) {
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
	expectErrContains(t, `
--!strict
local s: string? = "x"
if s ~= nil then
    s = 5
end
`, "could not be converted")
}

func TestRefine_EarlyReturnPersistsNegation(t *testing.T) {
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
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
assert(s)
need(s)
`)
}

func TestRefine_AndRhsSeesLhsNarrowing(t *testing.T) {
	expectOK(t, `
--!strict
local function need(x: string): number return #x end
local s: string? = "x"
local n = s ~= nil and need(s) or 0
print(n)
`)
}

func TestRefine_OrResultDropsNil(t *testing.T) {
	expectOK(t, `
--!strict
local s: string? = nil
local t: string = s or "default"
print(t)
`)
}

func TestRefine_DoesNotLeakPastBranch(t *testing.T) {
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
