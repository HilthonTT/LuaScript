package plugin

import (
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/hilthontt/luascript/internal/vm"
)

var errType = reflect.TypeFor[error]()

var byteSliceType = reflect.TypeFor[[]byte]()

const goValueKey = "\x00govalue"

var (
	goValueMeta *vm.Table
	goValueOnce sync.Once
)

func wrapGo(x any) *vm.Table {
	goValueOnce.Do(buildGoValueMeta)
	t := vm.NewTable(0, 1)
	t.Set(goValueKey, x)
	t.SetMetatable(goValueMeta)
	return t
}

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

func goValueMember(self *vm.Table, obj any, key string) vm.Value {
	rv := reflect.ValueOf(obj)
	if !rv.IsValid() {
		return nil
	}

	if m := rv.MethodByName(key); m.IsValid() {
		return bindMethod(self, m, key)
	}

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

func paramType(ft reflect.Type, i int) reflect.Type {
	last := ft.NumIn() - 1
	if ft.IsVariadic() && i >= last {
		return ft.In(last).Elem()
	}
	return ft.In(i)
}

func toGo(v vm.Value, want reflect.Type) (reflect.Value, error) {
	if v == nil {
		return reflect.Zero(want), nil
	}

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

func fromGo(rv reflect.Value) vm.Value {
	if !rv.IsValid() {
		return nil
	}

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

	if !rv.CanInterface() {
		return nil
	}
	return wrapGo(rv.Interface())
}
