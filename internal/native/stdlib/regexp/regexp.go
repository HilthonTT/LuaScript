package regexp

import (
	"regexp"
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

func submatchValues(s string, idx []int) []vm.Value {
	out := make([]vm.Value, 0, len(idx)/2)
	for i := 0; 2*i+1 < len(idx); i++ {
		if idx[2*i] < 0 {
			out = append(out, nil)
			continue
		}
		out = append(out, s[idx[2*i]:idx[2*i+1]])
	}
	return out
}

func luaInit(s string, init int64) int {
	n := int64(len(s))
	if init < 0 {
		init = n + init + 1
	}
	if init < 1 {
		init = 1
	}
	if init > n+1 {
		return len(s) + 1
	}
	return int(init - 1)
}

func RegisterRegexpPreload(v *vm.VM) {
	vm.RegisterPreload(v, "regexp", regexpLoader)
}

func regexpLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newRegexp()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newRegexp() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 2)

	methods.Set("compile", &vm.GoFunc{Name: "regexp:compile", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		pattern := vm.StringArg("regexp.compile", 1, args)
		re, err := regexp.Compile(pattern)
		if err != nil {
			panic(vm.Errorf("regexp.compile: %s", err.Error()))
		}
		return []vm.Value{newRegex(re)}
	}})

	methods.Set("quote", &vm.GoFunc{Name: "regexp:quote", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("regexp.quote", 1, args)
		return []vm.Value{regexp.QuoteMeta(s)}
	}})

	methods.Set("is_valid", &vm.GoFunc{Name: "regexp:is_valid", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		pattern := vm.StringArg("regexp.is_valid", 1, args)
		_, err := regexp.Compile(pattern)
		return []vm.Value{err == nil}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func newRegex(re *regexp.Regexp) *vm.Table {
	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 6)

	methods.Set("test", &vm.GoFunc{Name: "regex:test", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:test", 1, a)
		s := vm.StringArg("regex:test", 2, a)
		return []vm.Value{re.MatchString(s)}
	}})

	methods.Set("capture", &vm.GoFunc{Name: "regex:capture", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:capture", 1, a)
		s := vm.StringArg("regex:capture", 2, a)
		idx := re.FindStringSubmatchIndex(s)
		if idx == nil {
			return []vm.Value{nil}
		}
		return submatchValues(s, idx)
	}})

	methods.Set("find", &vm.GoFunc{Name: "regex:find", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:find", 1, a)
		s := vm.StringArg("regex:find", 2, a)
		init := luaInit(s, vm.OptInt("regex:find", 3, a, 1))
		if init > len(s) {
			return []vm.Value{nil}
		}
		idx := re.FindStringSubmatchIndex(s[init:])
		if idx == nil {
			return []vm.Value{nil}
		}
		caps := submatchValues(s[init:], idx)
		out := make([]vm.Value, 0, len(caps)+1)
		out = append(out, int64(init+idx[0]+1), int64(init+idx[1]))
		out = append(out, caps[1:]...)
		return out
	}})

	methods.Set("groups", &vm.GoFunc{Name: "regex:groups", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:groups", 1, a)
		s := vm.StringArg("regex:groups", 2, a)
		idx := re.FindStringSubmatchIndex(s)
		if idx == nil {
			return []vm.Value{nil}
		}
		names := re.SubexpNames()
		t := vm.NewTable(0, len(names))
		for i, name := range names {
			if name == "" || 2*i+1 >= len(idx) || idx[2*i] < 0 {
				continue
			}
			t.Set(name, s[idx[2*i]:idx[2*i+1]])
		}
		return []vm.Value{t}
	}})

	methods.Set("find_all_captures", &vm.GoFunc{Name: "regex:find_all_captures", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:find_all_captures", 1, a)
		s := vm.StringArg("regex:find_all_captures", 2, a)
		limit := int(vm.OptInt("regex:find_all_captures", 3, a, -1))
		all := re.FindAllStringSubmatchIndex(s, limit)
		out := vm.NewTable(len(all), 0)
		for _, idx := range all {
			vals := submatchValues(s, idx)
			row := vm.NewTable(len(vals), 0)
			for _, v := range vals {
				row.Append(v)
			}
			out.Append(row)
		}
		return []vm.Value{out}
	}})

	methods.Set("replace_func", &vm.GoFunc{Name: "regex:replace_func", Fn: func(v *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:replace_func", 1, a)
		s := vm.StringArg("regex:replace_func", 2, a)
		if len(a) < 3 || a[2] == nil {
			panic(vm.Errorf("bad argument #3 to 'regex:replace_func' (function expected)"))
		}
		fn := a[2]
		var b strings.Builder
		last := 0
		for _, idx := range re.FindAllStringSubmatchIndex(s, -1) {
			b.WriteString(s[last:idx[0]])
			res := v.CallValue(fn, submatchValues(s, idx), 1)
			if len(res) > 0 && res[0] != nil {
				b.WriteString(vm.ToString(res[0]))
			}
			last = idx[1]
		}
		b.WriteString(s[last:])
		return []vm.Value{b.String()}
	}})

	methods.Set("find_all", &vm.GoFunc{Name: "regex:find_all", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:find_all", 1, a)
		s := vm.StringArg("regex:find_all", 2, a)
		all := re.FindAllString(s, int(vm.OptInt("regex:find_all", 3, a, -1)))
		t := vm.NewTable(len(all), 0)
		for _, g := range all {
			t.Append(g)
		}
		return []vm.Value{t}
	}})

	methods.Set("replace", &vm.GoFunc{Name: "regex:replace", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("regex:replace", 1, a)
		s := vm.StringArg("regex:replace", 2, a)
		repl := vm.StringArg("regex:replace", 3, a)
		return []vm.Value{re.ReplaceAllString(s, repl)}
	}})

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
