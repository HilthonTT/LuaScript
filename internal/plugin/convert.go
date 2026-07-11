package plugin

import (
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/hilthontt/luascript/internal/vm"
)

// Value marshalling between the VM and reflected Go code.
//
// This file deliberately does not import Go's `plugin` package, so it
// compiles — and its tests run — on every platform, including the ones where
// dynamic loading itself is unavailable.
//
// The direction that matters is Lua -> Go: conversion is driven by the
// *target* type taken from the reflected function signature, never by
// guessing from the Lua value. That is what lets a plain Lua integer satisfy
// a Go `int`, a `time.Duration`, or a `float64` parameter without the script
// having to know which.
//
// Go -> Lua obeys the FFI rule in CLAUDE.md: only nil, bool, int64, float64,
// string, *Table, *Closure and *GoFunc may enter the runtime. Every integer
// kind therefore widens to int64 and every float to float64. Anything with no
// Lua counterpart at all — a struct, a pointer, an interface such as *sql.DB —
// is wrapped as a GoValue (see below) rather than leaked in raw.

var errType = reflect.TypeFor[error]()

var byteSliceType = reflect.TypeFor[[]byte]()

// goValueKey is the private instance-table key holding the backing Go value.
// The control-byte prefix keeps it out of the way of any field a script would
// reasonably index — the same trick ndarray uses for its backing array.
const goValueKey = "\x00govalue"

var (
	goValueMeta *vm.Table
	goValueOnce sync.Once
)

// wrapGo exposes an arbitrary Go value to Lua as a table sharing one
// metatable, with the value itself riding under goValueKey. Exported methods
// and struct fields are resolved on demand by __index, so a *sql.DB handed
// back from a plugin stays callable:
//
//	local rows = db:Query("select 1")
func wrapGo(x any) *vm.Table {
	goValueOnce.Do(buildGoValueMeta)
	t := vm.NewTable(0, 1)
	t.Set(goValueKey, x)
	t.SetMetatable(goValueMeta)
	return t
}

// unwrapGo recovers the Go value behind a GoValue table.
func unwrapGo(v vm.Value) (any, bool) {
	t, ok := v.(*vm.Table)
	if !ok {
		return nil, false
	}
	raw := t.Get(goValueKey)
	if raw == nil {
		return nil, false
	}
	return raw, true
}

func buildGoValueMeta() {
	goValueMeta = vm.NewTable(0, 2)

	goValueMeta.Set("__index", &vm.GoFunc{Name: "govalue:__index", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 2 {
			return []vm.Value{nil}
		}
		self, _ := args[0].(*vm.Table)
		raw, ok := unwrapGo(self)
		if !ok {
			return []vm.Value{nil}
		}
		key, ok := args[1].(string)
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{goValueMember(self, raw, key)}
	}})

	goValueMeta.Set("__tostring", &vm.GoFunc{Name: "govalue:__tostring", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		raw, ok := unwrapGo(args[0])
		if !ok {
			return []vm.Value{"<govalue>"}
		}
		return []vm.Value{fmt.Sprintf("govalue<%T>(%v)", raw, raw)}
	}})
}

// goValueMember resolves obj.key: an exported method (returned as a bound
// callable) or an exported struct field (converted to a Lua value). Returns
// nil when the name matches neither, which surfaces to the script as the
// usual "attempt to call a nil value".
func goValueMember(self *vm.Table, obj any, key string) vm.Value {
	rv := reflect.ValueOf(obj)
	if !rv.IsValid() {
		return nil
	}

	// Methods are looked up on the value as handed to us. A method with a
	// pointer receiver is only in the method set of the pointer, which is
	// exactly what a plugin returning *sql.DB gives us, so no addressing
	// dance is needed here.
	if m := rv.MethodByName(key); m.IsValid() {
		return bindMethod(self, m, key)
	}

	// Exported fields, following one pointer hop.
	sv := rv
	if sv.Kind() == reflect.Ptr {
		if sv.IsNil() {
			return nil
		}
		sv = sv.Elem()
	}
	if sv.Kind() == reflect.Struct {
		if f := sv.FieldByName(key); f.IsValid() && f.CanInterface() {
			return fromGo(f)
		}
	}
	return nil
}

