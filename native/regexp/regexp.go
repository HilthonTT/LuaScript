package regexp

import (
	"regexp"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterRegexpPreload installs the `regexp` module under package.preload.
func RegisterRegexpPreload(v *vm.VM) {
	pkg, ok := v.Globals.Get("package").(*vm.Table)
	if !ok {
		return
	}
	preload, ok := pkg.Get("preload").(*vm.Table)
	if !ok {
		preload = vm.NewTable(0, 4)
		pkg.Set("preload", preload)
	}
	preload.Set("regexp", &vm.GoFunc{Name: "preload.regexp", Fn: regexpLoader})
}

func regexpLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newRegexp()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newRegexp() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 2)

	// regexp.compile(pattern) -> regex object. An invalid pattern raises.
	methods.Set("compile", &vm.GoFunc{Name: "regexp:compile", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		pattern := vm.StringArg("regexp.compile", 1, args)
		re, err := regexp.Compile(pattern)
		if err != nil {
			panic(vm.Errorf("regexp.compile: %s", err.Error()))
		}
		return []vm.Value{newRegex(re)}
	}})

	// regexp.quote(s) -> a pattern that matches s literally.
	methods.Set("quote", &vm.GoFunc{Name: "regexp:quote", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("regexp.quote", 1, args)
		return []vm.Value{regexp.QuoteMeta(s)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// newRegex wraps a compiled *regexp.Regexp in a stateful object table.
// The *regexp.Regexp is captured in the method closures so the raw handle
// never leaks into script space.
func newRegex(re *regexp.Regexp) *vm.Table {
	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 6)

	// re:test(s) -> bool.
	methods.Set("test", &vm.GoFunc{Name: "regex:test", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:test", 1, a)
		s := vm.StringArg("regex:test", 2, a)
		return []vm.Value{re.MatchString(s)}
	}})

	// re:capture(s) -> first match, then any capture groups as extra
	// return values. Returns nil when there is no match. (Named
	// `capture` rather than `match` because `match` is a reserved
	// keyword in sakura — see the match statement.)
	methods.Set("capture", &vm.GoFunc{Name: "regex:capture", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:capture", 1, a)
		s := vm.StringArg("regex:capture", 2, a)
		sub := re.FindStringSubmatch(s)
		if sub == nil {
			return []vm.Value{nil}
		}
		out := make([]vm.Value, len(sub))
		for i, g := range sub {
			out[i] = g
		}
		return out
	}})

	// re:find_all(s) -> array table of every match string.
	methods.Set("find_all", &vm.GoFunc{Name: "regex:find_all", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:find_all", 1, a)
		s := vm.StringArg("regex:find_all", 2, a)
		all := re.FindAllString(s, -1)
		t := vm.NewTable(len(all), 0)
		for _, g := range all {
			t.Append(g)
		}
		return []vm.Value{t}
	}})

	// re:replace(s, repl) -> s with every match replaced by repl.
	// repl may use $1 / ${name} expansion (Go's ReplaceAllString rules).
	methods.Set("replace", &vm.GoFunc{Name: "regex:replace", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:replace", 1, a)
		s := vm.StringArg("regex:replace", 2, a)
		repl := vm.StringArg("regex:replace", 3, a)
		return []vm.Value{re.ReplaceAllString(s, repl)}
	}})

	// re:split(s) -> array table of the substrings between matches.
	methods.Set("split", &vm.GoFunc{Name: "regex:split", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:split", 1, a)
		s := vm.StringArg("regex:split", 2, a)
		parts := re.Split(s, -1)
		t := vm.NewTable(len(parts), 0)
		for _, p := range parts {
			t.Append(p)
		}
		return []vm.Value{t}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	o.SetMetatable(mt)
	return o
}
