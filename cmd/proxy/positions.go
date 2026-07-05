package proxy

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/lsp/protocol"
)

// The luascript front-end reports positions as 1-based (line, column) while
// LSP uses 0-based (line, character). Character offsets are technically
// UTF-16 code units in LSP, but luascript source is overwhelmingly ASCII, so
// a rune-count approximation is close enough for v1 and avoids the cost of a
// full UTF-16 re-encode on every diagnostic.

// lineWidth returns the number of characters on the given 1-based source line.
func lineWidth(src string, line1 int) int {
	if line1 < 1 {
		return 0
	}
	lines := strings.Split(src, "\n")
	idx := line1 - 1
	if idx < 0 || idx >= len(lines) {
		return 0
	}
	return len([]rune(strings.TrimRight(lines[idx], "\r")))
}

// wholeLine returns an LSP range spanning an entire 1-based source line. Used
// for diagnostics that carry a line but no column (typecheck / analyze).
func wholeLine(src string, line1 int) protocol.Range {
	if line1 < 1 {
		line1 = 1
	}
	l := uint32(line1 - 1)
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: 0},
		End:   protocol.Position{Line: l, Character: uint32(lineWidth(src, line1))},
	}
}

// spanFrom returns a range from a 1-based (line, col) to the end of that line.
// Used for parser errors, which report a precise start column.
func spanFrom(src string, line1, col1 int) protocol.Range {
	if line1 < 1 {
		line1 = 1
	}
	width := lineWidth(src, line1)
	start := col1 - 1
	if start < 0 {
		start = 0
	}
	if start > width {
		start = width
	}
	end := width
	if end < start {
		end = start
	}
	// Guarantee a non-empty range so the squiggle is visible even on an empty
	// line or when the reported column sits past the last character.
	if end == start {
		end = start + 1
	}
	l := uint32(line1 - 1)
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: uint32(start)},
		End:   protocol.Position{Line: l, Character: uint32(end)},
	}
}

// lineColRe extracts the "at line N, column C" (column optional) suffix that
// the parser's errorAt helper appends to every syntax error message.
var lineColRe = regexp.MustCompile(`at line (\d+)(?:, column (\d+))?`)

// extractLineCol pulls a 1-based (line, col) out of a parser error message.
// Returns (1, 0) when no position is embedded (col 0 means "unknown column").
func extractLineCol(msg string) (line, col int) {
	m := lineColRe.FindStringSubmatch(msg)
	if m == nil {
		return 1, 0
	}
	line, _ = strconv.Atoi(m[1])
	if line < 1 {
		line = 1
	}
	if m[2] != "" {
		col, _ = strconv.Atoi(m[2])
	}
	return line, col
}

// offsetToPosition converts a byte offset into an LSP position. Used when
// mapping cursor positions supplied by the client back onto the document.
func offsetToPosition(src string, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line := 0
	col := 0
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return protocol.Position{Line: uint32(line), Character: uint32(col)}
}

// positionToOffset converts an LSP position into a byte offset into src.
func positionToOffset(src string, pos protocol.Position) int {
	line := uint32(0)
	col := uint32(0)
	for i := 0; i < len(src); i++ {
		if line == pos.Line && col == pos.Character {
			return i
		}
		if src[i] == '\n' {
			if line == pos.Line {
				return i
			}
			line++
			col = 0
			continue
		}
		col++
	}
	return len(src)
}
