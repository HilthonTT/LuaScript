package vm

// The baseline `io` library (stdin/stdout only; see internal/native/stdlib/iox for the full one).

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// io — stdin/stdout only.
//
// This is the baseline `io` every VM gets, including embedded hosts that
// register no native modules. The full Lua 5.4 library (file handles,
// io.open/lines/seek, io.input/output redirection) lives in
// internal/native/stdlib/iox and replaces this table on the `io` global once
// the native registrars run; see cmd/luascript/natives.go. What stays here is
// the subset that needs no file system: reading stdin and writing stdout, with
// the same format strings the full library accepts so scripts behave the same
// either way.

func buildIOLibrary() *Table {
	t := NewTable(0, 4)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "io." + name, Fn: fn})
	}

	stdinReader := bufio.NewReader(os.Stdin)
	add("write", func(v *VM, args []Value) []Value {
		for _, a := range args {
			fmt.Fprint(os.Stdout, ToStringMM(v, a))
		}
		return nil
	})
	add("read", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return []Value{readLine(stdinReader, false)}
		}
		out := make([]Value, 0, len(args))
		for i := range args {
			// A numeric format is a byte count; a string format is one of
			// l/L/n/a, optionally with Lua 5.2's leading '*'.
			if n, ok := ToInteger(args[i]); ok {
				out = append(out, readCount(stdinReader, n))
				continue
			}
			spec := strings.TrimPrefix(StringArg("io.read", i+1, args), "*")
			if spec == "" {
				panic(Errorf("bad argument #%d to 'io.read' (invalid format)", i+1))
			}
			// Lua matches on the first letter only, so "line"/"number"/"all"
			// work as well as the one-letter forms.
			switch spec[0] {
			case 'l':
				out = append(out, readLine(stdinReader, false))
			case 'L':
				out = append(out, readLine(stdinReader, true))
			case 'a':
				// "a" never fails: at EOF it yields the empty string.
				rest, _ := io.ReadAll(stdinReader)
				out = append(out, string(rest))
			case 'n':
				out = append(out, readNumeral(stdinReader))
			default:
				panic(Errorf("bad argument #%d to 'io.read' (invalid format '%s')", i+1, spec))
			}
		}
		return out
	})
	return t
}

// readLine reads through the next newline. keepEol distinguishes "L" (newline
// retained) from "l" (stripped). Returns nil at EOF with nothing read, which
// is what terminates `for line in io.read` style loops.
func readLine(br *bufio.Reader, keepEol bool) Value {
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		return nil
	}
	if keepEol {
		return line
	}
	return strings.TrimRight(line, "\r\n")
}

// readCount reads exactly n bytes. n == 0 is Lua's EOF probe: it returns ""
// when more input exists and nil when it does not.
func readCount(br *bufio.Reader, n int64) Value {
	if n < 0 {
		panic(Errorf("bad argument to 'io.read' (invalid format)"))
	}
	if n == 0 {
		if _, err := br.Peek(1); err != nil {
			return nil
		}
		return ""
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(br, buf)
	if got == 0 && err != nil {
		return nil
	}
	// A short read at EOF returns what was available, as in reference Lua.
	return string(buf[:got])
}

// readNumeral implements the "n" format: skip leading space, then consume the
// longest prefix that still looks like a Lua numeral and convert it. Returns
// nil (Lua's `fail`) when the input does not start with a number, leaving the
// offending bytes unread so the caller can retry with another format.
func readNumeral(br *bufio.Reader) Value {
	for {
		c, err := br.ReadByte()
		if err != nil {
			return nil
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '\v' && c != '\f' {
			_ = br.UnreadByte()
			break
		}
	}
	var lit []byte
	for {
		c, err := br.ReadByte()
		if err != nil {
			break
		}
		if isNumeralByte(c) {
			lit = append(lit, c)
			continue
		}
		_ = br.UnreadByte()
		break
	}
	if len(lit) == 0 {
		return nil
	}
	// Reuse the runtime's own numeral parser so io.read("n") accepts exactly
	// what a numeric literal in source would (hex, exponents, integer subtype).
	if i, f, isInt, ok := ToNumber(string(lit)); ok {
		if isInt {
			return i
		}
		return f
	}
	return nil
}

// isNumeralByte reports whether c can appear in a Lua numeral. Deliberately
// permissive (it accepts "0x1p+4" and also nonsense like "1e+x"): the
// conversion step is what decides validity.
func isNumeralByte(c byte) bool {
	switch {
	case c >= '0' && c <= '9',
		c >= 'a' && c <= 'f', c >= 'A' && c <= 'F',
		c == 'x' || c == 'X', c == 'p' || c == 'P',
		c == '.', c == '+', c == '-':
		return true
	}
	return false
}

// Argument-validation helpers (NumArg, FloatArg, IntArg, StringArg,
// describeBadArg, plus TableArg/ClosureArg/etc.) live in stdlib_args.go.
