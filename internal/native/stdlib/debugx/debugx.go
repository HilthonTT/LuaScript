// Package debugx implements the `debug` native module. It covers the
// Lua 5.4 debug surface we can reasonably support given the VM's current
// frame representation:
//
//   - debug.traceback([msg], [level]) -> string
//   - debug.getinfo(level|function)   -> table {source, currentline, name, what, nparams, isvararg}
//   - debug.sethook(...)              -> stub no-op (signature compat only)
//   - debug.gethook()                 -> nil, nil, 0  (matches Lua's "no hook" reply)
//
// Local/upvalue introspection (getlocal/setlocal/getupvalue/setupvalue)
// would need the bytecode generator to emit a stable local-name table per
// proto, which it doesn't yet. Leaving those out for now keeps the module
// honest — every function it exposes does what its Lua-5.4 counterpart
// does, rather than silently returning nil.
//
// Package name `debugx` avoids colliding with Go's stdlib `runtime/debug`.
// The Lua-side module name is "debug".
package debugx

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterDebugPreload installs the `debug` module under package.preload.
func RegisterDebugPreload(v *vm.VM) {
	vm.RegisterPreload(v, "debug", debugLoader)
}

func debugLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newDebug()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newDebug() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 4)

	// debug.traceback([message], [level]) -> string
	//
	// `level` is 1-based and counts from the caller; level=1 skips this
	// builtin frame itself, matching Lua's convention. The traceback's
	// header line and `[C]:` / `<source>:<line>` formatting follow
	// Lua 5.4's output closely enough that downstream regex parsers (CI
	// log scrapers, IDE error squigglers) work without adjustment.
	methods.Set("traceback", &vm.GoFunc{Name: "debug:traceback", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		msg := ""
		if len(args) >= 1 {
			if s, ok := args[0].(string); ok {
				msg = s
			} else if args[0] != nil {
				// Non-string, non-nil message: per Lua, return it unchanged.
				return []vm.Value{args[0]}
			}
		}
		level := vm.OptInt("debug.traceback", 2, args, 1)
		return []vm.Value{formatTraceback(v, msg, int(level))}
	}})

	// debug.getinfo(level | function) -> table | nil
	//
	// The accepted "what" string of Lua's getinfo is ignored — we always
	// fill in every field we know how to produce. Calls beyond the frame
	// stack return nil, matching Lua's behaviour for out-of-range level.
	methods.Set("getinfo", &vm.GoFunc{Name: "debug:getinfo", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 {
			panic(vm.Errorf("bad argument #1 to 'debug.getinfo' (number or function expected)"))
		}
		switch x := args[0].(type) {
		case int64:
			info := frameInfo(v, int(x))
			if info == nil {
				return []vm.Value{nil}
			}
			return []vm.Value{info}
		case float64:
			info := frameInfo(v, int(x))
			if info == nil {
				return []vm.Value{nil}
			}
			return []vm.Value{info}
		case *vm.Closure:
			return []vm.Value{closureInfo(x)}
		case *vm.GoFunc:
			return []vm.Value{goFuncInfo(x)}
		}
		panic(vm.Errorf("bad argument #1 to 'debug.getinfo' (number or function expected, got %s)", vm.TypeName(args[0])))
	}})

	// debug.sethook / gethook — stubs. The VM has no instruction hook
	// or yield-trigger plumbing, and faking either would silently break
	// any program that actually relied on the hook firing. We accept the
	// arguments (so existing Lua code that calls sethook(nil) at startup
	// keeps working) and return nothing.
	methods.Set("sethook", &vm.GoFunc{Name: "debug:sethook", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return nil
	}})
	methods.Set("gethook", &vm.GoFunc{Name: "debug:gethook", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		// Lua returns (hook, mask, count). With no hook installed, the
		// canonical reply is (nil, "", 0).
		return []vm.Value{nil, "", int64(0)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// formatTraceback walks v.CallFrames() from inner to outer and renders a
// Lua-style traceback. `level` is 1-based and skips that many caller
// frames before listing; the conventional value is 1 (skip the helper
// itself). A negative or zero level lists every frame.
func formatTraceback(v *vm.VM, msg string, level int) string {
	frames := v.CallFrames()
	var b strings.Builder
	if msg != "" {
		b.WriteString(msg)
		b.WriteByte('\n')
	}
	b.WriteString("stack traceback:")

	// Skip the topmost `level` frames so the user's caller is the first
	// entry shown. The traceback line itself was already pushed onto
	// frames by the time we run, so level=1 hides this function's frame.
	end := len(frames) - level
	if level <= 0 || end > len(frames) {
		// Negative or zero level lists every frame (and must not index
		// past the slice).
		end = len(frames)
	}
	if end < 0 {
		end = 0
	}
	for i := end - 1; i >= 0; i-- {
		f := frames[i]
		b.WriteByte('\n')
		b.WriteByte('\t')
		b.WriteString(frameLocation(f))
		b.WriteString(": in ")
		b.WriteString(frameLabel(f))
	}
	return b.String()
}

// frameInfo turns the n-th frame (1-based) into a Lua-shaped info table.
// Returns nil if the level is out of range — matching Lua's getinfo,
// which returns nil rather than raising.
//
// Indexing follows Lua 5.4's convention: `getinfo(1)` is the function
// that *called* debug.getinfo (i.e. the most recent Lua frame on the
// stack). Because debug.getinfo is implemented as a GoFunc and GoFuncs
// don't push their own CallFrame, the innermost CallFrame already is
// the caller — so level=1 maps to frames[N-1], level=2 to frames[N-2],
// and so on outward.
func frameInfo(v *vm.VM, level int) *vm.Table {
	frames := v.CallFrames()
	idx := len(frames) - level
	if level <= 0 || idx < 0 || idx >= len(frames) {
		return nil
	}
	f := frames[idx]
	return closureInfo(f.Closure)
}

// closureInfo fills the standard getinfo table for a Lua closure.
func closureInfo(c *vm.Closure) *vm.Table {
	t := vm.NewTable(0, 8)
	t.Set("what", "Lua")
	t.Set("source", protoSource(c))
	t.Set("short_src", protoSource(c))
	t.Set("currentline", int64(-1)) // unknown without a live frame
	t.Set("name", protoName(c))
	t.Set("namewhat", "")
	t.Set("nparams", int64(c.Proto.NumParams))
	t.Set("isvararg", c.Proto.IsVararg)
	return t
}

// goFuncInfo is the C-function shape of getinfo's result.
func goFuncInfo(g *vm.GoFunc) *vm.Table {
	t := vm.NewTable(0, 4)
	t.Set("what", "C")
	t.Set("source", "=[C]")
	t.Set("short_src", "[C]")
	t.Set("currentline", int64(-1))
	t.Set("name", g.Name)
	t.Set("namewhat", "")
	return t
}

// frameLocation renders the `<source>:<line>` prefix used on each
// traceback line. A frame without a current instruction (paused at the
// very entry) falls back to the proto name.
func frameLocation(f *vm.CallFrame) string {
	if f == nil || f.Closure == nil || f.Closure.Proto == nil {
		return "[?]"
	}
	src := protoSource(f.Closure)
	line := currentLine(f)
	if line > 0 {
		return fmt.Sprintf("%s:%d", src, line)
	}
	return src
}

// frameLabel renders the trailing "in function 'name'" half of a
// traceback entry. Anonymous chunks become "main chunk"; nested
// functions inherit the proto name set by the bytecode generator.
func frameLabel(f *vm.CallFrame) string {
	if f == nil || f.Closure == nil || f.Closure.Proto == nil {
		return "?"
	}
	name := protoName(f.Closure)
	if name == "" {
		return "main chunk"
	}
	return "function '" + name + "'"
}

// currentLine pulls the source line of the instruction the frame is
// currently parked on. Each instruction stamps its originating source
// line; if the IP is out of range (e.g. the frame just returned), we
// fall back to the last known line, which is still useful for the
// traceback header.
func currentLine(f *vm.CallFrame) int {
	if f.Closure == nil || f.Closure.Proto == nil {
		return 0
	}
	ins := f.Closure.Proto.Instructions
	if len(ins) == 0 {
		return 0
	}
	idx := f.IP
	if idx >= len(ins) {
		idx = len(ins) - 1
	} else if idx < 0 {
		idx = 0
	}
	return ins[idx].SourceLine()
}

// protoSource returns the human-readable source identifier for a
// closure's prototype. We use the proto Name() — that's what the
// generator stamps for both the top-level chunk and inner functions.
func protoSource(c *vm.Closure) string {
	if c == nil || c.Proto == nil {
		return "[?]"
	}
	n := c.Proto.Name()
	if n == "" {
		return "?"
	}
	return n
}

// protoName extracts a callable display name from the prototype. For
// the top-level chunk this is empty (handled by frameLabel above).
func protoName(c *vm.Closure) string {
	if c == nil || c.Proto == nil {
		return ""
	}
	return c.Proto.Name()
}
