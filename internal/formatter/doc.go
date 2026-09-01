package formatter

type Doc interface{ isDoc() }

type (
	docNil struct{}

	docText struct{ s string }

	docLine struct{}

	docSoftLine struct{}

	docHardLine struct{}

	docNest struct {
		n     int
		inner Doc
	}

	docConcat struct{ parts []Doc }

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
