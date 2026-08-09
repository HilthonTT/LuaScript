package testx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

// Assertions raise vm.LuaError rather than returning a boolean. The VM stamps
// a LuaError raised from a builtin with the position of the innermost Lua
// frame — the line of the assertion call itself — so a failure message arrives
// already reading "suite_test.lsc:14: assert_eq failed", with no cooperation
// needed from the runner.

// failf raises an assertion failure. The message is pre-formatted and handed
// to Errorf as a literal so a value containing a percent sign cannot corrupt
// the output.
func failf(format string, args ...any) {
	panic(vm.Errorf("%s", fmt.Sprintf(format, args...)))
}

// detail formats an assertion failure as a headline plus indented expectation
// lines, which is what makes a diff readable in terminal output.
func detail(head, msg string, lines ...string) string {
	var b strings.Builder
	b.WriteString(head)
	if msg != "" {
		b.WriteString(": ")
		b.WriteString(msg)
	}
	for _, l := range lines {
		b.WriteString("\n  ")
		b.WriteString(l)
	}
	return b.String()
}

// optMsg reads the optional trailing message argument at 1-based position n.
func optMsg(args []vm.Value, n int) string {
	if len(args) < n || args[n-1] == nil {
		return ""
	}
	return vm.ToString(args[n-1])
}

