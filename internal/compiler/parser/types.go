package parser

import "fmt"

// Source mapping to map from the source code of the template to the
// in-memory representation.
type Position struct {
	Index int64
	Line  uint32
	Col   uint32
}

func (p *Position) String() string {
	return fmt.Sprintf("line %d, col %d (index %d)", p.Line, p.Col, p.Index)
}

// NewPosition initialises a position.
func NewPosition(index int64, line, col uint32) Position {
	return Position{
		Index: index,
		Line:  line,
		Col:   col,
	}
}

// Range of text within a file.
type Range struct {
	From Position
	To   Position
}

// NewRange creates a Range expression.
func NewRange(from, to Position) Range {
	return Range{
		From: Position{
			Index: int64(from.Index),
			Line:  uint32(from.Line),
			Col:   uint32(from.Col),
		},
		To: Position{
			Index: int64(to.Index),
			Line:  uint32(to.Line),
			Col:   uint32(to.Col),
		},
	}
}

// Expression containing Go code.
type Expression struct {
	Value string
	Range Range
}

// NewExpression creates a Go expression.
func NewExpression(value string, from, to Position) Expression {
	return Expression{
		Value: value,
		Range: Range{
			From: Position{
				Index: int64(from.Index),
				Line:  uint32(from.Line),
				Col:   uint32(from.Col),
			},
			To: Position{
				Index: int64(to.Index),
				Line:  uint32(to.Line),
				Col:   uint32(to.Col),
			},
		},
	}
}
