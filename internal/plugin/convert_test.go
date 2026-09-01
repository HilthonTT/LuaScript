package plugin

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

func call(t *testing.T, fn any, args ...vm.Value) (res []vm.Value, luaErr string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			luaErr = vm.ToStringMM(nil, r)
		}
	}()
	res = callReflected(reflect.ValueOf(fn), "f", args)
	return
}

func tbl(vals ...vm.Value) *vm.Table {
	t := vm.NewTable(len(vals), 0)
	for i, v := range vals {
		t.Set(int64(i+1), v)
	}
	return t
}

func TestCallReflectedArgumentConversion(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		args []vm.Value
		want vm.Value
		err  string
	}{
		{
			name: "string",
			fn:   strings.ToUpper,
			args: []vm.Value{"hello"},
			want: "HELLO",
		},
		{
			name: "lua int -> go int",
			fn:   func(n int) int { return n * 2 },
			args: []vm.Value{int64(21)},
			want: int64(42),
		},
		{
			name: "lua int -> go float64",
			fn:   func(f float64) float64 { return f / 2 },
			args: []vm.Value{int64(5)},
			want: 2.5,
		},
		{
			name: "lua int -> named type (time.Duration)",
			fn:   func(d time.Duration) string { return d.String() },
			args: []vm.Value{int64(90 * time.Second)},
			want: "1m30s",
		},
		{
			name: "integral float -> go int",
			fn:   func(n int) int { return n },
			args: []vm.Value{3.0},
			want: int64(3),
		},
		{
			name: "fractional float -> go int is rejected",
			fn:   func(n int) int { return n },
			args: []vm.Value{3.5},
			err:  "cannot convert number to int",
		},
		{
			name: "table -> []string",
			fn:   func(xs []string) string { return strings.Join(xs, "-") },
			args: []vm.Value{tbl("a", "b", "c")},
			want: "a-b-c",
		},
		{
			name: "table -> map[string]int",
			fn: func(m map[string]int) int {
				return m["a"] + m["b"]
			},
			args: []vm.Value{func() *vm.Table {
				m := vm.NewTable(0, 2)
				m.Set("a", int64(1))
				m.Set("b", int64(2))
				return m
			}()},
			want: int64(3),
		},
		{
			name: "variadic",
			fn: func(xs ...int) int {
				sum := 0
				for _, x := range xs {
					sum += x
				}
				return sum
			},
			args: []vm.Value{int64(1), int64(2), int64(3)},
			want: int64(6),
		},
		{
			name: "variadic with no args",
			fn:   func(xs ...int) int { return len(xs) },
			args: nil,
			want: int64(0),
		},
		{
			name: "any parameter takes the value as-is",
			fn:   func(v any) string { return reflect.TypeOf(v).String() },
			args: []vm.Value{"x"},
			want: "string",
		},
		{
			name: "nil fills the zero value",
			fn:   func(s string) int { return len(s) },
			args: []vm.Value{nil},
			want: int64(0),
		},
		{
			name: "string -> []byte",
			fn:   func(b []byte) int { return len(b) },
			args: []vm.Value{"abcd"},
			want: int64(4),
		},
		{
			name: "wrong arity",
			fn:   strings.ToUpper,
			args: []vm.Value{"a", "b"},
			err:  "expected 1 argument(s), got 2",
		},
		{
			name: "wrong type",
			fn:   strings.ToUpper,
			args: []vm.Value{true},
			err:  "cannot convert boolean to string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, luaErr := call(t, tt.fn, tt.args...)
			if tt.err != "" {
				if !strings.Contains(luaErr, tt.err) {
					t.Fatalf("expected error containing %q, got %q", tt.err, luaErr)
				}
				return
			}
			if luaErr != "" {
				t.Fatalf("unexpected error: %s", luaErr)
			}
			if len(res) != 1 || res[0] != tt.want {
				t.Fatalf("got %#v, want %#v", res, tt.want)
			}
		})
	}
}

