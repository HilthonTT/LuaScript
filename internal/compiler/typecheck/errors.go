package typecheck

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TypeError is one entry in the checker's error list. The checker
// accumulates errors instead of bailing on the first one, so users see
// the full picture per compile.
type TypeError struct {
	Line    int
	Code    string // short stable identifier for tooling, e.g. "incompat-assign"
	Message string // human-readable, single-line
	Got     *Type  // optional — populated for assignability errors
	Want    *Type  // optional — populated for assignability errors
}

// Format renders the error in Luau-style:
//
//	"Type 'string' could not be converted into 'number' at line 5"
//
// or, when only Message is set, just the bare message + line.
func (e *TypeError) Format() string {
	if e.Got != nil && e.Want != nil {
		return fmt.Sprintf("Type %s could not be converted into %s at line %d",
			quoteType(e.Got), quoteType(e.Want), e.Line)
	}
	return fmt.Sprintf("%s at line %d", e.Message, e.Line)
}

// quoteType renders a type for an error message. Ordinary types are
// double-quoted; a string singleton already carries quotes of its own
// (`"read"`), and double-quoting it again would produce the unreadable
// `"\"read\""`, so those are wrapped in Luau-style single quotes instead.
func quoteType(t *Type) string {
	s := t.String()
	if strings.Contains(s, "\"") {
		return "'" + s + "'"
	}
	return strconv.Quote(s)
}

// TypeErrors is the aggregate error type returned by Check. It satisfies
// `error` so the compiler glue can return it through the standard error
// channel.
type TypeErrors struct {
	Errors []TypeError
}

func (te *TypeErrors) Error() string {
	if te == nil || len(te.Errors) == 0 {
		return "no type errors"
	}
	parts := make([]string, len(te.Errors))
	for i, e := range te.Errors {
		parts[i] = e.Format()
	}
	return strings.Join(parts, "\n")
}

// sortByLine sorts errors by source line so output is stable.
func sortByLine(errs []TypeError) {
	sort.SliceStable(errs, func(i, j int) bool {
		return errs[i].Line < errs[j].Line
	})
}

// errf records a non-assignability error on the checker's error list.
func (c *checker) errf(line int, code, format string, args ...any) {
	c.errors = append(c.errors, TypeError{
		Line:    line,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	})
}

// errAssign records a "type X cannot flow to type Y" error with the two
// types attached for the formatter.
func (c *checker) errAssign(line int, got, want *Type) {
	c.errors = append(c.errors, TypeError{
		Line: line,
		Code: "incompat-assign",
		Got:  reportedGot(got, want),
		Want: want,
	})
}

// reportedGot picks how precisely to describe the offending type. When the
// target mentions a singleton, the exact value is the whole point of the
// error and is reported verbatim:
//
//	Type "\"append\"" could not be converted into "\"read\" | \"write\""
//
// When the target is an ordinary primitive the singleton adds noise — the
// programmer already sees the value in the source — so it widens to the base
// primitive and the message reads the way it always has:
//
//	Type "string" could not be converted into "number"
func reportedGot(got, want *Type) *Type {
	if mentionsLiteral(want) {
		return got
	}
	return widen(got)
}

// mentionsLiteral reports whether `t` is a singleton or a union with one.
func mentionsLiteral(t *Type) bool {
	if t == nil {
		return false
	}
	if t.Kind == KindLiteral {
		return true
	}
	if t.Kind == KindUnion {
		for _, m := range t.Union {
			if m.Kind == KindLiteral {
				return true
			}
		}
	}
	return false
}
