package json

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterJSONPreload(v *vm.VM) {
	vm.RegisterPreload(v, "json", jsonLoader)
}

func jsonLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newJson()
	mod.Set("VERSION", "0.1.0")

	return []vm.Value{mod}
}

func newJson() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 2)

	nullSentinel := vm.NewTable(0, 0)
	m.Set("null", nullSentinel)

	emptyArrayMarker := vm.NewTable(0, 0)
	m.Set("empty_array", emptyArrayMarker)

	enc := &encoder{null: nullSentinel, emptyArray: emptyArrayMarker}

	methods.Set("encode", &vm.GoFunc{Name: "json:encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.AnyArg("encode", 1, args)
		goValue := enc.toJSON(value, 0)

		indent := ""
		if len(args) >= 2 && args[1] != nil {
			if opts, ok := args[1].(*vm.Table); ok {
				switch n := opts.Get("indent").(type) {
				case int64:
					indent = strings.Repeat(" ", clampIndent(n))
				case string:
					indent = n
				}
			}
		}

		var jsonBytes []byte
		var err error
		if indent != "" {
			jsonBytes, err = json.MarshalIndent(goValue, "", indent)
		} else {
			jsonBytes, err = json.Marshal(goValue)
		}
		if err != nil {
			panic(vm.Errorf("json.encode: %s", err.Error()))
		}

		return []vm.Value{string(jsonBytes)}
	}})

	methods.Set("decode", &vm.GoFunc{Name: "json:decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		str := vm.StringArg("decode", 1, args)

		var goValue any
		decoder := json.NewDecoder(strings.NewReader(str))
		decoder.UseNumber()

		if err := decoder.Decode(&goValue); err != nil {
			panic(vm.Errorf("json.decode: %s", err.Error()))
		}
		if _, err := decoder.Token(); err != io.EOF {
			panic(vm.Errorf("json.decode: trailing content after JSON value"))
		}

		dec := &encoder{null: nil, emptyArray: emptyArrayMarker}
		if len(args) >= 2 && args[1] != nil {
			if opts, ok := args[1].(*vm.Table); ok {
				if n, ok := opts.Get("null").(*vm.Table); ok {
					dec.null = n
				}
			}
		}

		result := dec.fromJSON(goValue)
		return []vm.Value{result}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

type encoder struct {
	null       *vm.Table
	emptyArray *vm.Table
}

func clampIndent(n int64) int {
	if n < 0 {
		return 0
	}
	if n > 16 {
		return 16
	}
	return int(n)
}

func (e *encoder) fromJSON(v any) vm.Value {
	switch x := v.(type) {
	case nil:
		if e.null == nil {
			return nil
		}
		return e.null

	case bool:
		return x

	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()

	case float64:
		return x

	case string:
		return x

	case []any:
		t := vm.NewTable(len(x), 0)
		for i, item := range x {
			t.Set(int64(i+1), e.fromJSON(item))
		}
		return t

	case map[string]any:
		t := vm.NewTable(0, len(x))
		for key, value := range x {
			t.Set(key, e.fromJSON(value))
		}
		return t

	default:
		panic(vm.Errorf("json.decode: unsupported JSON value of Go type %T", x))
	}
}

const maxJSONDepth = 1000

func (e *encoder) toJSON(v vm.Value, depth int) any {
	if depth > maxJSONDepth {
		panic(vm.Errorf("json.encode: table nesting too deep (cyclic reference?)"))
	}
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int64:
		return x
	case float64:
		return x
	case string:
		return x

	case *vm.Table:
		if x == e.null {
			return nil
		}
		if x == e.emptyArray {
			return []any{}
		}
		if isArrayTable(x) {
			arr := make([]any, 0, x.Len())
			for i := int64(1); i <= x.Len(); i++ {
				arr = append(arr, e.toJSON(x.Get(i), depth+1))
			}
			return arr
		}

		obj := make(map[string]any)
		var key vm.Value = nil
		for {
			var value vm.Value
			key, value = x.Next(key)
			if key == nil {
				break
			}
			switch k := key.(type) {
			case string:
				obj[k] = e.toJSON(value, depth+1)
			case int64:
				obj[strconv.FormatInt(k, 10)] = e.toJSON(value, depth+1)
			case float64:
				obj[strconv.FormatFloat(k, 'g', -1, 64)] = e.toJSON(value, depth+1)
			}
		}
		return obj

	default:
		panic(vm.Errorf("json.encode: cannot encode a %s value", vm.TypeName(v)))
	}
}

func isArrayTable(t *vm.Table) bool {
	n := t.Len()
	if n == 0 {
		return false
	}
	for i := int64(1); i <= n; i++ {
		if t.Get(i) == nil {
			return false
		}
	}
	count := int64(0)
	var key vm.Value
	for {
		key, _ = t.Next(key)
		if key == nil {
			break
		}
		count++
		if count > n {
			return false
		}
	}
	return count == n
}
