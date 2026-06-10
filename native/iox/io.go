// Package iox implements Lua 5.4's `io` library — file handles with
// :read/:write/:lines/:seek/:flush/:close methods, plus the module-level
// helpers io.open / io.lines / io.read / io.write / io.close / io.type /
// io.tmpfile. The package directory is `iox` (not `io`) so it does not
// clash with Go's standard library; the module is exposed to Lua as `io`.
//
// `io.write` is also installed by the core stdlib's `io` library
// (vm/stdlib_modules.go) for the convenience of `print`-style usage on
// stdout. This module's `io.write` takes precedence when the user does
// `require("io")` because the require result overrides the core view.
package iox

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hilthontt/luascript/vm"
)

// RegisterIOPreload installs the io module at package.preload as "io".
func RegisterIOPreload(v *vm.VM) {
	vm.RegisterPreload(v, "io", loader)
}

func loader(_ *vm.VM, _ []vm.Value) []vm.Value {
	return []vm.Value{newIO()}
}

// fileHandle is the Go-side state behind every Lua file table. The Lua
// table itself just exposes methods; the *os.File and buffered reader
// are kept here in a closure-captured struct.
type fileHandle struct {
	f      *os.File
	br     *bufio.Reader
	closed bool
	// isStd indicates a process-owned stdin/stdout/stderr handle. Close()
	// on these is a no-op so user scripts can't sever the process's std
	// streams by closing them.
	isStd bool
}

func newIO() *vm.Table {
	m := vm.NewTable(0, 10)
	methods := vm.NewTable(0, 8)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "io." + name, Fn: fn})
	}

	// Pre-built stdio handles. Reads go through a shared bufio.Reader
	// so partial-line reads compose; writes go through os.File directly.
	stdinH := &fileHandle{f: os.Stdin, br: bufio.NewReader(os.Stdin), isStd: true}
	stdoutH := &fileHandle{f: os.Stdout, isStd: true}
	stderrH := &fileHandle{f: os.Stderr, isStd: true}

	stdinT := newFileTable(stdinH)
	stdoutT := newFileTable(stdoutH)
	stderrT := newFileTable(stderrH)

	m.Set("stdin", stdinT)
	m.Set("stdout", stdoutT)
	m.Set("stderr", stderrT)

	// default_output / default_input track which handle io.write / io.read
	// route through when no explicit file argument is given. Lua uses
	// io.output() / io.input() for this; we expose simpler accessors and
	// keep references in Go-side state.
	defaultIn := stdinH
	defaultOut := stdoutH

	add("open", func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("io.open", 1, args)
		mode := "r"
		if len(args) >= 2 {
			mode = vm.StringArg("io.open", 2, args)
		}
		f, err := openMode(path, mode)
		if err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{newFileTable(&fileHandle{f: f, br: bufio.NewReader(f)})}
	})

	add("lines", func(_ *vm.VM, args []vm.Value) []vm.Value {
		var br *bufio.Reader
		var closeOnDone *os.File
		if len(args) >= 1 {
			path := vm.StringArg("io.lines", 1, args)
			f, err := os.Open(path)
			if err != nil {
				panic(vm.Errorf("io.lines: %s", err.Error()))
			}
			br = bufio.NewReader(f)
			closeOnDone = f
		} else {
			br = defaultIn.br
		}
		iter := &vm.GoFunc{Name: "io:lines:iter", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			line, err := br.ReadString('\n')
			if err != nil && line == "" {
				if closeOnDone != nil {
					_ = closeOnDone.Close()
				}
				return []vm.Value{nil}
			}
			line = strings.TrimRight(line, "\n")
			line = strings.TrimRight(line, "\r")
			return []vm.Value{line}
		}}
		return []vm.Value{iter}
	})

	add("read", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return readFormats(defaultIn, args, 1)
	})

	add("write", func(v *vm.VM, args []vm.Value) []vm.Value {
		if defaultOut.closed {
			panic(vm.Errorf("io.write: default output is closed"))
		}
		for _, a := range args {
			fmt.Fprint(defaultOut.f, vm.ToStringMM(v, a))
		}
		// Lua returns the default-output handle so io.write() chains.
		return []vm.Value{stdoutT}
	})

	add("close", func(_ *vm.VM, args []vm.Value) []vm.Value {
		var h *fileHandle
		if len(args) >= 1 {
			h = handleFrom(args[0])
		} else {
			h = defaultOut
		}
		if h == nil {
			return []vm.Value{nil, "io.close: not a file handle"}
		}
		return closeHandle(h)
	})

	add("flush", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if defaultOut.closed || defaultOut.isStd {
			return []vm.Value{stdoutT}
		}
		_ = defaultOut.f.Sync()
		return []vm.Value{stdoutT}
	})

	add("tmpfile", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		f, err := os.CreateTemp("", "lsc_tmp_*")
		if err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{newFileTable(&fileHandle{f: f, br: bufio.NewReader(f)})}
	})

	add("type", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 {
			return []vm.Value{nil}
		}
		h := handleFrom(args[0])
		if h == nil {
			return []vm.Value{nil}
		}
		if h.closed {
			return []vm.Value{"closed file"}
		}
		return []vm.Value{"file"}
	})

	// io.input(filename or file?) / io.output(filename or file?). With no
	// arg they return the current default handle; with one they switch it.
	add("input", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 {
			return []vm.Value{newFileTable(defaultIn)}
		}
		switch a := args[0].(type) {
		case string:
			f, err := os.Open(a)
			if err != nil {
				panic(vm.Errorf("io.input: %s", err.Error()))
			}
			defaultIn = &fileHandle{f: f, br: bufio.NewReader(f)}
		default:
			h := handleFrom(a)
			if h == nil {
				panic(vm.Errorf("io.input: expected file or filename"))
			}
			defaultIn = h
		}
		return []vm.Value{newFileTable(defaultIn)}
	})
	add("output", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 {
			return []vm.Value{newFileTable(defaultOut)}
		}
		switch a := args[0].(type) {
		case string:
			f, err := os.Create(a)
			if err != nil {
				panic(vm.Errorf("io.output: %s", err.Error()))
			}
			defaultOut = &fileHandle{f: f}
		default:
			h := handleFrom(a)
			if h == nil {
				panic(vm.Errorf("io.output: expected file or filename"))
			}
			defaultOut = h
		}
		return []vm.Value{newFileTable(defaultOut)}
	})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// handleFrom unwraps a file-table back to its *fileHandle. Returns nil if
