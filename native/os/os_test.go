package os_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/compiler"
	"github.com/hilthontt/luascript/compiler/parser"
	native_os "github.com/hilthontt/luascript/native/os"
	"github.com/hilthontt/luascript/vm"
)

// runOS compiles and runs `src` on a VM with the os module preloaded.
// Mirrors vm.run but lives in the test package; the vm.run helper is
// package-private so cannot be reused here.
func runOS(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	native_os.RegisterOSPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

// runOSErr expects execution to fail and returns the error message.
func runOSErr(t *testing.T, src string) string {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	native_os.RegisterOSPreload(v)
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatalf("expected runtime error; got success\nsource:\n%s", src)
	}
	return e.Error()
}

func TestPwdReturnsString(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		r = os.pwd()
	`)
	got, ok := v.Globals.Get("r").(string)
	if !ok || got == "" {
		t.Errorf("pwd = %v, want non-empty string", v.Globals.Get("r"))
	}
}

func TestHostnameReturnsString(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		r = os.hostname()
	`)
	got, ok := v.Globals.Get("r").(string)
	if !ok || got == "" {
		t.Errorf("hostname = %v, want non-empty string", v.Globals.Get("r"))
	}
}

func TestGetenvReturnsNilForUnset(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		r = os.getenv("_.lsc_DEFINITELY_UNSET__")
	`)
	if got := v.Globals.Get("r"); got != nil {
		t.Errorf("getenv of unset = %v (%T), want nil", got, got)
	}
}

func TestModuleVersionExposed(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		r = os.VERSION
	`)
	if got := v.Globals.Get("r"); got != "0.1.0" {
		t.Errorf("VERSION = %v, want 0.1.0", got)
	}
}

func TestPlatformAndArchExposed(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		p = os.platform
		a = os.arch
	`)
	if p, ok := v.Globals.Get("p").(string); !ok || p == "" {
		t.Errorf("platform = %v, want non-empty string", v.Globals.Get("p"))
	}
	if a, ok := v.Globals.Get("a").(string); !ok || a == "" {
		t.Errorf("arch = %v, want non-empty string", v.Globals.Get("a"))
	}
}

func TestSeekConstantsAreIntegers(t *testing.T) {
	v := runOS(t, `
		local os = require("os")
		a = os.seek_set
		b = os.seek_cur
		c = os.seek_end
	`)
	for _, name := range []string{"a", "b", "c"} {
		if _, ok := v.Globals.Get(name).(int64); !ok {
			t.Errorf("%s = %v (%T), want int64", name, v.Globals.Get(name), v.Globals.Get(name))
		}
	}
}

func TestCreateWriteReadRoundtrip(t *testing.T) {
	tmp := filepath.ToSlash(filepath.Join(t.TempDir(), "rt.txt"))
	src := `
		local os = require("os")
		local f = os.create("` + tmp + `")
		f:write("hello world")
		f:close()

		local g = os.open("` + tmp + `", os.o_rdonly, 0)
		r = g:read(11)
		g:close()
	`
	v := runOS(t, src)
	if got := v.Globals.Get("r"); got != "hello world" {
		t.Errorf("read = %v, want %q", got, "hello world")
	}
}

func TestStatReturnsTable(t *testing.T) {
	tmp := filepath.ToSlash(filepath.Join(t.TempDir(), "stat.txt"))
	src := `
		local os = require("os")
		local f = os.create("` + tmp + `")
		f:write("abc")
		local s = f:stat()
		name = s.name
		size = s.size
		is_dir = s.is_dir
		f:close()
	`
	v := runOS(t, src)
	if got, _ := v.Globals.Get("name").(string); got != "stat.txt" {
		t.Errorf("name = %v, want stat.txt", v.Globals.Get("name"))
	}
	if got, _ := v.Globals.Get("size").(int64); got != 3 {
		t.Errorf("size = %v, want 3", v.Globals.Get("size"))
	}
	if got := v.Globals.Get("is_dir"); got != false {
		t.Errorf("is_dir = %v, want false", got)
	}
}

func TestMkdirAndRemove(t *testing.T) {
	tmp := filepath.ToSlash(filepath.Join(t.TempDir(), "subdir"))
	src := `
		local os = require("os")
		os.mkdir("` + tmp + `", 493)  -- 0755 in decimal
		os.remove("` + tmp + `")
		r = "ok"
	`
	v := runOS(t, src)
	if got := v.Globals.Get("r"); got != "ok" {
		t.Errorf("r = %v, want ok", got)
	}
}

func TestCreateRejectsNonString(t *testing.T) {
	// Lua allows number→string coercion for StringArg, so we pass a
	// table — which cannot be coerced to a string.
	msg := runOSErr(t, `
		local os = require("os")
		os.create({})
	`)
	if !strings.Contains(msg, "string expected") {
		t.Errorf("error = %q, want it to mention 'string expected'", msg)
	}
}

func TestOpenRequiresThreeArgs(t *testing.T) {
	msg := runOSErr(t, `
		local os = require("os")
		os.open("foo")
	`)
	// Either missing-arg or unable-to-open style error is acceptable;
	// just confirm we error out rather than silently succeeding.
	if msg == "" {
		t.Errorf("expected an error message")
	}
}
