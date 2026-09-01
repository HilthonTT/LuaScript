package json_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	nativejson "github.com/hilthontt/luascript/internal/native/stdlib/json"
	"github.com/hilthontt/luascript/internal/vm"
)

func runJSON(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	nativejson.RegisterJSONPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func runJSONErr(t *testing.T, src string) string {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	nativejson.RegisterJSONPreload(v)
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatalf("expected runtime error; got success\nsource:\n%s", src)
	}
	return e.Error()
}

func TestDecodeRoundTrip(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		local t = json.decode('{"a": 1, "b": [10, 20], "c": "x", "d": true}')
		a = t.a
		b = t.b[2]
		c = t.c
		d = t.d
	`)
	if got := v.Globals.Get("a"); !vm.Equal(got, int64(1)) {
		t.Errorf("a = %v, want 1", got)
	}
	if got := v.Globals.Get("b"); !vm.Equal(got, int64(20)) {
		t.Errorf("b = %v, want 20", got)
	}
	if got := v.Globals.Get("c"); !vm.Equal(got, "x") {
		t.Errorf("c = %v, want \"x\"", got)
	}
	if got := v.Globals.Get("d"); !vm.Equal(got, true) {
		t.Errorf("d = %v, want true", got)
	}
}

func TestDecodeRejectsTrailingContent(t *testing.T) {
	for _, payload := range []string{
		`{"a":1} <html>`,
		`{"a":1}{"b":2}`,
		`[1,2] garbage`,
	} {
		msg := runJSONErr(t, `
			local json = require("json")
			json.decode('`+payload+`')
		`)
		if !strings.Contains(msg, "trailing content") {
			t.Errorf("decode(%q): got %q, want a trailing-content error", payload, msg)
		}
	}
}

func TestDecodeAllowsTrailingWhitespace(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		r = json.decode('  {"a": 1}  \n')
		a = r.a
	`)
	if got := v.Globals.Get("a"); !vm.Equal(got, int64(1)) {
		t.Errorf("a = %v, want 1", got)
	}
}

func TestEncodeRejectsUnencodableValues(t *testing.T) {
	msg := runJSONErr(t, `
		local json = require("json")
		json.encode({ cb = function() end })
	`)
	if !strings.Contains(msg, "cannot encode") {
		t.Errorf("encode of a function: got %q, want a 'cannot encode' error", msg)
	}
	if strings.Contains(msg, "0x") {
		t.Errorf("encode error leaked a pointer: %q", msg)
	}
}

func TestEncodeArraysAndObjects(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		arr = json.encode({1, 2, 3})
		obj = json.encode({a = 1})
	`)
	if got := v.Globals.Get("arr"); !vm.Equal(got, "[1,2,3]") {
		t.Errorf("encode({1,2,3}) = %v, want [1,2,3]", got)
	}
	if got := v.Globals.Get("obj"); !vm.Equal(got, `{"a":1}`) {
		t.Errorf("encode({a=1}) = %v, want {\"a\":1}", got)
	}
}

func TestDecodeNullSentinel(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")

		-- Default: nulls become nil, as they always have.
		local dflt = json.decode('[1, null, 3]')
		defaultLen = #dflt

		-- Opt in and the array keeps its shape.
		local kept = json.decode('[1, null, 3]', { null = json.null })
		keptLen = #kept
		middleIsNull = kept[2] == json.null
		lastValue = kept[3]

		-- An explicit null is now distinguishable from a missing key.
		local obj = json.decode('{"present": null}', { null = json.null })
		presentIsNull = obj.present == json.null
		absentIsNil = obj.missing == nil
	`)
	if got := v.Globals.Get("defaultLen"); !vm.Equal(got, int64(1)) {
		t.Errorf("default decode of [1,null,3] has length %v, want 1 (unchanged behaviour)", got)
	}
	if got := v.Globals.Get("keptLen"); !vm.Equal(got, int64(3)) {
		t.Errorf("sentinel decode of [1,null,3] has length %v, want 3", got)
	}
	for _, name := range []string{"middleIsNull", "presentIsNull", "absentIsNil"} {
		if got := v.Globals.Get(name); !vm.Equal(got, true) {
			t.Errorf("%s = %v, want true", name, got)
		}
	}
	if got := v.Globals.Get("lastValue"); !vm.Equal(got, int64(3)) {
		t.Errorf("lastValue = %v, want 3", got)
	}
}

func TestEncodeNullSentinel(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		obj = json.encode({ a = json.null })
		arr = json.encode({ 1, json.null, 3 })
	`)
	if got := v.Globals.Get("obj"); !vm.Equal(got, `{"a":null}`) {
		t.Errorf("encode({a = json.null}) = %v, want {\"a\":null}", got)
	}
	if got := v.Globals.Get("arr"); !vm.Equal(got, `[1,null,3]`) {
		t.Errorf("encode({1, json.null, 3}) = %v, want [1,null,3]", got)
	}
}

func TestEncodeEmptyArrayMarker(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		plain = json.encode({})
		forced = json.encode(json.empty_array)
		nested = json.encode({ items = json.empty_array })
	`)
	if got := v.Globals.Get("plain"); !vm.Equal(got, "{}") {
		t.Errorf("encode({}) = %v, want {} (unchanged)", got)
	}
	if got := v.Globals.Get("forced"); !vm.Equal(got, "[]") {
		t.Errorf("encode(json.empty_array) = %v, want []", got)
	}
	if got := v.Globals.Get("nested"); !vm.Equal(got, `{"items":[]}`) {
		t.Errorf("nested empty_array = %v, want {\"items\":[]}", got)
	}
}

func TestEncodeIndent(t *testing.T) {
	v := runJSON(t, `
		local json = require("json")
		compact = json.encode({ a = 1 })
		spaced = json.encode({ a = 1 }, { indent = 2 })
		tabbed = json.encode({ a = 1 }, { indent = "\t" })
	`)
	if got := v.Globals.Get("compact"); !vm.Equal(got, `{"a":1}`) {
		t.Errorf("compact = %v, want no whitespace", got)
	}
	if got := v.Globals.Get("spaced"); !vm.Equal(got, "{\n  \"a\": 1\n}") {
		t.Errorf("indent = 2 gave %q", got)
	}
	if got := v.Globals.Get("tabbed"); !vm.Equal(got, "{\n\t\"a\": 1\n}") {
		t.Errorf("indent = tab gave %q", got)
	}
}
