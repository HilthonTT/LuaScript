package vm

import (
	"strings"
	"testing"
)

func TestIndexMetamethodWithTable(t *testing.T) {
	v := run(t, `
		base = {greeting = "hello"}
		obj = setmetatable({}, {__index = base})
		r = obj.greeting
	`)
	assertGlobalEqual(t, v, "r", "hello")
}

func TestIndexMetamethodWithFunction(t *testing.T) {
	v := run(t, `
		obj = setmetatable({}, {__index = function(t, k) return "lookup-" .. k end})
		r = obj.x
	`)
	assertGlobalEqual(t, v, "r", "lookup-x")
}

func TestIndexChainsThroughMultipleMetatables(t *testing.T) {
	v := run(t, `
		grandparent = {color = "blue"}
		parent = setmetatable({}, {__index = grandparent})
		child = setmetatable({}, {__index = parent})
		r = child.color
	`)
	assertGlobalEqual(t, v, "r", "blue")
}

func TestIndexPrefersRawValueOverMetamethod(t *testing.T) {
	v := run(t, `
		obj = setmetatable({x = "raw"}, {__index = function() return "meta" end})
		r = obj.x
	`)
	assertGlobalEqual(t, v, "r", "raw")
}

func TestNewIndexMetamethodInterceptsNewKeys(t *testing.T) {
	v := run(t, `
		log = {}
		obj = setmetatable({}, {__newindex = function(t, k, v) log[k] = v end})
		obj.foo = 42
		r = log.foo
		raw = rawget(obj, "foo")
	`)
	assertGlobalEqual(t, v, "r", int64(42))
	assertGlobalEqual(t, v, "raw", nil)
}

func TestNewIndexNotCalledForExistingKey(t *testing.T) {
	v := run(t, `
		obj = setmetatable({x = 1}, {__newindex = function() error("should not fire") end})
		obj.x = 2
		r = obj.x
	`)
	assertGlobalEqual(t, v, "r", int64(2))
}

func TestAddMetamethod(t *testing.T) {
	v := run(t, `
		mt = {__add = function(a, b) return "sum" end}
		x = setmetatable({}, mt)
		r = x + 1
	`)
	assertGlobalEqual(t, v, "r", "sum")
}

func TestSubMulDivMetamethods(t *testing.T) {
	v := run(t, `
		mt = {
			__sub = function() return "s" end,
			__mul = function() return "m" end,
			__div = function() return "d" end,
		}
		x = setmetatable({}, mt)
		a, b, c = x - 1, x * 1, x / 1
	`)
	assertGlobalEqual(t, v, "a", "s")
	assertGlobalEqual(t, v, "b", "m")
	assertGlobalEqual(t, v, "c", "d")
}

func TestUnaryMinusMetamethod(t *testing.T) {
	v := run(t, `
		x = setmetatable({n = 7}, {__unm = function(t) return -t.n end})
		r = -x
	`)
	assertGlobalEqual(t, v, "r", int64(-7))
}

func TestEqMetamethodForTables(t *testing.T) {
	v := run(t, `
		mt = {__eq = function(a, b) return a.id == b.id end}
		a = setmetatable({id = 1}, mt)
		b = setmetatable({id = 1}, mt)
		c = setmetatable({id = 2}, mt)
		ab = (a == b)
		ac = (a == c)
	`)
	assertGlobalEqual(t, v, "ab", true)
	assertGlobalEqual(t, v, "ac", false)
}

func TestLtMetamethod(t *testing.T) {
	v := run(t, `
		mt = {__lt = function(a, b) return a.n < b.n end}
		a = setmetatable({n = 1}, mt)
		b = setmetatable({n = 2}, mt)
		r = a < b
		s = b < a
	`)
	assertGlobalEqual(t, v, "r", true)
	assertGlobalEqual(t, v, "s", false)
}

func TestConcatMetamethod(t *testing.T) {
	v := run(t, `
		mt = {__concat = function(a, b) return "C" end}
		x = setmetatable({}, mt)
		r = x .. "y"
		s = "y" .. x
	`)
	assertGlobalEqual(t, v, "r", "C")
	assertGlobalEqual(t, v, "s", "C")
}

