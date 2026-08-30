package vm

// The baseline `table` library.

import (
	"sort"
	"strings"
)

// table

func buildTableLibrary() *Table {
	t := NewTable(0, 8)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "table." + name, Fn: fn})
	}

	add("insert", func(_ *VM, args []Value) []Value {
		tbl := TableArg("table.insert", 1, args)
		switch len(args) {
		case 1:
			panic(LuaError("bad argument to 'table.insert' (value expected)"))
		case 2:
			// Append at the end.
			tbl.Set(tbl.Len()+1, args[1])
		case 3:
			pos := IntArg("table.insert", 2, args)
			val := args[2]
			n := tbl.Len()
			// Lua 5.4: the position must be in [1, n+1].
			if pos < 1 || pos > n+1 {
				panic(LuaError("bad argument #2 to 'table.insert' (position out of bounds)"))
			}
			// Shift right to make room for the new element.
			for i := n; i >= pos; i-- {
				tbl.Set(i+1, tbl.Get(i))
			}
			tbl.Set(pos, val)
		default:
			panic(LuaError("wrong number of arguments to 'table.insert'"))
		}
		return nil
	})
	add("remove", func(_ *VM, args []Value) []Value {
		tbl := TableArg("table.remove", 1, args)
		n := tbl.Len()
		if n == 0 {
			return []Value{nil}
		}
		pos := OptInt("table.remove", 2, args, n)
		// Lua 5.4 validates pos unless it equals the default (n); an out-of-range
		// pos must error, not silently drive a multi-billion-iteration shift loop.
		if pos != n && (pos < 1 || pos > n+1) {
			panic(LuaError("bad argument #2 to 'table.remove' (position out of bounds)"))
		}
		removed := tbl.Get(pos)
		for i := pos; i < n; i++ {
			tbl.Set(i, tbl.Get(i+1))
		}
		if pos <= n {
			tbl.Set(n, nil)
		}
		return []Value{removed}
	})
	add("concat", func(_ *VM, args []Value) []Value {
		tbl := TableArg("table.concat", 1, args)
		sep := OptString("table.concat", 2, args, "")
		lo := OptInt("table.concat", 3, args, 1)
		hi := OptInt("table.concat", 4, args, tbl.Len())
		// Same wide-span guard as unpack/move: the caller picks both bounds, so
		// without it table.concat(t, "", 1, math.maxinteger) spins the VM for
		// the rest of the process's life. uint64 subtraction is exact here
		// because the loop below only runs when hi >= lo.
		if hi >= lo && uint64(hi)-uint64(lo) >= 1<<24 {
			panic(LuaError("too many elements to concat"))
		}
		var b strings.Builder
		for i := lo; i <= hi; i++ {
			if i > lo {
				b.WriteString(sep)
			}
			el := tbl.Get(i)
			switch e := el.(type) {
			case string:
				b.WriteString(e)
			case int64, float64:
				b.WriteString(ToString(el))
			default:
				// Reference Lua errors here; silently rendering "nil" or
				// "table: 0x…" into the result masks caller bugs.
				panic(Errorf("invalid value (%s) at index %d in table for 'table.concat'", TypeName(el), i))
			}
		}
		return []Value{b.String()}
	})
	add("sort", func(v *VM, args []Value) []Value {
		tbl := TableArg("table.sort", 1, args)
		var less func(a, b Value) bool
		if len(args) >= 2 && args[1] != nil {
			cmp := args[1]
			if _, ok := cmp.(*Closure); !ok {
				if _, ok := cmp.(*GoFunc); !ok {
					panic(Errorf("bad argument #2 to 'table.sort' (function expected, got %s)", TypeName(cmp)))
				}
			}
			less = func(a, b Value) bool {
				rs := v.CallValue(cmp, []Value{a, b}, 1)
				return len(rs) > 0 && IsTruthy(rs[0])
			}
		} else {
			less = v.lessMM
		}
		n := tbl.Len()
		elems := make([]Value, n)
		for i := int64(0); i < n; i++ {
			elems[i] = tbl.Get(i + 1)
		}
		sort.SliceStable(elems, func(i, j int) bool { return less(elems[i], elems[j]) })
		for i, e := range elems {
			tbl.Set(int64(i)+1, e)
		}
		return nil
	})
	add("move", func(_ *VM, args []Value) []Value {
		a1 := TableArg("table.move", 1, args)
		f := IntArg("table.move", 2, args)
		e := IntArg("table.move", 3, args)
		d := IntArg("table.move", 4, args)
		a2 := a1
		if len(args) >= 5 && args[4] != nil {
			a2 = TableArg("table.move", 5, args)
		}
		if e >= f {
			// Same wide-span overflow guard as unpack: hi-lo+1 can wrap.
			if uint64(e)-uint64(f) >= 1<<24 {
				panic(LuaError("too many elements to move"))
			}
			if d > e || d <= f || a1 != a2 {
				for i := int64(0); i <= e-f; i++ {
					a2.Set(d+i, a1.Get(f+i))
				}
			} else {
				// Overlapping shift within one table: copy back-to-front
				// so sources aren't overwritten before they're read.
				for i := e - f; i >= 0; i-- {
					a2.Set(d+i, a1.Get(f+i))
				}
			}
		}
		return []Value{a2}
	})
	add("unpack", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return nil
		}
		tbl := TableArg("table.unpack", 1, args)
		lo := OptInt("table.unpack", 2, args, 1)
		hi := OptInt("table.unpack", 3, args, tbl.Len())
		if hi < lo {
			return nil
		}
		// The element count is hi-lo+1, but that expression overflows int64 for a
		// wide (lo,hi) span (e.g. mininteger..maxinteger), wrapping negative/small
		// and bypassing the guard while the loop counter itself wraps and never
		// terminates. Compute the span in uint64, which is exact here because
		// hi>=lo: uint64(hi)-uint64(lo) == the true non-negative difference.
		if uint64(hi)-uint64(lo) >= 1<<24 {
			panic(LuaError("too many results to unpack"))
		}
		out := make([]Value, 0, hi-lo+1)
		for i := lo; i <= hi; i++ {
			out = append(out, tbl.Get(i))
		}
		return out
	})
	add("pack", func(_ *VM, args []Value) []Value {
		tbl := NewTable(len(args), 1)
		for i, a := range args {
			tbl.Set(int64(i+1), a)
		}
		tbl.Set("n", int64(len(args)))
		return []Value{tbl}
	})
	return t
}
