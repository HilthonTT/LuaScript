package errors

import "fmt"

const (
	_ = iota

	EndOfFileError
	UnexpectedTokenError
	UnexpectedEndError
	InvalidAssignmentError
	SyntaxError
)

type Error struct {
	Message string
	ErrType int
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) IsEOF() bool {
	return e.ErrType == EndOfFileError
}

func (e *Error) IsUnexpectedEnd() bool {
	return e.ErrType == UnexpectedEndError
}

func (e *Error) IsUnexpectedToken() bool {
	return e.ErrType == UnexpectedTokenError
}

func (e *Error) IsUnexpectedEmptyLine(stmtCount int) bool {
	return e.IsUnexpectedEnd() && stmtCount == 0
}

func InitError(msg string, errType int) *Error {
	return &Error{Message: msg, ErrType: errType}
}

func NewTypeParsingError(tokenLiteral, targetType string, line int) *Error {
	msg := fmt.Sprintf("could not parse %q as %s. Line: %d", tokenLiteral, targetType, line)
	return InitError(msg, SyntaxError)
}