// bindMethod wraps a reflected method as a Lua callable. Lua's `obj:m(x)`
// desugars to `obj.m(obj, x)`, so the receiver arrives as the first argument
// and has to be dropped — but only when it really is this object, so that a
// dot-call (`obj.m(x)`) keeps working too.
func bindMethod(self *vm.Table, m reflect.Value, name string) *vm.GoFunc {
	return &vm.GoFunc{Name: "govalue:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) > 0 {
			if t, ok := args[0].(*vm.Table); ok && t == self {
				args = args[1:]
			}
		}
		return callReflected(m, "govalue:"+name, args)
	}}
}

// callReflected converts args to the function's parameter types, calls it, and
// converts the results back. Errors are raised as Lua errors, matching every
// other native module.
func callReflected(fn reflect.Value, name string, args []vm.Value) []vm.Value {
	ft := fn.Type()

	if ft.IsVariadic() {
		if len(args) < ft.NumIn()-1 {
			panic(vm.Errorf("%s: expected at least %d argument(s), got %d", name, ft.NumIn()-1, len(args)))
		}
	} else if len(args) != ft.NumIn() {
		panic(vm.Errorf("%s: expected %d argument(s), got %d", name, ft.NumIn(), len(args)))
	}

	in := make([]reflect.Value, len(args))
	for i, a := range args {
		want := paramType(ft, i)
		rv, err := toGo(a, want)
		if err != nil {
			panic(vm.Errorf("bad argument #%d to '%s' (%v)", i+1, name, err))
		}
		in[i] = rv
	}

	out := fn.Call(in)

	res := make([]vm.Value, len(out))
	for i, o := range out {
		// A returned error is the one Go type with a natural Lua spelling:
		// nil when there is no error, its message otherwise. It keeps its
		// position, so `local v, err = p.Open(...)` reads as it does in Lua.
		if o.Type() == errType {
			if o.IsNil() {
				res[i] = nil
			} else {
				res[i] = o.Interface().(error).Error()
			}
			continue
		}
		res[i] = fromGo(o)
	}
	return res
}

// paramType is the declared type of parameter i, accounting for the variadic
// tail (where every argument from NumIn()-1 on has the slice's element type).
func paramType(ft reflect.Type, i int) reflect.Type {
	last := ft.NumIn() - 1
	if ft.IsVariadic() && i >= last {
		return ft.In(last).Elem()
	}
	return ft.In(i)
}

// toGo converts a Lua value to a reflect.Value of type `want`.
func toGo(v vm.Value, want reflect.Type) (reflect.Value, error) {
	// nil fills in the zero value, which is the closest Go has to Lua's nil
	// for any parameter type (nil pointer, empty string, 0, ...).
	if v == nil {
		return reflect.Zero(want), nil
	}

	// A GoValue passing straight back into Go: hand over the value it wraps.
	if raw, ok := unwrapGo(v); ok {
		rv := reflect.ValueOf(raw)
		switch {
		case rv.Type().AssignableTo(want):
			return rv, nil
		case rv.Type().ConvertibleTo(want):
			return rv.Convert(want), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot use govalue<%s> as %s", rv.Type(), want)
	}

	// An empty interface parameter takes whatever we have, unconverted.
	if want.Kind() == reflect.Interface && want.NumMethod() == 0 {
		return reflect.ValueOf(plainGo(v)), nil
	}

	switch x := v.(type) {
	case bool:
		if want.Kind() == reflect.Bool {
			return reflect.ValueOf(x).Convert(want), nil
		}
	case string:
		if want.Kind() == reflect.String || want == byteSliceType {
			return reflect.ValueOf(x).Convert(want), nil
		}
	case int64:
		if rv, ok := numberToGo(float64(x), x, true, want); ok {
			return rv, nil
		}
	case float64:
		if rv, ok := numberToGo(x, int64(x), false, want); ok {
			return rv, nil
		}
	case *vm.Table:
		return tableToGo(x, want)
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", vm.TypeName(v), want)
}

// numberToGo converts a Lua number to any Go numeric type. A float with a
// fractional part is rejected for an integer parameter rather than silently
// truncated — Lua 5.4 raises "number has no integer representation" for the
// same mistake.
func numberToGo(f float64, i int64, isInt bool, want reflect.Type) (reflect.Value, bool) {
	switch want.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if !isInt {
			if f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) {
				return reflect.Value{}, false
			}
			i = int64(f)
		}
		rv := reflect.ValueOf(i)
		if !rv.Type().ConvertibleTo(want) {
			return reflect.Value{}, false
		}
		return rv.Convert(want), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(f).Convert(want), true
	}
	return reflect.Value{}, false
}

