package typecheck

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type TypeError struct {
	Line    int
	Code    string
	Message string
	Got     *Type
	Want    *Type
}

func (e *TypeError) Format() string {
	if e.Got != nil && e.Want != nil {
		return fmt.Sprintf("Type %s could not be converted into %s at line %d",
			quoteType(e.Got), quoteType(e.Want), e.Line)
	}
	return fmt.Sprintf("%s at line %d", e.Message, e.Line)
}

func quoteType(t *Type) string {
	s := t.String()
	if strings.Contains(s, "\"") {
		return "'" + s + "'"
	}
	return strconv.Quote(s)
}

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

func sortByLine(errs []TypeError) {
	sort.SliceStable(errs, func(i, j int) bool {
		return errs[i].Line < errs[j].Line
	})
}

func (c *checker) errf(line int, code, format string, args ...any) {
	c.errors = append(c.errors, TypeError{
		Line:    line,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	})
}

func (c *checker) errAssign(line int, got, want *Type) {
	c.errors = append(c.errors, TypeError{
		Line: line,
		Code: "incompat-assign",
		Got:  reportedGot(got, want),
		Want: want,
	})
}

func reportedGot(got, want *Type) *Type {
	if mentionsLiteral(want) {
		return got
	}
	return widen(got)
}

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