// arg is not a file table created by this module.
func handleFrom(v vm.Value) *fileHandle {
	t, ok := v.(*vm.Table)
	if !ok {
		return nil
	}
	raw := t.Get("__handle")
	h, _ := raw.(*fileHandle)
	return h
}

// newFileTable builds the Lua-visible table for a file handle. The handle
// itself is stashed at table["__handle"] so the methods can retrieve it
// via colon dispatch; "__handle" is mangled enough that user scripts are
// unlikely to overwrite it accidentally.
func newFileTable(h *fileHandle) *vm.Table {
	t := vm.NewTable(0, 8)
	t.Set("__handle", h)

	methods := vm.NewTable(0, 8)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "file:" + name, Fn: fn})
	}

	add("read", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:read", 1, args) // discard self
		return readFormats(h, args, 2)
	})
	add("write", func(v *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:write", 1, args) // discard self
		if h.closed {
			return []vm.Value{nil, "file is closed"}
		}
		for i := 1; i < len(args); i++ {
			if _, err := fmt.Fprint(h.f, vm.ToStringMM(v, args[i])); err != nil {
				return []vm.Value{nil, err.Error()}
			}
		}
		return []vm.Value{t}
	})
	add("lines", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:lines", 1, args) // discard self
		iter := &vm.GoFunc{Name: "file:lines:iter", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			if h.closed {
				return []vm.Value{nil}
			}
			line, err := h.br.ReadString('\n')
			if err != nil && line == "" {
				return []vm.Value{nil}
			}
			line = strings.TrimRight(line, "\n")
			line = strings.TrimRight(line, "\r")
			return []vm.Value{line}
		}}
		return []vm.Value{iter}
	})
	add("seek", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:seek", 1, args)
		whence := "cur"
		offset := int64(0)
		if len(args) >= 2 {
			if s, ok := args[1].(string); ok {
				whence = s
			}
		}
		if len(args) >= 3 {
			offset = vm.IntArg("file:seek", 3, args)
		}
		var w int
		switch whence {
		case "set":
			w = io.SeekStart
		case "cur":
			w = io.SeekCurrent
		case "end":
			w = io.SeekEnd
		default:
			return []vm.Value{nil, "invalid whence: " + whence}
		}
		pos, err := h.f.Seek(offset, w)
		if err != nil {
			return []vm.Value{nil, err.Error()}
		}
		// Seeking invalidates the buffered reader.
		if h.br != nil {
			h.br.Reset(h.f)
		}
		return []vm.Value{pos}
	})
	add("flush", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:flush", 1, args)
		if h.closed || h.isStd {
			return []vm.Value{t}
		}
		if err := h.f.Sync(); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{t}
	})
	add("close", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:close", 1, args)
		return closeHandle(h)
	})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	t.SetMetatable(mt)
	return t
}

