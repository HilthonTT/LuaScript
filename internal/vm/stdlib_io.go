package vm

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

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
			if n, ok := ToInteger(args[i]); ok {
				out = append(out, readCount(stdinReader, n))
				continue
			}
			spec := strings.TrimPrefix(StringArg("io.read", i+1, args), "*")
			if spec == "" {
				panic(Errorf("bad argument #%d to 'io.read' (invalid format)", i+1))
			}
			switch spec[0] {
			case 'l':
				out = append(out, readLine(stdinReader, false))
			case 'L':
				out = append(out, readLine(stdinReader, true))
			case 'a':
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
	return string(buf[:got])
}

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
	if i, f, isInt, ok := ToNumber(string(lit)); ok {
		if isInt {
			return i
		}
		return f
	}
	return nil
}

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
