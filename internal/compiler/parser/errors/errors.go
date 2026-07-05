// Package errors defines the typed parser error used throughout the
// luascript front-end. Lua 5.4 has a small, focused set of failure modes,
// so the categories here are intentionally narrow.
package errors

import "fmt"

// Enums for different kinds of syntax errors.
const (
	_ = iota

	// EndOfFileError represents a normal EOF reached while expecting more input.
	EndOfFileError
	// UnexpectedTokenError means a token did not match what the grammar expected.
	UnexpectedTokenError
	// UnexpectedEndError means an `end` keyword appeared with nothing to close
	// (used by the REPL to detect "still typing" vs. "real syntax error").
	UnexpectedEndError
	// InvalidAssignmentError means the LHS of an `=` is not a valid var.
	InvalidAssignmentError
	// SyntaxError is the catch-all for grammatical mistakes.
	SyntaxError
)

// Error represents a parser failure with a human-readable message and a
// machine-readable category.
type Error struct {
	Message string
	ErrType int
}

// Error implements the error interface, returning the message verbatim.
// Having this here lets callers return *Error directly as an `error` while
// preserving the typed-introspection helpers (IsEOF etc.) for the REPL.
func (e *Error) Error() string { return e.Message }

// IsEOF reports whether the error is an end-of-file marker.
func (e *Error) IsEOF() bool { return e.ErrType == EndOfFileError }

// IsUnexpectedEnd reports whether the error is an unmatched `end`.
func (e *Error) IsUnexpectedEnd() bool { return e.ErrType == UnexpectedEndError }

// IsUnexpectedToken reports whether the error is the generic "unexpected
// token" category.
func (e *Error) IsUnexpectedToken() bool { return e.ErrType == UnexpectedTokenError }

// IsUnexpectedEmptyLine reports whether the error is an unmatched `end`
// arising from REPL input with no preceding statements.
func (e *Error) IsUnexpectedEmptyLine(stmtCount int) bool {
	return e.IsUnexpectedEnd() && stmtCount == 0
}

// InitError builds an Error from a message and category.
func InitError(msg string, errType int) *Error {
	return &Error{Message: msg, ErrType: errType}
}

// NewTypeParsingError reports a literal that could not be converted to its
// declared numeric type (e.g. an integer literal too large for int64).
func NewTypeParsingError(tokenLiteral, targetType string, line int) *Error {
	msg := fmt.Sprintf("could not parse %q as %s. Line: %d", tokenLiteral, targetType, line)
	return InitError(msg, SyntaxError)
}
