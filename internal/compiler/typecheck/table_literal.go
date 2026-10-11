package typecheck

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/v2/internal/compiler/ast"
)

// typeOfExpressionExpecting types e like typeOfExpression, except that a
// table constructor is checked against want (contextual typing). Table
// constructors still type as `any` afterwards, so callers' own
// assignability checks never report the same problem twice.
func (c *checker) typeOfExpressionExpecting(e ast.Expression, want *Type) *Type {
	return c.typeOfExpressionAt(e, want, "")
}

func (c *checker) typeOfExpressionAt(e ast.Expression, want *Type, path string) *Type {
	switch n := e.(type) {
	case *ast.ParenExpression:
		return c.typeOfExpressionAt(n.Inner, want, path)
	case *ast.TableConstructor:
		if want != nil {
			c.checkTableLiteral(n, want, path)
			return anyT
		}
	}
	return c.typeOfExpression(e)
}

// expectedFor returns expected[i], or nil when there is none.
func expectedFor(expected []*Type, i int) *Type {
	if i < len(expected) {
		return expected[i]
	}
	return nil
}

// checkTableLiteral checks every entry of a table constructor against the
// type the context expects. path names the literal for nested errors
// ("" at the top level, "address" or "[2]" further in).
func (c *checker) checkTableLiteral(t *ast.TableConstructor, want *Type, path string) {
	want = boundOf(want)
	if want == nil || want.Kind == KindAny || want.Kind == KindUnknown || want.Kind == KindTypeParam {
		c.typeOfTableConstructor(t)
		return
	}

	candidates, open := tableCandidates(want)
	if open {
		c.typeOfTableConstructor(t)
		return
	}
	switch len(candidates) {
	case 0:
		got := c.inferTableLiteral(t)
		c.errAssignAt(t.Line(), got, want, path)
		return
	case 1:
		c.checkTableLiteralShape(t, candidates[0], path)
		return
	}

	// Several table members (a discriminated union, say): the literal must
	// fit one of them. Try each without reporting, then commit to the first
	// that fits.
	for _, cand := range candidates {
		if c.countErrors(func() { c.checkTableLiteralShape(t, cand, path) }) == 0 {
			c.checkTableLiteralShape(t, cand, path)
			return
		}
	}
	got := c.inferTableLiteral(t)
	c.errAssignAt(t.Line(), got, want, path)
}

// tableCandidates lists the table members of want. open is true when want
// admits a table without a shape to check against (any, unknown, a type
// parameter).
func tableCandidates(want *Type) (out []*Type, open bool) {
	members := []*Type{want}
	if want.Kind == KindUnion {
		members = want.Union
	}
	for _, m := range members {
		m = boundOf(m)
		switch m.Kind {
		case KindAny, KindUnknown, KindTypeParam:
			return nil, true
		case KindTable:
			if m.Table == nil {
				return nil, true
			}
			out = append(out, m)
		}
	}
	return out, false
}

// countErrors runs fn and returns how many type errors it would have
// reported, discarding them.
func (c *checker) countErrors(fn func()) int {
	saved := c.errors
	c.errors = nil
	fn()
	n := len(c.errors)
	c.errors = saved
	return n
}

func (c *checker) checkTableLiteralShape(t *ast.TableConstructor, want *Type, path string) {
	shape := want.Table
	fields := make(map[string]*Type, len(shape.Fields))
	for _, f := range shape.Fields {
		fields[f.Key] = f.Type
	}
	// An empty `{}` type accepts any table.
	anyShape := len(shape.Fields) == 0 && shape.Indexer == nil
	idx := shape.Indexer

	provided := map[string]bool{}
	hasSpread := false
	position := 0

	for _, f := range t.Fields {
		if f.IsSpread {
			hasSpread = true
			spread := boundOf(c.typeOfExpression(f.Value))
			if spread.Kind != KindAny && spread.Kind != KindTable && spread.Kind != KindUnknown {
				c.errf(f.Value.Line(), "spread-non-table",
					"cannot spread a value of type %q into a table", spread.String())
			}
			continue
		}

		name, named := literalFieldName(f)
		switch {
		case named:
			provided[name] = true
			sub := joinPath(path, name)
			if ft, ok := fields[name]; ok {
				c.checkLiteralValue(f.Value, ft, sub)
				continue
			}
			if idx != nil && assignable(stringT, idx.Key) {
				c.checkLiteralValue(f.Value, idx.Value, sub)
				continue
			}
			c.walkExpressionDiscard(f.Value)
			if !anyShape {
				c.errf(f.Value.Line(), "extra-field",
					"table literal has field %q, which type %q does not declare",
					sub, want.String())
			}

		case f.Key == nil:
			position++
			sub := fmt.Sprintf("%s[%d]", path, position)
			if idx != nil && assignable(numberT, idx.Key) {
				c.checkLiteralValue(f.Value, idx.Value, sub)
				continue
			}
			c.walkExpressionDiscard(f.Value)
			if !anyShape {
				c.errf(f.Value.Line(), "extra-field",
					"table literal has positional entry %s, but type %q has no array part",
					sub, want.String())
			}

		default:
			key := c.typeOfExpression(f.Key)
			if idx == nil {
				c.walkExpressionDiscard(f.Value)
				if !anyShape && key.Kind != KindAny {
					c.errf(f.Value.Line(), "extra-field",
						"table literal has a [%s] key, but type %q has no indexer",
						widen(key).String(), want.String())
				}
				continue
			}
			if !assignable(key, idx.Key) {
				c.errAssignAt(f.Key.Line(), key, idx.Key, path+"[key]")
			}
			c.checkLiteralValue(f.Value, idx.Value, path+"[]")
		}
	}

	if hasSpread || len(t.Fields) == 0 {
		// A spread may supply any field; an empty `{}` is the usual
		// "build it up field by field" idiom.
		return
	}
	impl := c.implMembers[baseAliasName(want.AliasName)]
	for _, f := range shape.Fields {
		if provided[f.Key] || assignable(nilT, f.Type) {
			continue
		}
		if _, isImpl := impl[f.Key]; isImpl {
			continue
		}
		c.errf(t.Line(), "missing-field",
			"table literal is missing required field %q of type %q",
			joinPath(path, f.Key), f.Type.String())
	}
}

