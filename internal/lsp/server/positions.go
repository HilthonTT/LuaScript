package server

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

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
	if end == start {
		end = start + 1
	}
	l := uint32(line1 - 1)
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: uint32(start)},
		End:   protocol.Position{Line: l, Character: uint32(end)},
	}
}

var lineColRe = regexp.MustCompile(`at line (\d+)(?:, column (\d+))?`)

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