// readFormats implements the shared read-format logic for both io.read
// and file:read. The reader argument is the handle to read from; argStart
// is the index in args where the format strings begin (1 for io.read,
// 2 for file:read because of the self slot).
func readFormats(h *fileHandle, args []vm.Value, argStart int) []vm.Value {
	if h.closed {
		return []vm.Value{nil, "file is closed"}
	}
	if h.br == nil {
		// Output-only handle (e.g. stdout opened via io.output(name)).
		return []vm.Value{nil, "file is not readable"}
	}
	formats := args[argStart-1:]
	if len(formats) == 0 {
		formats = []vm.Value{"l"}
	}
	out := make([]vm.Value, 0, len(formats))
	for _, f := range formats {
		switch fv := f.(type) {
		case string:
			fmtStr := strings.TrimPrefix(fv, "*")
			switch fmtStr {
			case "l":
				line, err := h.br.ReadString('\n')
				if err != nil && line == "" {
					out = append(out, nil)
					return out
				}
				line = strings.TrimRight(line, "\n")
				line = strings.TrimRight(line, "\r")
				out = append(out, line)
			case "L":
				line, err := h.br.ReadString('\n')
				if err != nil && line == "" {
					out = append(out, nil)
					return out
				}
				out = append(out, line)
			case "a":
				var b strings.Builder
				buf := make([]byte, 4096)
				for {
					n, err := h.br.Read(buf)
					if n > 0 {
						b.Write(buf[:n])
					}
					if err != nil {
						break
					}
				}
				out = append(out, b.String())
			case "n":
				// Read a number — skip leading spaces, accumulate digits/sign/dot/e.
				if err := skipSpaces(h.br); err != nil {
					out = append(out, nil)
					return out
				}
				num, err := readNumber(h.br)
				if err != nil {
					out = append(out, nil)
					return out
				}
				out = append(out, num)
			default:
				panic(vm.Errorf("io.read: invalid format %q", fv))
			}
		default:
			n, ok := vm.ToInteger(fv)
			if !ok {
				panic(vm.Errorf("io.read: invalid format"))
			}
			if n < 0 {
				panic(vm.Errorf("io.read: invalid format (negative count)"))
			}
			if n == 0 {
				// Lua semantics: returns "" if at EOF or not, depending on err.
				out = append(out, "")
				continue
			}
			// Read up to n bytes via a LimitReader so a huge n over a short
			// stream grows the buffer only to what's actually available
			// (no upfront make([]byte, n) OOM, no negative-length crash).
			data, err := io.ReadAll(io.LimitReader(h.br, n))
			if err != nil {
				panic(vm.Errorf("io.read: %s", err.Error()))
			}
			if len(data) == 0 {
				out = append(out, nil) // at EOF
				return out
			}
			out = append(out, string(data))
		}
	}
	return out
}

func closeHandle(h *fileHandle) []vm.Value {
	if h.closed {
		return []vm.Value{nil, "file is already closed"}
	}
	if h.isStd {
		// Closing a std handle is a no-op so scripts can't break the process.
		return []vm.Value{true}
	}
	if err := h.f.Close(); err != nil {
		return []vm.Value{nil, err.Error()}
	}
	h.closed = true
	return []vm.Value{true}
}

// openMode translates a Lua mode string into Go's O_* flags. Lua modes:
// "r" (read), "w" (write/truncate), "a" (append), "r+"/"w+"/"a+" for
// update; trailing "b" is accepted and ignored (no text mode here).
func openMode(path, mode string) (*os.File, error) {
	mode = strings.TrimSuffix(mode, "b")
	var flag int
	switch mode {
	case "r":
		flag = os.O_RDONLY
	case "w":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "r+":
		flag = os.O_RDWR
	case "w+":
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a+":
		flag = os.O_RDWR | os.O_CREATE | os.O_APPEND
	default:
		return nil, fmt.Errorf("invalid mode: %q", mode)
	}
	return os.OpenFile(path, flag, 0644)
}

func skipSpaces(br *bufio.Reader) error {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return br.UnreadByte()
		}
	}
}

// readNumber consumes a numeric token (decimal or float) from br and
// returns it as int64 or float64. Tolerant of trailing junk — once the
// pattern stops matching, the rest stays in the reader.
func readNumber(br *bufio.Reader) (vm.Value, error) {
	var b strings.Builder
	saw := false
	for {
		c, err := br.ReadByte()
		if err != nil {
			break
		}
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' || c == 'e' || c == 'E' {
			b.WriteByte(c)
			saw = true
			continue
		}
		_ = br.UnreadByte()
		break
	}
	if !saw {
		return nil, fmt.Errorf("no number")
	}
	s := b.String()
	if strings.ContainsAny(s, ".eE") {
		var f float64
		if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
			return nil, err
		}
		return f, nil
	}
	var i int64
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil {
		return nil, err
	}
	return i, nil
}
