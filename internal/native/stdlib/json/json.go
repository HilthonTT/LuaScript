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

	// json.null is a sentinel standing for a JSON null.
	//
	// Decoding null to Lua nil loses information: nil is indistinguishable
	// from an absent key, and inside an array it truncates everything after
	// it, so [1, null, 3] decodes to a table of length 1. Passing
	// `{ null = json.null }` to decode keeps those nulls as this value
	// instead.
	//
	// Opt-in rather than default: nil is what decode has always produced, and
	// silently changing it would break every `if t.field == nil` already
	// written against this module. Encoding json.null always produces a null,
	// which is safe in either mode.
	//
	// An empty table with this identity is used rather than a string or number
	// so it can never collide with real data.
	nullSentinel := vm.NewTable(0, 0)
	m.Set("null", nullSentinel)

	// json.empty_array marks a table that must encode as [] rather than {}.
	// An empty Lua table is ambiguous — it is both an empty array and an empty
	// object — and encode has to pick one, so a caller who needs the other has
	// no way to say so without this.
	emptyArrayMarker := vm.NewTable(0, 0)
	m.Set("empty_array", emptyArrayMarker)

	// nullOut is what decode substitutes for a JSON null; nil unless the
	// caller opts in per call. The encoder used for encoding always knows the
	// sentinel so json.null round-trips regardless.
	enc := &encoder{null: nullSentinel, emptyArray: emptyArrayMarker}

	methods.Set("encode", &vm.GoFunc{Name: "json:encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.AnyArg("encode", 1, args) // Allow table or other types
		goValue := enc.toJSON(value, 0)

		// An options table may request indented output. Compact stays the
		// default: it is what goes over the wire.
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
		decoder.UseNumber() // Important: preserve integers when possible

		if err := decoder.Decode(&goValue); err != nil {
			panic(vm.Errorf("json.decode: %s", err.Error()))
		}
		// Decode stops at the end of the first complete value, so without this
		// check `{"a":1} <html>` would parse as {a=1} and the trailing bytes
		// would vanish — exactly the case where a caller most needs to be told
		// the payload was not the JSON document they expected.
		if _, err := decoder.Token(); err != io.EOF {
			panic(vm.Errorf("json.decode: trailing content after JSON value"))
		}

		// opts.null selects what a JSON null becomes. Absent, it stays nil —
		// the behavior this module has always had.
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

// encoder carries the two sentinel identities across a conversion so both
// directions agree about what a JSON null and a forced empty array are.
type encoder struct {
	null       *vm.Table
	emptyArray *vm.Table
}

// clampIndent bounds a numeric indent request. A caller-chosen width multiplies
// the output size, so an absurd value is capped rather than allowed to turn a
// small document into a huge one.
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
		// e.null is nil unless the caller asked for a sentinel, in which case
		// nulls survive as a distinguishable value instead of vanishing.
		if e.null == nil {
			return nil
		}
		return e.null

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
			t.Set(int64(i+1), e.fromJSON(item)) // 1-based indexing
		}
		return t

	case map[string]any: // JSON Object
		t := vm.NewTable(0, len(x))
		for key, value := range x {
			t.Set(key, e.fromJSON(value))
		}
		return t

	default:
		// encoding/json with UseNumber only ever produces the cases above, so
		// this is unreachable; raise rather than inventing a value if the
		// decoder ever grows a new one.
		panic(vm.Errorf("json.decode: unsupported JSON value of Go type %T", x))
	}
}

// maxJSONDepth bounds encode recursion so a cyclic table raises a catchable
// Lua error instead of overflowing the Go stack (a fatal, uncatchable crash).
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
		// Sentinels are matched by identity, before any structural test: both
		// are empty tables, so shape alone cannot tell them from real data.
		if x == e.null {
			return nil
		}
		if x == e.emptyArray {
			return []any{}
		}
		if isArrayTable(x) {
			// Encode as JSON array
			arr := make([]any, 0, x.Len())
			for i := int64(1); i <= x.Len(); i++ {
				arr = append(arr, e.toJSON(x.Get(i), depth+1))
			}
			return arr
		}

		// Encode as JSON object. JSON keys must be strings, so integer/float
		// keys are stringified (e.g. {[2]=10} -> {"2":10}) instead of being
		// silently dropped. Only bool/table keys, which have no sensible key
		// form, are skipped.
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
		// Functions, coroutines and host userdata have no JSON form. The old
		// fallback rendered them with %v, so a stray function silently became
		// the string "0xc000123456" inside otherwise-valid output — a bug that
		// only surfaces downstream, in whoever consumes the document.
		panic(vm.Errorf("json.encode: cannot encode a %s value", vm.TypeName(v)))
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
	// A mixed table ({1,2,3, name="x"}) must NOT encode as an array — that
	// would silently drop the hash keys. Count the total entries: exactly n
	// means a pure 1..n array.
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
