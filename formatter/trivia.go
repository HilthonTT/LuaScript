package formatter

import (
	"sort"
	"strings"
)

// TriviaKind tags the flavor of a piece of trivia.
type TriviaKind int

const (
	// LineComment is `-- ...` to end of line.
	LineComment TriviaKind = iota
	// LongComment is `--[[ ... ]]` (may span lines).
	LongComment
	// BlankLine is a run of one-or-more blank lines, collapsed to one.
	BlankLine
)

// Trivia is a comment or a blank-line break recovered from the raw source.
// The Line range is 1-based and inclusive on both ends.
type Trivia struct {
	Line    int
	EndLine int
	Col     int // 1-based start column
	Kind    TriviaKind
	Text    string // For comments: the raw text including the leading "--".
}

// scanTrivia walks `src` and recovers comments and blank-line breaks.
//
// Why we don't drive this off the lexer: the existing lexer (compiler/lexer)
// discards comments inside absorbComment(). Reusing it would require a
// dedicated lexer mode, which is a larger blast radius than v1 warrants.
//
// The scanner is deliberately small and self-contained: it tracks string and
// long-string contexts so `--` inside a literal is not mistaken for a
// comment. It does NOT validate syntax — malformed input still falls through
// to the parser, which is the source of truth for errors.
func scanTrivia(src string) []Trivia {
	var out []Trivia
	line := 1
	col := 1
	i := 0
	n := len(src)

	// Track blank-line runs so a string of "\n\n\n\n" collapses to one
	// BlankLine entry on the first blank line in the run.
	emittedBlankFor := 0

	advance := func(c byte) {
		if c == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}

	for i < n {
		c := src[i]

		// Short string literal.
		if c == '"' || c == '\'' {
			quote := c
			advance(c)
			i++
			for i < n {
				ch := src[i]
				if ch == '\\' && i+1 < n {
					advance(ch)
					i++
					advance(src[i])
					i++
					continue
				}
				if ch == quote {
					advance(ch)
					i++
					break
				}
				advance(ch)
				i++
			}
			continue
		}

		// Long string literal: [[ ... ]] (no level for v1; matches lexer).
		if c == '[' && i+1 < n && src[i+1] == '[' {
			advance(src[i])
			i++
			advance(src[i])
			i++
			for i < n {
				if src[i] == ']' && i+1 < n && src[i+1] == ']' {
					advance(src[i])
					i++
					advance(src[i])
					i++
					break
				}
				advance(src[i])
				i++
			}
			continue
		}

		// Comment: -- ... or --[[ ... ]]
		if c == '-' && i+1 < n && src[i+1] == '-' {
			startLine := line
			startCol := col
			start := i
			advance(src[i])
			i++
			advance(src[i])
			i++
			if i+1 < n && src[i] == '[' && src[i+1] == '[' {
				// Long comment.
				advance(src[i])
				i++
				advance(src[i])
				i++
				for i < n {
					if src[i] == ']' && i+1 < n && src[i+1] == ']' {
						advance(src[i])
						i++
						advance(src[i])
						i++
						break
					}
					advance(src[i])
					i++
				}
				out = append(out, Trivia{
					Line:    startLine,
					EndLine: line, // line is now the line *after* the closing ]] if it was on its own; close enough for grouping
					Col:     startCol,
					Kind:    LongComment,
					Text:    src[start:i],
				})
				continue
			}
			// Line comment: consume to end of line, not past the newline.
			for i < n && src[i] != '\n' {
				advance(src[i])
				i++
			}
			out = append(out, Trivia{
				Line:    startLine,
				EndLine: startLine,
				Col:     startCol,
				Kind:    LineComment,
				Text:    src[start:i],
			})
			continue
		}

		// Blank-line detection: when we are at the start of a line (col == 1)
		// and the line contains only whitespace before its terminating '\n',
		// emit a BlankLine trivia for that line (once per run).
		if col == 1 && c != '\n' && (c == ' ' || c == '\t' || c == '\r') {
			// Look ahead on this line.
			j := i
			for j < n && src[j] != '\n' {
				if src[j] != ' ' && src[j] != '\t' && src[j] != '\r' {
					break
				}
				j++
			}
			if j >= n || src[j] == '\n' {
				// Whitespace-only line.
				if emittedBlankFor != line-1 {
					out = append(out, Trivia{Line: line, EndLine: line, Col: 1, Kind: BlankLine})
				}
				emittedBlankFor = line
			}
			// Fall through; advance one char.
		} else if col == 1 && c == '\n' {
			// Truly empty line.
			if emittedBlankFor != line-1 {
				out = append(out, Trivia{Line: line, EndLine: line, Col: 1, Kind: BlankLine})
			}
			emittedBlankFor = line
		}

		advance(c)
		i++
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out
}

// normalizeLineComment strips trailing spaces from a `-- foo   ` line so the
// emitted form is canonical, and ensures there is exactly one space after
// the leading `--` unless the comment is `--` / `---...` (separator-style)
// or a Luau mode directive (`--!strict`).
func normalizeLineComment(text string) string {
	t := strings.TrimRight(text, " \t")
	// Preserve mode directive verbatim.
	if strings.HasPrefix(t, "--!") {
		return t
	}
	// Preserve hyphen-only separators like `---` or `----`.
	body := strings.TrimPrefix(t, "--")
	if body == "" || strings.TrimLeft(body, "-") == "" {
		return t
	}
	if strings.HasPrefix(body, " ") {
		return t
	}
	return "-- " + body
}
