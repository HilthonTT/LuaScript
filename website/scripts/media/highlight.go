package main

import (
	"image/color"
	"strings"
	"unicode"
)

var keywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true, "end": true,
	"for": true, "function": true, "goto": true, "if": true, "in": true, "local": true,
	"not": true, "or": true, "repeat": true, "return": true, "then": true, "until": true,
	"while": true, "continue": true, "match": true, "try": true, "catch": true,
	"throw": true, "defer": true, "type": true, "struct": true, "enum": true,
	"impl": true, "interface": true,
}

var constants = map[string]bool{"nil": true, "true": true, "false": true, "self": true}

var primitives = map[string]bool{
	"number": true, "string": true, "boolean": true, "any": true, "never": true,
	"unknown": true, "thread": true,
}

var builtins = map[string]bool{
	"print": true, "require": true, "pairs": true, "ipairs": true, "tostring": true,
	"tonumber": true, "pcall": true, "error": true, "assert": true, "typeof": true,
	"setmetatable": true, "math": true, "string": true, "table": true, "os": true,
}

// highlight colors one line of LuaScript. It is a line-local lexer, which is
// all the short demo programs need.
func highlight(src string) []Span {
	var out []Span
	emit := func(s string, c color.RGBA) {
		if s != "" {
			out = append(out, Span{Text: s, FG: c})
		}
	}
	rs := []rune(src)
	prevSig := "" // previous significant token, to spot type positions
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == '-' && i+1 < len(rs) && rs[i+1] == '-':
			emit(string(rs[i:]), cDim)
			return out
		case r == '"' || r == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != r {
				if rs[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(rs))
			emit(string(rs[i:j]), cGreen)
			i, prevSig = j, "str"
		case r == '`':
			j := i + 1
			start := i
			for j < len(rs) && rs[j] != '`' {
				if rs[j] == '{' {
					emit(string(rs[start:j+1]), cGreen)
					k := j + 1
					for k < len(rs) && rs[k] != '}' {
						k++
					}
					out = append(out, highlight(string(rs[j+1:k]))...)
					start, j = k, k
					continue
				}
				j++
			}
			j = min(j+1, len(rs))
			emit(string(rs[start:j]), cGreen)
			i, prevSig = j, "str"
		case unicode.IsDigit(r):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'x' || unicode.Is(unicode.ASCII_Hex_Digit, rs[j])) {
				j++
			}
			emit(string(rs[i:j]), cOrange)
			i, prevSig = j, "num"
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			word := string(rs[i:j])
			rest := strings.TrimLeft(string(rs[j:]), " ")
			c := cText
			switch {
			case keywords[word]:
				c = cPurple
			case constants[word]:
				c = cOrange
			case primitives[word] && (prevSig == ":" || prevSig == "|" || prevSig == "{" || prevSig == "(" || prevSig == ","):
				c = cType
			case unicode.IsUpper(rs[i]):
				c = cType
			case prevSig == "function" || strings.HasPrefix(rest, "(") && !keywords[word]:
				c = cBlue
				if builtins[word] {
					c = cCyan
				}
			case builtins[word]:
				c = cCyan
			}
			emit(word, c)
			i, prevSig = j, word
		case r == ' ' || r == '\t':
			j := i
			for j < len(rs) && (rs[j] == ' ' || rs[j] == '\t') {
				j++
			}
			emit(string(rs[i:j]), cText)
			i = j
		default:
			c := cDim
			if strings.ContainsRune("+-*/%^#=<>~|&?.", r) {
				c = cMagenta
			}
			if r == ':' || r == ',' || r == '.' {
				c = cDim
			}
			emit(string(r), c)
			i, prevSig = i+1, string(r)
		}
	}
	return out
}
