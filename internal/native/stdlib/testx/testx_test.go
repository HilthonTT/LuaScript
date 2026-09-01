package testx_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/native/stdlib/testx"
	"github.com/hilthontt/luascript/internal/vm"
)

func runSuite(t *testing.T, src string, tune func(*testx.Registry)) *testx.Registry {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	chunks[0].SetSource("suite.lsc")

	reg := testx.NewRegistry()
	if tune != nil {
		tune(reg)
	}
	v := vm.New()
	testx.Install(v, reg)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return reg
}

func names(reg *testx.Registry) []string {
	out := make([]string, 0, len(reg.Results))
	for _, r := range reg.Results {
		out = append(out, r.Status.String()+" "+r.Name)
	}
	return out
}

func wantResults(t *testing.T, reg *testx.Registry, want ...string) {
	t.Helper()
	got := names(reg)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("results =\n  %s\nwant\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestPassFailSkipAreRecorded(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("passes", function() t.assert_eq(1, 1) end)
		t.test("fails", function() t.assert_eq(1, 2) end)
		t.skip("skipped")
	`, nil)

	wantResults(t, reg, "pass passes", "fail fails", "skip skipped")

	pass, fail, skip := reg.Counts()
	if pass != 1 || fail != 1 || skip != 1 {
		t.Errorf("counts = (%d, %d, %d), want (1, 1, 1)", pass, fail, skip)
	}
	if !reg.Failed() {
		t.Error("Failed() = false, want true")
	}
}

func TestFailureCarriesPosition(t *testing.T) {
	reg := runSuite(t, `local t = require("test")
t.test("fails", function()
  t.assert_eq(1, 2)
end)`, nil)

	if len(reg.Results) != 1 {
		t.Fatalf("recorded %d results, want 1", len(reg.Results))
	}
	msg := reg.Results[0].Message
	if !strings.HasPrefix(msg, "suite.lsc:3:") {
		t.Errorf("message = %q, want it to start with the assertion's position", msg)
	}
	if !strings.Contains(msg, "expected: 2") || !strings.Contains(msg, "actual:   1") {
		t.Errorf("message = %q, want it to show both values", msg)
	}
}

func TestFailureCarriesTraceback(t *testing.T) {
	reg := runSuite(t, `local t = require("test")
local function helper() t.assert_eq(1, 2) end
t.test("fails", function() helper() end)`, nil)

	stack := reg.Results[0].Stack
	if !strings.Contains(stack, "stack traceback:") {
		t.Fatalf("stack = %q, want a rendered traceback", stack)
	}
	if !strings.Contains(stack, "helper") {
		t.Errorf("stack = %q, want the intermediate frame named", stack)
	}
	if strings.Contains(stack, "main chunk") {
		t.Errorf("stack = %q, want the caller's frames excluded", stack)
	}
}

func TestDescribeNestsNames(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.describe("outer", function()
			t.test("a", function() end)
			t.describe("inner", function()
				t.test("b", function() end)
			end)
		end)
		t.test("top", function() end)
	`, nil)

	wantResults(t, reg, "pass outer/a", "pass outer/inner/b", "pass top")
}