// tableToGo converts a Lua table to a Go slice or map. Slices are read from
// the table's 1-based array part; maps take every entry.
func tableToGo(t *vm.Table, want reflect.Type) (reflect.Value, error) {
	switch want.Kind() {
	case reflect.Slice:
		n := int(t.Len())
		out := reflect.MakeSlice(want, n, n)
		for i := range n {
			ev, err := toGo(t.Get(int64(i+1)), want.Elem())
			if err != nil {
				return reflect.Value{}, fmt.Errorf("element %d: %w", i+1, err)
			}
			out.Index(i).Set(ev)
		}
		return out, nil

	case reflect.Map:
		out := reflect.MakeMap(want)
		for k, v := t.Next(nil); k != nil; k, v = t.Next(k) {
			if ks, ok := k.(string); ok && ks == goValueKey {
				continue
			}
			kv, err := toGo(k, want.Key())
			if err != nil {
				return reflect.Value{}, fmt.Errorf("key %v: %w", k, err)
			}
			ev, err := toGo(v, want.Elem())
			if err != nil {
				return reflect.Value{}, fmt.Errorf("value at key %v: %w", k, err)
			}
			out.SetMapIndex(kv, ev)
		}
		return out, nil
	}
	return reflect.Value{}, fmt.Errorf("cannot convert table to %s", want)
}

// plainGo renders a Lua value as the Go value an interface{} parameter should
// receive. Tables become []any when they look like sequences and map[string]any
// otherwise — the same shape the json module produces.
func plainGo(v vm.Value) any {
	t, ok := v.(*vm.Table)
	if !ok {
		return v
	}
	if n := int(t.Len()); n > 0 {
		out := make([]any, n)
		for i := range n {
			out[i] = plainGo(t.Get(int64(i + 1)))
		}
		return out
	}
	out := map[string]any{}
	for k, val := t.Next(nil); k != nil; k, val = t.Next(k) {
		ks, ok := k.(string)
		if !ok || ks == goValueKey {
			continue
		}
		out[ks] = plainGo(val)
	}
	return out
}

// fromGo converts a reflected Go value into a runtime-tracked Lua value,
// wrapping anything without a Lua counterpart as a GoValue.
func fromGo(rv reflect.Value) vm.Value {
	if !rv.IsValid() {
		return nil
	}

	// Unwrap interface cells so we dispatch on the dynamic type.
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		if err, ok := rv.Interface().(error); ok {
			return err.Error()
		}
		return fromGo(rv.Elem())
	}

	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return rv.String()

	case reflect.Slice, reflect.Array:
		// []byte is a Lua string, not a table of numbers.
		if rv.Kind() == reflect.Slice && rv.Type() == byteSliceType {
			return string(rv.Bytes())
		}
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return nil
		}
		t := vm.NewTable(rv.Len(), 0)
		for i := range rv.Len() {
			t.Set(int64(i+1), fromGo(rv.Index(i)))
		}
		return t

	case reflect.Map:
		if rv.IsNil() {
			return nil
		}
		t := vm.NewTable(0, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			k := fromGo(iter.Key())
			// Only Lua-hashable keys can index a table; anything else
			// (a struct key, say) has nowhere to go.
			switch k.(type) {
			case string, int64, float64, bool:
				t.Set(k, fromGo(iter.Value()))
			}
		}
		return t

	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		return wrapGo(rv.Interface())
	}

	// Structs, funcs, channels, and anything else: keep the Go value alive
	// behind a GoValue so its methods stay reachable.
	if !rv.CanInterface() {
		return nil
	}
	return wrapGo(rv.Interface())
}