// registerAssertions installs the assertion surface. set is the module's
// member installer, passed in so this file need not know how the table is
// built.
func registerAssertions(set func(string, func(*vm.VM, []vm.Value) []vm.Value)) {
	// assert_eq(got, want [, msg]) — `==` semantics, __eq included.
	set("assert_eq", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_eq", 1, args)
		want := vm.AnyArg("test.assert_eq", 2, args)
		if !v.EqualMM(got, want) {
			failf("%s", detail("assert_eq failed", optMsg(args, 3),
				"expected: "+render(v, want),
				"actual:   "+render(v, got)))
		}
		return nil
	})

	// assert_ne(got, unwanted [, msg])
	set("assert_ne", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_ne", 1, args)
		unwanted := vm.AnyArg("test.assert_ne", 2, args)
		if v.EqualMM(got, unwanted) {
			failf("%s", detail("assert_ne failed", optMsg(args, 3),
				"both values are "+render(v, got)))
		}
		return nil
	})

	// assert_deep_eq(got, want [, msg]) — structural table comparison.
	set("assert_deep_eq", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_deep_eq", 1, args)
		want := vm.AnyArg("test.assert_deep_eq", 2, args)
		if !deepEqual(v, got, want, map[pair]bool{}) {
			failf("%s", detail("assert_deep_eq failed", optMsg(args, 3),
				"expected: "+render(v, want),
				"actual:   "+render(v, got)))
		}
		return nil
	})

	// assert_true / assert_false use Lua truthiness: only nil and false are
	// false, so assert_true(0) passes exactly as `if 0 then` would.
	set("assert_true", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_true", 1, args)
		if !vm.IsTruthy(got) {
			failf("%s", detail("assert_true failed", optMsg(args, 2),
				"value: "+render(v, got)))
		}
		return nil
	})
	set("assert_false", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_false", 1, args)
		if vm.IsTruthy(got) {
			failf("%s", detail("assert_false failed", optMsg(args, 2),
				"value: "+render(v, got)))
		}
		return nil
	})

	set("assert_nil", func(v *vm.VM, args []vm.Value) []vm.Value {
		if len(args) > 0 && args[0] != nil {
			failf("%s", detail("assert_nil failed", optMsg(args, 2),
				"value: "+render(v, args[0])))
		}
		return nil
	})
	set("assert_not_nil", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 || args[0] == nil {
			failf("%s", detail("assert_not_nil failed", optMsg(args, 2),
				"value is nil"))
		}
		return nil
	})

	// assert_near(got, want [, eps] [, msg]) — float comparison with an
	// absolute tolerance. The third argument doubles as the message when it
	// is a string, so the common two-and-a-message form needs no placeholder.
	set("assert_near", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.FloatArg("test.assert_near", 1, args)
		want := vm.FloatArg("test.assert_near", 2, args)
		eps, msg := 1e-9, ""
		if len(args) >= 3 {
			if s, ok := args[2].(string); ok {
				msg = s
			} else {
				eps = vm.FloatArg("test.assert_near", 3, args)
				msg = optMsg(args, 4)
			}
		}
		if diff := abs(got - want); diff > eps {
			failf("%s", detail("assert_near failed", msg,
				fmt.Sprintf("expected: %v (±%v)", want, eps),
				fmt.Sprintf("actual:   %v", got),
				fmt.Sprintf("diff:     %v", diff)))
		}
		return nil
	})

	// assert_type(value, typename [, msg]) — typename as `type()` reports it.
	set("assert_type", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_type", 1, args)
		want := vm.StringArg("test.assert_type", 2, args)
		if actual := vm.TypeName(got); actual != want {
			failf("%s", detail("assert_type failed", optMsg(args, 3),
				"expected type: "+want,
				"actual type:   "+actual,
				"value:         "+render(v, got)))
		}
		return nil
	})

	// assert_len(value, n [, msg]) — length as `#` computes it.
	set("assert_len", func(v *vm.VM, args []vm.Value) []vm.Value {
		got := vm.AnyArg("test.assert_len", 1, args)
		want := vm.IntArg("test.assert_len", 2, args)
		var actual int64
		switch x := got.(type) {
		case string:
			actual = int64(len(x))
		case *vm.Table:
			actual = x.Len()
		default:
			panic(vm.Errorf("bad argument #1 to 'test.assert_len' (string or table expected, got %s)", vm.TypeName(got)))
		}
		if actual != want {
			failf("%s", detail("assert_len failed", optMsg(args, 3),
				fmt.Sprintf("expected length: %d", want),
				fmt.Sprintf("actual length:   %d", actual),
				"value: "+render(v, got)))
		}
		return nil
	})

	// assert_contains(haystack, needle [, msg]) — substring for strings,
	// membership for tables (array part and hash values alike).
	set("assert_contains", func(v *vm.VM, args []vm.Value) []vm.Value {
		hay := vm.AnyArg("test.assert_contains", 1, args)
		needle := vm.AnyArg("test.assert_contains", 2, args)
		switch h := hay.(type) {
		case string:
			n, ok := needle.(string)
			if !ok {
				panic(vm.Errorf("bad argument #2 to 'test.assert_contains' (string expected, got %s)", vm.TypeName(needle)))
			}
			if !strings.Contains(h, n) {
				failf("%s", detail("assert_contains failed", optMsg(args, 3),
					"haystack: "+strconv.Quote(h),
					"needle:   "+strconv.Quote(n)))
			}
		case *vm.Table:
			if !tableContains(v, h, needle) {
				failf("%s", detail("assert_contains failed", optMsg(args, 3),
					"table:  "+render(v, h),
					"needle: "+render(v, needle)))
			}
		default:
			panic(vm.Errorf("bad argument #1 to 'test.assert_contains' (string or table expected, got %s)", vm.TypeName(hay)))
		}
		return nil
	})

	// assert_match(s, pattern [, msg]) — Lua pattern, same engine as
	// string.find.
	set("assert_match", func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("test.assert_match", 1, args)
		pat := vm.StringArg("test.assert_match", 2, args)
		if !patternMatches(s, pat) {
			failf("%s", detail("assert_match failed", optMsg(args, 3),
				"pattern: "+strconv.Quote(pat),
				"subject: "+strconv.Quote(s)))
		}
		return nil
	})

	// assert_error(fn [, pattern] [, msg]) — expects fn to raise, and
	// returns the error value so a test can inspect it further.
	set("assert_error", func(v *vm.VM, args []vm.Value) []vm.Value {
		fn := vm.AnyArg("test.assert_error", 1, args)
		pat, msg := "", ""
		if len(args) >= 2 && args[1] != nil {
			pat = vm.StringArg("test.assert_error", 2, args)
			msg = optMsg(args, 3)
		}
		_, errVal, failed := v.SafeCall(fn, nil)
		if !failed {
			failf("%s", detail("assert_error failed", msg,
				"the function returned normally; an error was expected"))
		}
		if pat != "" {
			text := vm.ToStringMM(v, errVal)
			if !patternMatches(text, pat) {
				failf("%s", detail("assert_error failed", msg,
					"pattern: "+strconv.Quote(pat),
					"error:   "+strconv.Quote(text)))
			}
		}
		return []vm.Value{errVal}
	})

	// assert_no_error(fn [, msg]) — the inverse; forwards fn's results.
	set("assert_no_error", func(v *vm.VM, args []vm.Value) []vm.Value {
		fn := vm.AnyArg("test.assert_no_error", 1, args)
		res, errVal, failed := v.SafeCall(fn, nil)
		if failed {
			failf("%s", detail("assert_no_error failed", optMsg(args, 2),
				"error: "+vm.ToStringMM(v, errVal)))
		}
		return res
	})

	// fail([msg]) — unconditional failure, for a branch that should be
	// unreachable.
	set("fail", func(_ *vm.VM, args []vm.Value) []vm.Value {
		msg := optMsg(args, 1)
		if msg == "" {
			msg = "test failed"
		}
		failf("%s", msg)
		return nil
	})
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// patternMatches reports whether pat matches anywhere in s, using the same
// pattern engine string.find uses.
func patternMatches(s, pat string) bool {
	_, _, _, ok := vm.PatternFind(s, pat, 1)
	return ok
}