func (c *checker) checkLiteralValue(v ast.Expression, want *Type, path string) {
	got := c.typeOfExpressionAt(v, want, path)
	if !assignable(got, want) {
		c.errAssignAt(v.Line(), got, want, path)
	}
}

// inferTableLiteral walks t and returns a structural type describing it,
// used only to describe a literal in an error message.
func (c *checker) inferTableLiteral(t *ast.TableConstructor) *Type {
	var fields []TableField
	var elems []*Type
	for _, f := range t.Fields {
		if f.IsSpread {
			c.walkExpressionDiscard(f.Value)
			continue
		}
		var vt *Type
		if tc, ok := f.Value.(*ast.TableConstructor); ok {
			vt = c.inferTableLiteral(tc)
		} else {
			vt = widen(c.typeOfExpression(f.Value))
		}
		if name, ok := literalFieldName(f); ok {
			fields = upsertField(fields, name, vt)
			continue
		}
		if f.Key != nil {
			c.walkExpressionDiscard(f.Key)
			continue
		}
		elems = append(elems, vt)
	}
	var idx *Indexer
	if len(elems) > 0 {
		idx = &Indexer{Key: numberT, Value: NewUnion(elems...)}
	}
	return NewTable(fields, idx)
}

func literalFieldName(f ast.TableField) (string, bool) {
	if f.Key == nil {
		return "", false
	}
	if id, ok := f.Key.(*ast.Identifier); ok && !f.IsBracketed {
		return id.Name, true
	}
	if s, ok := f.Key.(*ast.StringLiteral); ok {
		return s.Value, true
	}
	return "", false
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func baseAliasName(name string) string {
	if i := strings.IndexByte(name, '<'); i >= 0 {
		return name[:i]
	}
	return name
}

// errAssignAt is errAssign with the literal path the mismatch sits at.
func (c *checker) errAssignAt(line int, got, want *Type, path string) {
	if c.silent {
		return
	}
	c.errors = append(c.errors, TypeError{
		Line:  line,
		Code:  "incompat-assign",
		Got:   reportedGot(got, want),
		Want:  want,
		Field: path,
	})
}

// contextualParam is the type a table-constructor argument at index i is
// checked against. Generic functions are skipped: their parameters mention
// type variables that are only solved from the arguments themselves.
func contextualParam(fn *FunctionShape, i int) *Type {
	if fn == nil || fn.Struct != nil || len(fn.TypeParams) > 0 {
		return nil
	}
	if i < len(fn.Params) {
		return fn.Params[i]
	}
	if fn.IsVararg {
		return fn.VarargType
	}
	return nil
}

// assignTargetType is the declared type of an assignment target, or nil
// when it has none worth checking a table literal against. It reports
// nothing: walkAssignStatement does its own diagnostics.
func (c *checker) assignTargetType(t ast.Expression) *Type {
	switch tgt := t.(type) {
	case *ast.Identifier:
		if declared, ok := c.env.lookupDeclared(tgt.Name); ok {
			return declared
		}
	case *ast.IndexExpression:
		name, ok := staticIndexName(tgt)
		if !ok {
			return nil
		}
		prev := c.silent
		c.silent = true
		base := boundOf(c.typeOfExpression(tgt.Object))
		c.silent = prev
		if base == nil || base.Kind != KindTable || base.Table == nil {
			return nil
		}
		for _, f := range base.Table.Fields {
			if f.Key == name {
				return f.Type
			}
		}
		if idx := base.Table.Indexer; idx != nil && assignable(stringT, idx.Key) {
			return idx.Value
		}
	}
	return nil
}
