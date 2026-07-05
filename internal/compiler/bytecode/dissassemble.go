package bytecode

import (
	"fmt"
	"strings"
)

func Disassemble(sets []*InstructionSet) string {
	var out strings.Builder
	for i, s := range sets {
		if i > 0 {
			out.WriteByte('\n')
		}
		disassembleSet(&out, s, 0)
	}
	return out.String()
}

func disassembleSet(out *strings.Builder, is *InstructionSet, depth int) {
	indent := strings.Repeat("  ", depth)

	writeHeader(out, is, indent)
	for _, in := range is.Instructions {
		writeInstruction(out, in, indent)
	}

	for _, proto := range is.Protos {
		out.WriteByte('\n')
		disassembleSet(out, proto, depth+1)
	}
}

func writeHeader(out *strings.Builder, is *InstructionSet, indent string) {
	name := is.Name()
	if name == "" {
		name = "<anonymous>"
	}

	// Lua-style "n+" notation: a trailing '+' means vararg.
	varargMark := ""
	if is.IsVararg {
		varargMark = "+"
	}

	fmt.Fprintf(out,
		"%s%s %s (%d%s params, %d locals, %d upvalues, %d instructions)\n",
		indent, is.Type(), name,
		is.NumParams, varargMark,
		is.NumLocals,
		len(is.Upvalues),
		len(is.Instructions),
	)

	for i, uv := range is.Upvalues {
		src := "upvalue"
		if uv.InStack {
			src = "local"
		}
		fmt.Fprintf(out, "%s  upvalue[%d] %s (%s %d)\n",
			indent, i, uv.Name, src, uv.Index)
	}
}

func writeInstruction(out *strings.Builder, in *Instruction, indent string) {
	fmt.Fprintf(out, "%s  %4d  [src %4d]  %-16s %s\n",
		indent,
		in.Line(),
		in.SourceLine(),
		in.ActionName(),
		formatParams(in.Params),
	)
}

func formatParams(params []any) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, p := range params {
		switch v := p.(type) {
		case *anchor:
			// Resolve forward-jump anchors to their target PC.
			parts = append(parts, fmt.Sprintf("-> %d", v.line))
		case string:
			// Quote strings so embedded whitespace/newlines stay legible.
			parts = append(parts, fmt.Sprintf("%q", v))
		default:
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, " ")
}
