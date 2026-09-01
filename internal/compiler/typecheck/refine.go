package typecheck

import (
	"maps"
	"slices"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type refinement map[string]*Type

func (c *checker) refine(cond ast.Expression, positive bool) refinement {
	switch n := cond.(type) {
	case *ast.ParenExpression:
		return c.refine(n.Inner, positive)

	case *ast.UnaryExpression:
		if n.Op == "not" {
			return c.refine(n.Operand, !positive)
		}

	case *ast.Identifier:
		return c.refineTruthiness(n.Name, positive)

	case *ast.BinaryExpression:
		return c.refineBinary(n, positive)
	}
	return nil
}

func (c *checker) refineTruthiness(name string, positive bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	if positive {
		nt := removeKind(t, KindNil)
		if Same(nt, t) {
			return nil
		}
		return refinement{name: nt}
	}
	falsy := keepKinds(t, KindNil, KindBoolean)
	if falsy.Kind == KindNever || Same(falsy, t) {
		return nil
	}
	return refinement{name: falsy}
}

func (c *checker) refineBinary(n *ast.BinaryExpression, positive bool) refinement {
	switch n.Op {
	case "and":
		if positive {
			return mergeRefine(c.refine(n.Left, true), c.refine(n.Right, true))
		}
		return nil

	case "or":
		if !positive {
			return mergeRefine(c.refine(n.Left, false), c.refine(n.Right, false))
		}
		return nil

	case "==", "~=":
		eq := (n.Op == "==") == positive
		return c.refineEquality(n.Left, n.Right, eq)
	}
	return nil
}

func (c *checker) refineEquality(a, b ast.Expression, eq bool) refinement {
	if name, guard, ok := typeGuardTarget(a); ok && c.builtinInScope(guard) {
		if lit, ok := stringLitValue(b); ok {
			return c.refineTypeGuard(name, lit, eq)
		}
	}
	if name, guard, ok := typeGuardTarget(b); ok && c.builtinInScope(guard) {
		if lit, ok := stringLitValue(a); ok {
			return c.refineTypeGuard(name, lit, eq)
		}
	}
	if name, ok := identName(a); ok && isNilLiteral(b) {
		return c.refineNilGuard(name, eq)
	}
	if name, ok := identName(b); ok && isNilLiteral(a) {
		return c.refineNilGuard(name, eq)
	}
	if name, ok := identName(a); ok {
		if lit := c.staticLiteralType(b); lit != nil {
			return c.refineLiteralGuard(name, lit, eq)
		}
	}
	if name, ok := identName(b); ok {
		if lit := c.staticLiteralType(a); lit != nil {
			return c.refineLiteralGuard(name, lit, eq)
		}
	}
	return nil
}

func (c *checker) staticLiteralType(e ast.Expression) *Type {
	switch n := e.(type) {
	case *ast.ParenExpression:
		return c.staticLiteralType(n.Inner)
	case *ast.StringLiteral:
		return NewStringLiteral(n.Value, "")
	case *ast.IntegerLiteral:
		return NewNumberLiteral(float64(n.Value), "")
	case *ast.FloatLiteral:
		return NewNumberLiteral(n.Value, "")
	case *ast.BooleanLiteral:
		return NewBooleanLiteral(n.Value, "")
	case *ast.IndexExpression:
		base, ok := n.Object.(*ast.Identifier)
		if !ok {
			return nil
		}
		name, ok := staticIndexName(n)
		if !ok {
			return nil
		}
		bt, ok := c.env.lookup(base.Name)
		if !ok || bt == nil || bt.Kind != KindTable || bt.Table == nil {
			return nil
		}
		for _, f := range bt.Table.Fields {
			if f.Key == name && f.Type != nil && f.Type.Kind == KindLiteral {
				return f.Type
			}
		}
	}
	return nil
}

func (c *checker) refineLiteralGuard(name string, lit *Type, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToLiteral(t, lit)
	} else {
		nt = removeLiteral(t, lit)
	}
	if nt == nil || Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

func narrowToLiteral(t, lit *Type) *Type {
	if t == nil || lit == nil {
		return nil
	}
	switch t.Kind {
	case KindAny, KindUnknown:
		return lit
	case KindUnion:
		kept := make([]*Type, 0, 1)
		for _, m := range t.Union {
			if Same(m, lit) || (m.Kind != KindLiteral && m.Kind == baseKind(lit)) {
				kept = append(kept, lit)
				break
			}
		}
		return NewUnion(kept...)
	}
	if Same(t, lit) {
		return t
	}
	if t.Kind == baseKind(lit) {
		return lit
	}
	return neverT
}

func removeLiteral(t, lit *Type) *Type {
	if t == nil || lit == nil {
		return nil
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if !Same(m, lit) {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if Same(t, lit) {
		return neverT
	}
	return t
}

func (c *checker) refineTypeGuard(name, kindStr string, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	k, ok := kindForTypeString(kindStr)
	if !ok {
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToKind(t, k)
	} else {
		nt = removeKind(t, k)
	}
	if Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

func (c *checker) refineNilGuard(name string, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToKind(t, KindNil)
	} else {
		nt = removeKind(t, KindNil)
	}
	if Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

func (c *checker) applyRefinement(r refinement) {
	for name, t := range r {
		c.env.defineRefined(name, t)
	}
}

func mergeRefine(a, b refinement) refinement {
	switch {
	case len(a) == 0:
		return b
	case len(b) == 0:
		return a
	}
	out := make(refinement, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

func narrowToKind(t *Type, k Kind) *Type {
	if t == nil {
		return t
	}
	switch t.Kind {
	case KindAny, KindUnknown:
		if p := primitiveForKind(k); p != nil {
			return p
		}
		return t
	case KindUnion:
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if baseKind(m) == k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if baseKind(t) == k {
		return t
	}
	return neverT
}

func removeKind(t *Type, k Kind) *Type {
	if t == nil || t.Kind == KindAny || t.Kind == KindUnknown {
		return t
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if baseKind(m) != k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if baseKind(t) == k {
		return neverT
	}
	return t
}

func keepKinds(t *Type, kinds ...Kind) *Type {
	if t == nil || t.Kind == KindAny || t.Kind == KindUnknown {
		return t
	}
	want := func(k Kind) bool {
		return slices.Contains(kinds, k)
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if want(baseKind(m)) {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if want(baseKind(t)) {
		return t
	}
	return neverT
}

func primitiveForKind(k Kind) *Type {
	switch k {
	case KindNumber:
		return numberT
	case KindString:
		return stringT
	case KindBoolean:
		return booleanT
	case KindNil:
		return nilT
	}
	return nil
}

func kindForTypeString(s string) (Kind, bool) {
	switch s {
	case "number":
		return KindNumber, true
	case "string":
		return KindString, true
	case "boolean":
		return KindBoolean, true
	case "nil":
		return KindNil, true
	case "table":
		return KindTable, true
	case "function":
		return KindFunction, true
	}
	return 0, false
}

func typeGuardTarget(e ast.Expression) (target, guardFn string, ok bool) {
	call, isCall := e.(*ast.CallExpression)
	if !isCall || len(call.Args) != 1 {
		return "", "", false
	}
	fn, isIdent := call.Func.(*ast.Identifier)
	if !isIdent || (fn.Name != "type" && fn.Name != "typeof") {
		return "", "", false
	}
	target, ok = identName(call.Args[0])
	return target, fn.Name, ok
}

func identName(e ast.Expression) (string, bool) {
	if p, ok := e.(*ast.ParenExpression); ok {
		return identName(p.Inner)
	}
	if id, ok := e.(*ast.Identifier); ok {
		return id.Name, true
	}
	return "", false
}

func stringLitValue(e ast.Expression) (string, bool) {
	if p, ok := e.(*ast.ParenExpression); ok {
		return stringLitValue(p.Inner)
	}
	if s, ok := e.(*ast.StringLiteral); ok {
		return s.Value, true
	}
	return "", false
}

func isNilLiteral(e ast.Expression) bool {
	if p, ok := e.(*ast.ParenExpression); ok {
		return isNilLiteral(p.Inner)
	}
	_, ok := e.(*ast.NilLiteral)
	return ok
}