func TestDescribeBodyFailureIsContained(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.describe("broken", function() error("declaration failed") end)
		t.test("still runs", function() end)
	`, nil)

	wantResults(t, reg, "fail broken", "pass still runs")
	if !strings.Contains(reg.Results[0].Message, "declaration failed") {
		t.Errorf("message = %q, want the raised error", reg.Results[0].Message)
	}
}

func TestHookOrder(t *testing.T) {
	src := `
		local t = require("test")
		log = {}
		local function note(s) log[#log + 1] = s end
		t.describe("outer", function()
			t.before_each(function() note("outer-before") end)
			t.after_each(function() note("outer-after") end)
			t.describe("inner", function()
				t.before_each(function() note("inner-before") end)
				t.after_each(function() note("inner-after") end)
				t.test("x", function() note("body") end)
			end)
		end)
	`
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	testx.Install(v, testx.NewRegistry())
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v", err)
	}

	logTable, ok := v.Globals.Get("log").(*vm.Table)
	if !ok {
		t.Fatal("log global is not a table")
	}
	var got []string
	for i := int64(1); i <= logTable.Len(); i++ {
		got = append(got, vm.ToString(logTable.Get(i)))
	}
	want := []string{"outer-before", "inner-before", "body", "inner-after", "outer-after"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("hook order = %v, want %v", got, want)
	}
}

func TestAfterEachRunsWhenSetupFails(t *testing.T) {
	src := `
		local t = require("test")
		cleaned = false
		t.before_each(function() error("setup exploded") end)
		t.after_each(function() cleaned = true end)
		t.test("x", function() end)
	`
	chunks, _ := compiler.CompileToInstructions(src, parser.NormalMode)
	v := vm.New()
	reg := testx.NewRegistry()
	testx.Install(v, reg)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v", err)
	}

	if got := v.Globals.Get("cleaned"); got != true {
		t.Errorf("cleaned = %v, want true", got)
	}
	if len(reg.Results) != 1 || reg.Results[0].Status != testx.StatusFail {
		t.Fatalf("results = %v, want one failure", names(reg))
	}
	if !strings.Contains(reg.Results[0].Message, "before_each:") {
		t.Errorf("message = %q, want it attributed to before_each", reg.Results[0].Message)
	}
}

func TestFilterSelectsBySubstringAndPattern(t *testing.T) {
	src := `
		local t = require("test")
		t.describe("math", function()
			t.test("rounds down", function() end)
			t.test("rounds up", function() end)
		end)
		t.test("unrelated", function() end)
	`
	cases := []struct {
		filter string
		want   []string
	}{
		{"rounds", []string{"pass math/rounds down", "pass math/rounds up"}},
		{"math/", []string{"pass math/rounds down", "pass math/rounds up"}},
		{"^math", []string{"pass math/rounds down", "pass math/rounds up"}},
		{"up$", []string{"pass math/rounds up"}},
		{"nothing", nil},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			reg := runSuite(t, src, func(r *testx.Registry) { r.Filter = tc.filter })
			wantResults(t, reg, tc.want...)
		})
	}
}

func TestListOnlyRecordsWithoutRunning(t *testing.T) {
	src := `
		local t = require("test")
		ran = false
		t.test("a", function() ran = true end)
	`
	chunks, _ := compiler.CompileToInstructions(src, parser.NormalMode)
	v := vm.New()
	reg := testx.NewRegistry()
	reg.ListOnly = true
	testx.Install(v, reg)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v", err)
	}
	if got := v.Globals.Get("ran"); got == true {
		t.Error("the test body ran under ListOnly")
	}
	wantResults(t, reg, "skip a")
}

func TestFailFastStopsRecording(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("a", function() end)
		t.test("b", function() t.fail("stop here") end)
		t.test("c", function() end)
	`, func(r *testx.Registry) { r.FailFast = true })

	wantResults(t, reg, "pass a", "fail b")
	if !reg.Aborted() {
		t.Error("Aborted() = false, want true")
	}
}

func TestOnResultSeesEveryResult(t *testing.T) {
	var seen []string
	runSuite(t, `
		local t = require("test")
		t.test("a", function() end)
		t.test("b", function() t.fail() end)
	`, func(r *testx.Registry) {
		r.OnResult = func(res testx.Result) { seen = append(seen, res.Name) }
	})
	if strings.Join(seen, ",") != "a,b" {
		t.Errorf("OnResult saw %v, want [a b]", seen)
	}
}

