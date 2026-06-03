package json

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/vm"
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

	methods.Set("encode", &vm.GoFunc{Name: "json:encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.AnyArg("encode", 1, args) // Allow table or other types
		goValue := vmToJSONValue(value)

		jsonBytes, err := json.Marshal(goValue)
		if err != nil {
			panic(vm.Errorf("json.encode: %s", err.Error()))
		}

		return []vm.Value{string(jsonBytes)}
	}})

	methods.Set("decode", &vm.GoFunc{Name: "json:decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		str := vm.StringArg("decode", 1, args)

		var goValue any
		decoder := json.NewDecoder(strings.NewReader(str))
		decoder.UseNumber() // Important: preserve integers when possible

		if err := decoder.Decode(&goValue); err != nil {
			panic(vm.Errorf("json.decode: %s", err.Error()))
		}

		result := jsonToVMValue(goValue)
		return []vm.Value{result}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func jsonToVMValue(v any) vm.Value {
	switch x := v.(type) {
	case nil:
		return nil

	case bool:
		return x

	case json.Number:
		// Try integer first
		if i, err := x.Int64(); err == nil {
			return i
		}
		// Otherwise float
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String() // fallback

	case float64:
		return x

	case string:
		return x

	case []any: // JSON Array
		t := vm.NewTable(len(x), 0)
		for i, item := range x {
			t.Set(int64(i+1), jsonToVMValue(item)) // 1-based indexing
		}
		return t

	case map[string]any: // JSON Object
		t := vm.NewTable(0, len(x))
		for key, value := range x {
			t.Set(key, jsonToVMValue(value))
		}
		return t

	default:
		// Fallback
		return fmt.Sprintf("%v", x)
	}
}

func vmToJSONValue(v vm.Value) any {
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
		if isArrayTable(x) {
			// Encode as JSON array
			arr := make([]any, 0, x.Len())
			for i := int64(1); i <= x.Len(); i++ {
				arr = append(arr, vmToJSONValue(x.Get(i)))
			}
			return arr
		}

		// Encode as JSON object
		obj := make(map[string]any)
		var key vm.Value = nil
		for {
			var value vm.Value
			key, value = x.Next(key)
			if key == nil {
				break
			}
			if strKey, ok := key.(string); ok {
				obj[strKey] = vmToJSONValue(value)
			}
			// Non-string keys are ignored (standard JSON behavior)
		}
		return obj

	default:
		return fmt.Sprintf("%v", x) // fallback
	}
}

func isArrayTable(t *vm.Table) bool {
	n := t.Len()
	if n == 0 {
		return false
	}
	// Check that all integer keys from 1 to n exist
	for i := int64(1); i <= n; i++ {
		if t.Get(i) == nil {
			return false
		}
	}
	return true
}
