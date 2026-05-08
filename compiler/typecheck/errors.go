package typecheck

import (
	"fmt"
	"sort"
	"strings"
)

// TypeError is one entry in the checker's error list. The checker
// accumulates errors instead of bailing on the first one, so users see
// the full picture per compile.
type TypeError struct {
	Line    int
	Code    string  // short stable identifier for tooling, e.g. "incompat-assign"
	Message string  // human-readable, single-line
	Got     *Type   // optional — populated for assignability errors
	Want    *Type   // optional — populated for assignability errors
}

// Format renders the error in Luau-style:
//
//	"Type 'string' could not be converted into 'number' at line 5"
//
// or, when only Message is set, just the bare message + line.
func (e *TypeError) Format() string {
	if e.Got != nil && e.Want != nil {
		return fmt.Sprintf("Type %q could not be converted into %q at line %d",
			e.Got.String(), e.Want.String(), e.Line)
	}
	return fmt.Sprintf("%s at line %d", e.Message, e.Line)
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
