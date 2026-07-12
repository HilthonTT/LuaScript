package formatter

// Doc is a node in the pretty-printer's intermediate representation. The
// shape follows Lindig's "Strictly Pretty" (2000) and Prettier's IR: a small
// algebra of layout primitives that the renderer walks with a remaining-
// width counter, deciding for each Group whether to lay it out flat (no
// breaks) or broken (Lines become newline+indent).
//
// Why a Doc IR rather than direct string emission: emitters can describe
// "this is a group of pieces, glue them with a soft break, and break the
// whole thing only if it doesn't fit" without manual width arithmetic.
// Long argument lists, long table constructors, and chained method calls
// then break correctly without per-node code.
type Doc interface{ isDoc() }

type (
	// docNil is the empty document.
	docNil struct{}

	// docText is a literal string with no break behavior. The renderer
	// assumes Text contains no embedded newlines — use Line / HardLine for
	// line breaks.
	docText struct{ s string }

	// docLine is a soft line break: rendered as a space in flat mode, or as
	// "\n" + current indent in break mode.
	docLine struct{}

	// docSoftLine is a soft break that flattens to *nothing* (not a space).
	// Useful between the opening delimiter and the first item of a group.
	docSoftLine struct{}

	// docHardLine forces a line break regardless of fit.
	docHardLine struct{}

	// docNest increases the indent level by N for its Inner doc.
	docNest struct {
		n     int
		inner Doc
	}

	// docConcat is sequential composition.
	docConcat struct{ parts []Doc }

	// docGroup is the placement decision boundary: the renderer first tries
	// to render Inner with all enclosed Lines flattened; if the result fits
	// in the remaining width, it commits to flat mode, otherwise it falls
	// back to break mode and Lines become real newlines.
	docGroup struct{ inner Doc }
)

func (docNil) isDoc()      {}
func (docText) isDoc()     {}
func (docLine) isDoc()     {}
func (docSoftLine) isDoc() {}
func (docHardLine) isDoc() {}
func (docNest) isDoc()     {}
func (docConcat) isDoc()   {}
func (docGroup) isDoc()    {}

// Constructors. These are tiny on purpose: emit code reads more naturally
// with short names (text, line, group, nest, concat).

func nilDoc() Doc {
	return docNil{}
}

func text(s string) Doc {
	return docText{s}
}

func line() Doc {
	return docLine{}
}

func softLine() Doc {
	return docSoftLine{}
}

func hardLine() Doc {
	return docHardLine{}
}

func nest(n int, d Doc) Doc {
	return docNest{n: n, inner: d}
}

func group(d Doc) Doc {
	return docGroup{inner: d}
}

// concat composes documents end-to-end. Nil-document arguments are dropped
// so emit sites can write `concat(maybeNil, ..., last)` without wrapping
// every optional piece in an `if`.
func concat(parts ...Doc) Doc {
	flat := make([]Doc, 0, len(parts))
	for _, p := range parts {
		if p == nil {
			continue
		}
		if _, ok := p.(docNil); ok {
			continue
		}
		if c, ok := p.(docConcat); ok {
			flat = append(flat, c.parts...)
			continue
		}
		flat = append(flat, p)
	}
	if len(flat) == 0 {
		return docNil{}
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return docConcat{parts: flat}
}

// join interleaves a separator between docs. `join(text(", "), a, b, c)` →
// `a, b, c`. Empty input returns nilDoc.
func join(sep Doc, parts ...Doc) Doc {
	if len(parts) == 0 {
		return nilDoc()
	}
	out := make([]Doc, 0, len(parts)*2-1)
	for i, p := range parts {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, p)
	}
	return concat(out...)
}