func TestAssertions(t *testing.T) {
	cases := []struct {
		name string
		pass string
		fail string
	}{
		{"assert_eq", `t.assert_eq(1, 1.0)`, `t.assert_eq(1, 2)`},
		{"assert_ne", `t.assert_ne(1, 2)`, `t.assert_ne("a", "a")`},
		{"assert_deep_eq", `t.assert_deep_eq({1,{2}}, {1,{2}})`, `t.assert_deep_eq({1,{2}}, {1,{3}})`},
		{"assert_deep_eq_len", `t.assert_deep_eq({a=1}, {a=1})`, `t.assert_deep_eq({a=1}, {a=1,b=2})`},
		{"assert_true", `t.assert_true(0)`, `t.assert_true(nil)`},
		{"assert_false", `t.assert_false(false)`, `t.assert_false(0)`},
		{"assert_nil", `t.assert_nil(nil)`, `t.assert_nil(false)`},
		{"assert_not_nil", `t.assert_not_nil(false)`, `t.assert_not_nil(nil)`},
		{"assert_near", `t.assert_near(1.0, 1.0000000001)`, `t.assert_near(1.0, 1.1)`},
		{"assert_near_eps", `t.assert_near(1.0, 1.05, 0.1)`, `t.assert_near(1.0, 1.05, 0.01)`},
		{"assert_type", `t.assert_type("s", "string")`, `t.assert_type("s", "number")`},
		{"assert_len_string", `t.assert_len("abc", 3)`, `t.assert_len("abc", 4)`},
		{"assert_len_table", `t.assert_len({1,2}, 2)`, `t.assert_len({1,2}, 3)`},
		{"assert_contains_string", `t.assert_contains("hello", "ell")`, `t.assert_contains("hello", "xyz")`},
		{"assert_contains_table", `t.assert_contains({1,2,3}, 2)`, `t.assert_contains({1,2,3}, 9)`},
		{"assert_match", `t.assert_match("v1.2", "%d%.%d")`, `t.assert_match("nope", "%d%.%d")`},
		{"assert_error", `t.assert_error(function() error("x") end)`, `t.assert_error(function() end)`},
		{"assert_error_pattern", `t.assert_error(function() error("disk full") end, "disk")`,
			`t.assert_error(function() error("disk full") end, "network")`},
		{"assert_no_error", `t.assert_no_error(function() end)`, `t.assert_no_error(function() error("x") end)`},
		{"fail", `local _ = 1`, `t.fail("nope")`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := runSuite(t, `
				local t = require("test")
				t.test("should pass", function() `+tc.pass+` end)
				t.test("should fail", function() `+tc.fail+` end)
			`, nil)
			if len(reg.Results) != 2 {
				t.Fatalf("recorded %d results, want 2", len(reg.Results))
			}
			if reg.Results[0].Status != testx.StatusPass {
				t.Errorf("%s should have passed: %s", tc.pass, reg.Results[0].Message)
			}
			if reg.Results[1].Status != testx.StatusFail {
				t.Errorf("%s should have failed", tc.fail)
			}
		})
	}
}

func TestAssertErrorReturnsTheValue(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("x", function()
			local err = t.assert_error(function() error("the message") end)
			t.assert_match(tostring(err), "the message")
		end)
	`, nil)
	if reg.Results[0].Status != testx.StatusPass {
		t.Errorf("assert_error did not return the error value: %s", reg.Results[0].Message)
	}
}

func TestDeepEqualHandlesCycles(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("cyclic", function()
			local a = { n = 1 }; a.self = a
			local b = { n = 1 }; b.self = b
			t.assert_deep_eq(a, b)
		end)
	`, nil)
	if reg.Results[0].Status != testx.StatusPass {
		t.Errorf("cyclic deep_eq failed: %s", reg.Results[0].Message)
	}
}

func TestDeepEqualRespectsEq(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		local mt = { __eq = function(a, b) return a.id == b.id end }
		t.test("equal by id", function()
			local a = setmetatable({ id = 1, note = "x" }, mt)
			local b = setmetatable({ id = 1, note = "y" }, mt)
			t.assert_deep_eq(a, b)
		end)
		t.test("unequal by id", function()
			local a = setmetatable({ id = 1 }, mt)
			local b = setmetatable({ id = 2 }, mt)
			t.assert_deep_eq(a, b)
		end)
	`, nil)
	wantResults(t, reg, "pass equal by id", "fail unequal by id")
}

func TestFailureMessagePreviewsTables(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("x", function() t.assert_deep_eq({ 1, 2, name = "a" }, { 1, 3, name = "b" }) end)
	`, nil)
	msg := reg.Results[0].Message
	if !strings.Contains(msg, `{1, 3, name = "b"}`) || !strings.Contains(msg, `{1, 2, name = "a"}`) {
		t.Errorf("message = %q, want structural previews of both tables", msg)
	}
}

func TestUserMessageIsIncluded(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.test("x", function() t.assert_eq(1, 2, "counter should have advanced") end)
	`, nil)
	if !strings.Contains(reg.Results[0].Message, "counter should have advanced") {
		t.Errorf("message = %q, want the caller's note", reg.Results[0].Message)
	}
}

func TestItIsAnAliasForTest(t *testing.T) {
	reg := runSuite(t, `
		local t = require("test")
		t.it("reads better this way", function() end)
	`, nil)
	wantResults(t, reg, "pass reads better this way")
}

func TestVersionIsExposed(t *testing.T) {
	src := `local t = require("test") ver = t.VERSION`
	chunks, _ := compiler.CompileToInstructions(src, parser.NormalMode)
	v := vm.New()
	testx.Install(v, testx.NewRegistry())
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v", err)
	}
	if got := v.Globals.Get("ver"); got != testx.Version {
		t.Errorf("test.VERSION = %v, want %v", got, testx.Version)
	}
}