func TestFromGoWidensNumbers(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		want vm.Value
	}{
		{"int8", func() int8 { return -8 }, int64(-8)},
		{"int32", func() int32 { return 32 }, int64(32)},
		{"uint8", func() uint8 { return 255 }, int64(255)},
		{"uint64", func() uint64 { return 7 }, int64(7)},
		{"float32", func() float32 { return 0.5 }, 0.5},
		{"bool", func() bool { return true }, true},
		{"byte slice is a string", func() []byte { return []byte("hi") }, "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, luaErr := call(t, tt.fn)
			if luaErr != "" {
				t.Fatalf("unexpected error: %s", luaErr)
			}
			if res[0] != tt.want {
				t.Fatalf("got %#v (%T), want %#v (%T)", res[0], res[0], tt.want, tt.want)
			}
		})
	}
}

func TestFromGoSlice(t *testing.T) {
	res, luaErr := call(t, func() []string { return []string{"a", "b"} })
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}
	got, ok := res[0].(*vm.Table)
	if !ok {
		t.Fatalf("expected a table, got %T", res[0])
	}
	if got.Len() != 2 || got.Get(int64(1)) != "a" || got.Get(int64(2)) != "b" {
		t.Fatalf("bad table: len=%d [1]=%v [2]=%v", got.Len(), got.Get(int64(1)), got.Get(int64(2)))
	}
}

func TestErrorReturnMapsToNilOrMessage(t *testing.T) {
	fn := func(fail bool) (string, error) {
		if fail {
			return "", errors.New("boom")
		}
		return "ok", nil
	}

	res, luaErr := call(t, fn, false)
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}
	if len(res) != 2 || res[0] != "ok" || res[1] != nil {
		t.Fatalf("success case: got %#v, want [ok nil]", res)
	}

	res, luaErr = call(t, fn, true)
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}
	if len(res) != 2 || res[0] != "" || res[1] != "boom" {
		t.Fatalf("failure case: got %#v, want [\"\" boom]", res)
	}
}

type greeter struct {
	Name   string
	hidden int
}

func (g *greeter) Greet(punct string) string { return "hi " + g.Name + punct }

func TestGoValueMethodAndFieldAccess(t *testing.T) {
	res, luaErr := call(t, func() *greeter { return &greeter{Name: "ada", hidden: 1} })
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}
	self, ok := res[0].(*vm.Table)
	if !ok {
		t.Fatalf("expected a GoValue table, got %T", res[0])
	}
	raw, ok := unwrapGo(self)
	if !ok {
		t.Fatal("returned table is not a GoValue")
	}

	if got := goValueMember(self, raw, "Name"); got != "ada" {
		t.Errorf("field Name: got %#v, want \"ada\"", got)
	}
	if got := goValueMember(self, raw, "hidden"); got != nil {
		t.Errorf("unexported field should be nil, got %#v", got)
	}
	if got := goValueMember(self, raw, "Nope"); got != nil {
		t.Errorf("unknown member should be nil, got %#v", got)
	}

	m, ok := goValueMember(self, raw, "Greet").(*vm.GoFunc)
	if !ok {
		t.Fatal("Greet did not resolve to a callable")
	}
	got := m.Fn(nil, []vm.Value{self, "!"})
	if len(got) != 1 || got[0] != "hi ada!" {
		t.Fatalf("g:Greet(\"!\") = %#v, want \"hi ada!\"", got)
	}

	got = m.Fn(nil, []vm.Value{"!"})
	if len(got) != 1 || got[0] != "hi ada!" {
		t.Fatalf("g.Greet(\"!\") = %#v, want \"hi ada!\"", got)
	}
}

func TestGoValueRoundTripsBackIntoGo(t *testing.T) {
	res, _ := call(t, func() *greeter { return &greeter{Name: "ada"} })
	handle := res[0]

	res, luaErr := call(t, func(g *greeter) string { return g.Name }, handle)
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}
	if res[0] != "ada" {
		t.Fatalf("got %#v, want \"ada\"", res[0])
	}
}
