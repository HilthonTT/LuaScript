package iox

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/hilthontt/luascript/internal/vm"
)

func ownFile(h *fileHandle) *fileHandle {
	runtime.SetFinalizer(h, func(h *fileHandle) {
		if !h.closed && !h.isStd {
			_ = h.f.Close()
		}
	})
	return h
}

func RegisterIOPreload(v *vm.VM) {
	vm.RegisterPreload(v, "io", loader)
}

func loader(_ *vm.VM, _ []vm.Value) []vm.Value {
	return []vm.Value{newIO()}
}

type fileHandle struct {
	f      *os.File
	br     *bufio.Reader
	closed bool
	isStd  bool
}

func newIO() *vm.Table {
	m := vm.NewTable(0, 10)
	methods := vm.NewTable(0, 8)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "io." + name, Fn: fn})
	}

	stdinH := &fileHandle{f: os.Stdin, br: bufio.NewReader(os.Stdin), isStd: true}
	stdoutH := &fileHandle{f: os.Stdout, isStd: true}
	stderrH := &fileHandle{f: os.Stderr, isStd: true}

	stdinT := newFileTable(stdinH)
	stdoutT := newFileTable(stdoutH)
	stderrT := newFileTable(stderrH)

	m.Set("stdin", stdinT)
	m.Set("stdout", stdoutT)
	m.Set("stderr", stderrT)

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
		return []vm.Value{newFileTable(ownFile(&fileHandle{f: f, br: bufio.NewReader(f)}))}
	})

	add("lines", func(_ *vm.VM, args []vm.Value) []vm.Value {
		var br *bufio.Reader
		var owned *os.File
		if len(args) >= 1 {
			path := vm.StringArg("io.lines", 1, args)
			f, err := os.Open(path)
			if err != nil {
				panic(vm.Errorf("io.lines: %s", err.Error()))
			}
			br = bufio.NewReader(f)
			owned = f
		} else {
			br = defaultIn.br
		}
		var once sync.Once
		closeOwned := func() {
			once.Do(func() {
				if owned != nil {
					_ = owned.Close()
				}
			})
		}
		iter := &vm.GoFunc{Name: "io:lines:iter", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			line, err := br.ReadString('\n')
			if err != nil && line == "" {
				closeOwned()
				return []vm.Value{nil}
			}
			line = strings.TrimRight(line, "\n")
			line = strings.TrimRight(line, "\r")
			return []vm.Value{line}
		}}
		if owned != nil {
			runtime.SetFinalizer(iter, func(*vm.GoFunc) { closeOwned() })
		}
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
		return []vm.Value{newFileTable(defaultOut)}
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
		if !defaultOut.closed && !defaultOut.isStd {
			_ = defaultOut.f.Sync()
		}
		return []vm.Value{newFileTable(defaultOut)}
	})

	add("tmpfile", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		f, err := os.CreateTemp("", "lsc_tmp_*")
		if err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{newFileTable(ownFile(&fileHandle{f: f, br: bufio.NewReader(f)}))}
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
			defaultIn = ownFile(&fileHandle{f: f, br: bufio.NewReader(f)})
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
			defaultOut = ownFile(&fileHandle{f: f})
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

func handleFrom(v vm.Value) *fileHandle {
	t, ok := v.(*vm.Table)
	if !ok {
		return nil
	}
	raw := t.Get("__handle")
	h, _ := raw.(*fileHandle)
	return h
}

func newFileTable(h *fileHandle) *vm.Table {
	t := vm.NewTable(0, 8)
	t.Set("__handle", h)

	methods := vm.NewTable(0, 8)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "file:" + name, Fn: fn})
	}

	add("read", func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:read", 1, args)
		return readFormats(h, args, 2)
	})
	add("write", func(v *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("file:write", 1, args)
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
		_ = vm.TableArg("file:lines", 1, args)
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

func readFormats(h *fileHandle, args []vm.Value, argStart int) []vm.Value {
	if h.closed {
		return []vm.Value{nil, "file is closed"}
	}
	if h.br == nil {
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
				out = append(out, "")
				continue
			}
			data, err := io.ReadAll(io.LimitReader(h.br, n))
			if err != nil {
				panic(vm.Errorf("io.read: %s", err.Error()))
			}
			if len(data) == 0 {
				out = append(out, nil)
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
		return []vm.Value{true}
	}
	if err := h.f.Close(); err != nil {
		return []vm.Value{nil, err.Error()}
	}
	h.closed = true
	return []vm.Value{true}
}

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
