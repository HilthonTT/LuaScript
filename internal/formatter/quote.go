package formatter

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// quoteLua renders s as a double-quoted LuaScript string literal. It must only
// use escapes the lexer understands: strconv.Quote emits Go's \uXXXX form,
// which the lexer reads as a bare `u` followed by text, silently changing the
// string's contents.
func quoteLua(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= utf8.RuneSelf && !unicode.IsPrint(r):
			fmt.Fprintf(&b, `\u{%X}`, r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}