func TestLenMetamethod(t *testing.T) {
	v := run(t, `
		x = setmetatable({}, {__len = function() return 42 end})
		r = #x
	`)
	assertGlobalEqual(t, v, "r", int64(42))
}

func TestCallMetamethod(t *testing.T) {
	v := run(t, `
		x = setmetatable({prefix = "hi:"}, {__call = function(self, name) return self.prefix .. name end})
		r = x("world")
	`)
	assertGlobalEqual(t, v, "r", "hi:world")
}

func TestRawgetSkipsMetamethod(t *testing.T) {
	v := run(t, `
		obj = setmetatable({}, {__index = function() return "meta" end})
		got = rawget(obj, "missing")
	`)
	assertGlobalEqual(t, v, "got", nil)
}

func TestRawsetSkipsMetamethod(t *testing.T) {
	v := run(t, `
		count = 0
		obj = setmetatable({}, {__newindex = function() count = count + 1 end})
		rawset(obj, "k", 99)
		stored = rawget(obj, "k")
	`)
	assertGlobalEqual(t, v, "count", int64(0))
	assertGlobalEqual(t, v, "stored", int64(99))
}

func TestRawequal(t *testing.T) {
	v := run(t, `
		mt = {__eq = function() return true end}
		a = setmetatable({}, mt)
		b = setmetatable({}, mt)
		eq = (a == b)             -- metamethod kicks in
		raweq = rawequal(a, b)    -- pure pointer equality
	`)
	assertGlobalEqual(t, v, "eq", true)
	assertGlobalEqual(t, v, "raweq", false)
}

func TestClassPatternViaMetatables(t *testing.T) {
	v := run(t, `
		Animal = {}
		Animal.__index = Animal
		function Animal.new(name)
			local self = setmetatable({}, Animal)
			self.name = name
			return self
		end
		function Animal:greet()
			return "I am " .. self.name
		end
		a = Animal.new("Whiskers")
		greeting = a:greet()
	`)
	assertGlobalEqual(t, v, "greeting", "I am Whiskers")
}

func TestArithOnTableWithoutMetamethodErrors(t *testing.T) {
	msg := runErr(t, "--!nocheck\nr = ({}) + 1")
	if !strings.Contains(msg, "arithmetic") {
		t.Errorf("error = %q, want it to mention arithmetic", msg)
	}
}

func TestTostringMetamethod(t *testing.T) {
	v := run(t, `
		obj = setmetatable({}, {__tostring = function(self) return "custom-repr" end})
		r = tostring(obj)
	`)
	assertGlobalEqual(t, v, "r", "custom-repr")
}

func TestTostringFallsBackWithoutMetamethod(t *testing.T) {
	v := run(t, `
		obj = {}
		r = tostring(obj)
	`)
	g := v.Globals.Get("r")
	s, ok := g.(string)
	if !ok || !strings.HasPrefix(s, "table:") {
		t.Errorf("expected default 'table: …' rendering, got %q", s)
	}
}

func TestErrorPropagatesValueToPcall(t *testing.T) {
	v := run(t, `
		obj = setmetatable({}, {__tostring = function() return "boom" end})
		ok, err = pcall(function() error(obj) end)
		same = err == obj
		rendered = tostring(err)
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "same", true)
	assertGlobalEqual(t, v, "rendered", "boom")
}

func TestErrorPropagatesNonStringValueToPcall(t *testing.T) {
	v := run(t, `
		ok, err = pcall(function() error({code = 42}) end)
		code = err.code
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "code", int64(42))
}

func TestUncaughtErrorRoutesThroughTostringMetamethod(t *testing.T) {
	msg := runErr(t, `
		obj = setmetatable({}, {__tostring = function() return "boom" end})
		error(obj)
	`)
	if !strings.Contains(msg, "boom") {
		t.Errorf("uncaught error = %q, want it to contain %q", msg, "boom")
	}
}

func TestTostringMetamethodMustReturnString(t *testing.T) {
	msg := runErr(t, `
		obj = setmetatable({}, {__tostring = function() return 42 end})
		print(obj)
	`)
	if !strings.Contains(msg, "__tostring") {
		t.Errorf("error = %q, want it to mention __tostring", msg)
	}
}
