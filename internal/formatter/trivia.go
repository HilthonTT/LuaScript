package formatter

import (
	"sort"
	"strings"
)

type TriviaKind int

const (
	LineComment TriviaKind = iota
	LongComment
	BlankLine
)

type Trivia struct {
	Line    int
	EndLine int
	Col     int
	Kind    TriviaKind
	Text    string
}

func scanTrivia(src string) []Trivia {
	var out []Trivia
	line := 1
	col := 1
	i := 0
	n := len(src)

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

		if c == '-' && i+1 < n && src[i+1] == '-' {
			startLine := line
			startCol := col
			start := i
			advance(src[i])
			i++
			advance(src[i])
			i++
			if i+1 < n && src[i] == '[' && src[i+1] == '[' {
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
					EndLine: line,
					Col:     startCol,
					Kind:    LongComment,
					Text:    src[start:i],
				})
				continue
			}
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

		if col == 1 && c != '\n' && (c == ' ' || c == '\t' || c == '\r') {
			j := i
			for j < n && src[j] != '\n' {
				if src[j] != ' ' && src[j] != '\t' && src[j] != '\r' {
					break
				}
				j++
			}
			if j >= n || src[j] == '\n' {
				if emittedBlankFor != line-1 {
					out = append(out, Trivia{Line: line, EndLine: line, Col: 1, Kind: BlankLine})
				}
				emittedBlankFor = line
			}
		} else if col == 1 && c == '\n' {
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

func normalizeLineComment(text string) string {
	t := strings.TrimRight(text, " \t")
	if strings.HasPrefix(t, "--!") {
		return t
	}
	body := strings.TrimPrefix(t, "--")
	if body == "" || strings.TrimLeft(body, "-") == "" {
		return t
	}
	if strings.HasPrefix(body, " ") {
		return t
	}
	return "-- " + body
}