// tableContains reports whether any value in t equals needle, scanning the
// array part and then the hash part.
func tableContains(v *vm.VM, t *vm.Table, needle vm.Value) bool {
	var key vm.Value
	for {
		k, val := t.Next(key)
		if k == nil {
			return false
		}
		if v.EqualMM(val, needle) {
			return true
		}
		key = k
	}
}

// pair identifies a table comparison already in progress, so a cyclic
// structure terminates instead of recursing forever.
type pair struct{ a, b *vm.Table }

// deepEqual compares structurally: equal under `==` short-circuits, two tables
// compare key-by-key, everything else is unequal.
//
// A table carrying an __eq metamethod is compared with it rather than
// structurally — the script has said what equality means for that type, and
// overruling it would make deep comparison disagree with `==`.
func deepEqual(v *vm.VM, a, b vm.Value, seen map[pair]bool) bool {
	if v.EqualMM(a, b) {
		return true
	}
	ta, aok := a.(*vm.Table)
	tb, bok := b.(*vm.Table)
	if !aok || !bok {
		return false
	}
	if hasMeta(ta, "__eq") || hasMeta(tb, "__eq") {
		return false // EqualMM already consulted it and said no.
	}
	p := pair{ta, tb}
	if seen[p] {
		return true // Already being compared further up; assume equal.
	}
	seen[p] = true
	defer delete(seen, p)

	if ta.EntryCount() != tb.EntryCount() {
		return false
	}
	var key vm.Value
	for {
		k, av := ta.Next(key)
		if k == nil {
			return true
		}
		if !deepEqual(v, av, tb.Get(k), seen) {
			return false
		}
		key = k
	}
}

// hasMeta reports whether t's metatable defines the named metamethod.
func hasMeta(t *vm.Table, name string) bool {
	mt := t.Metatable()
	return mt != nil && mt.Get(name) != nil
}

// render formats a value for a failure message. Strings are quoted so an
// expected "3" is distinguishable from an expected 3, and plain tables get a
// shallow structural preview rather than an address nobody can act on.
func render(v *vm.VM, val vm.Value) string {
	switch x := val.(type) {
	case nil, bool, int64, float64:
		return vm.ToString(val)
	case string:
		return strconv.Quote(x)
	case *vm.Table:
		if hasMeta(x, "__tostring") {
			return vm.ToStringMM(v, x)
		}
		return previewTable(v, x, 0)
	default:
		return vm.ToStringMM(v, val)
	}
}

// previewEntries caps how much of a table a failure message reproduces:
// enough to see what differs, not so much that the message buries the test.
const previewEntries = 8

// previewTable renders a table as a Lua-ish constructor, array part first.
func previewTable(v *vm.VM, t *vm.Table, depth int) string {
	if depth > 2 {
		return "{...}"
	}
	n := t.Len()
	parts := make([]string, 0, previewEntries+1)
	truncated := false

	for i := int64(1); i <= n; i++ {
		if len(parts) == previewEntries {
			truncated = true
			break
		}
		parts = append(parts, renderNested(v, t.Get(i), depth+1))
	}

	var key vm.Value
	for {
		k, val := t.Next(key)
		if k == nil {
			break
		}
		key = k
		// Skip the array part, already rendered positionally above.
		if i, ok := k.(int64); ok && i >= 1 && i <= n {
			continue
		}
		if len(parts) >= previewEntries {
			truncated = true
			break
		}
		parts = append(parts, renderKey(v, k)+" = "+renderNested(v, val, depth+1))
	}

	if truncated {
		parts = append(parts, "...")
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// renderNested is render one level down, so nested tables preview too.
func renderNested(v *vm.VM, val vm.Value, depth int) string {
	if t, ok := val.(*vm.Table); ok && !hasMeta(t, "__tostring") {
		return previewTable(v, t, depth)
	}
	return render(v, val)
}

// renderKey formats a table key in constructor position: an identifier-shaped
// string key is bare, anything else is bracketed.
func renderKey(v *vm.VM, k vm.Value) string {
	if s, ok := k.(string); ok && isIdentifier(s) {
		return s
	}
	return "[" + render(v, k) + "]"
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
