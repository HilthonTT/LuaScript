package typecheck

// `match` exhaustiveness checking.
//
// A `match` whose arms miss a case is a no-op at runtime — control falls out
// of the statement and the following code runs with whatever state it had.
// When every arm ends in `return`, that shows up as a function silently
// returning nil. This file turns that class of bug into a compile error, but
// only where the checker can prove what the complete set of cases *is*.
//
// A subject's domain is enumerable in exactly three cases:
//
//	tagged enum      `Shape` → { Circle, Rect, Unit }   (the variant names)
//	literal union    `"read" | "write"`, and the classic enums that desugar
//	                 to one (`Color` → `1 | 2 | 3`)
//	boolean          → { true, false }
//
// Anything else — `any`, `string`, a table, an unannotated local — has no
// finite domain, so no diagnostic is possible and none is reported. That is
// the design: precision arrives with types, and untyped Lua keeps working
// exactly as before.
//
// Only *unguarded* arms count as coverage. `Circle(r) if r > 0` proves
// nothing about the `r <= 0` case, so a match built entirely from guarded
// arms is never exhaustive.

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// maxReportedCases caps how many missing cases an error message lists, so a
// wide enum doesn't produce an unreadable line. The count is always exact.
const maxReportedCases = 4

// checkMatchExhaustive reports the cases `s` fails to handle, given the
// subject's type. A no-op when the subject's domain isn't enumerable or an
// irrefutable arm already covers everything.
func (c *checker) checkMatchExhaustive(s *ast.MatchStatement, subject *Type) {
	if hasIrrefutableArm(s.Arms) {
		return
	}
	domain, kind := c.domainOf(subject)
	if len(domain) == 0 {
		return
	}

	covered := c.coveredCases(s.Arms, subject)
	missing := make([]string, 0, len(domain))
	for _, d := range domain {
		if !covered[d.key] {
			missing = append(missing, d.label)
		}
	}
	if len(missing) == 0 {
		return
	}

	c.errf(s.Line(), "match-not-exhaustive",
		"`match` on %s does not handle %s: %s",
		kind, pluralCases(len(missing)), joinCapped(missing, maxReportedCases))
}

// matchCase is one member of a subject's domain: `key` identifies it for
// set arithmetic, `label` is how it is written in a diagnostic.
type matchCase struct {
	key   string
	label string
}

// domainOf enumerates the values a subject of type `t` can take, plus a short
// phrase naming the sort of domain it is (used in the error message). Returns
// nil when the type has no finite domain.
func (c *checker) domainOf(t *Type) ([]matchCase, string) {
	if t == nil {
		return nil, ""
	}

	// A tagged enum: the nominal alias name identifies the declaration, and
	// the recorded variant list is the domain.
	if t.AliasName != "" {
		if variants, ok := c.taggedEnums[t.AliasName]; ok {
			out := make([]matchCase, len(variants))
			for i, v := range variants {
				out[i] = matchCase{key: "tag:" + v, label: t.AliasName + "." + v}
			}
			return out, fmt.Sprintf("enum %q", t.AliasName)
		}
	}

	// `boolean` is the one primitive with a domain small enough to enumerate.
	if t.Kind == KindBoolean {
		return []matchCase{
			{key: literalKey(&LiteralValue{Base: KindBoolean, Bool: true}), label: "true"},
			{key: literalKey(&LiteralValue{Base: KindBoolean, Bool: false}), label: "false"},
		}, "a boolean"
	}

	// A union of singletons — written directly (`"read" | "write"`) or
	// produced by a classic enum declaration.
	lits, ok := literalMembers(t)
	if !ok {
		return nil, ""
	}
	// A lone singleton that nobody named is the type of an inline literal
	// scrutinee (`match 42 do`). Its "domain" is the one value written right
	// there, so demanding an arm for it would be noise, not a diagnostic.
	// A declared alias — even a single-member one — is a real domain.
	if len(lits) < 2 && t.AliasName == "" {
		return nil, ""
	}
	// A classic enum's members read far better by name than by number:
	// `Color.BLUE` rather than `3`.
	memberNames := c.classicEnums[t.AliasName]
	out := make([]matchCase, len(lits))
	for i, l := range lits {
		label := formatLiteral(l)
		if l.Base == KindNumber {
			if idx := int(l.Num) - 1; idx >= 0 && idx < len(memberNames) {
				label = t.AliasName + "." + memberNames[idx]
			}
		}
		out[i] = matchCase{key: literalKey(l), label: label}
	}
	switch {
	case len(memberNames) > 0:
		return out, fmt.Sprintf("enum %q", t.AliasName)
	case t.AliasName != "":
		return out, fmt.Sprintf("type %q", t.AliasName)
	}
	return out, "a singleton union"
}

// coveredCases collects the domain keys the arms handle. Guarded arms are
// skipped: a guard can fail, so such an arm proves nothing about the case.
func (c *checker) coveredCases(arms []ast.MatchStmtArm, subject *Type) map[string]bool {
	covered := map[string]bool{}
	for i := range arms {
		arm := &arms[i]
		if arm.Guard != nil {
			continue
		}
		switch arm.Pattern.Kind {
		case ast.MatchDestructurePos, ast.MatchDestructureNamed:
			// `Shape.Circle(r)` / `Point{ x = a }` — Tag is the final path
			// segment, which is the variant name for an enum.
			covered["tag:"+arm.Pattern.Tag] = true
		case ast.MatchValue:
			for _, v := range arm.Pattern.Values {
				if lit := c.staticLiteralType(v); lit != nil && lit.Lit != nil {
					covered[literalKey(lit.Lit)] = true
					continue
				}
				// A nullary tagged-enum variant is referenced by path
				// (`Shape.Unit`) and compared by identity, so it lands here
				// rather than in the literal branch.
				if seg, ok := pathTail(v); ok {
					covered["tag:"+seg] = true
				}
			}
		case ast.MatchTyped:
			// A typed arm matches every value the subject can hold whenever
			// the subject flows into the arm's type — `x: Shape` against a
			// `Shape`, but also `n: number` against a classic enum, whose
			// members are all numbers. (`x: any` is handled earlier, by
			// hasIrrefutableArm.)
			if arm.Pattern.Type != nil && assignable(subject, c.resolveAST(arm.Pattern.Type)) {
				return c.allCovered(subject)
			}
		}
	}
	return covered
}

// allCovered returns a set marking every case of the subject's domain, used
// when an arm is proven to match anything the subject can be.
func (c *checker) allCovered(subject *Type) map[string]bool {
	domain, _ := c.domainOf(subject)
	covered := make(map[string]bool, len(domain))
	for _, d := range domain {
		covered[d.key] = true
	}
	return covered
}

// hasIrrefutableArm reports whether some unguarded arm matches every value:
// the `_` wildcard, or a typed pattern annotated `any`.
func hasIrrefutableArm(arms []ast.MatchStmtArm) bool {
	for i := range arms {
		arm := &arms[i]
		if arm.Guard != nil {
			continue
		}
		if arm.Pattern.Kind == ast.MatchWildcard {
			return true
		}
		if arm.Pattern.Kind == ast.MatchTyped {
			if p, ok := arm.Pattern.Type.(*ast.TypePrimitive); ok && p.Name == "any" {
				return true
			}
		}
	}
	return false
}

// literalKey is the set-membership identity of a singleton value. Numbers
// share one key regardless of spelling, matching Lua's `1 == 1.0`.
func literalKey(l *LiteralValue) string {
	if l == nil {
		return ""
	}
	switch l.Base {
	case KindString:
		return "s:" + l.Str
	case KindNumber:
		return fmt.Sprintf("n:%v", l.Num)
	case KindBoolean:
		return fmt.Sprintf("b:%t", l.Bool)
	}
	return ""
}

// pathTail returns the final identifier of an `A.B.C` path expression (or a
// bare identifier). Mirrors the parser's own classification of enum-variant
// patterns, which is keyed on the same last segment.
func pathTail(e ast.Expression) (string, bool) {
	switch n := e.(type) {
	case *ast.Identifier:
		return n.Name, true
	case *ast.IndexExpression:
		if n.IsDot {
			if s, ok := n.Index.(*ast.StringLiteral); ok {
				return s.Value, true
			}
			if id, ok := n.Index.(*ast.Identifier); ok {
				return id.Name, true
			}
		}
	}
	return "", false
}

func pluralCases(n int) string {
	if n == 1 {
		return "1 case"
	}
	return fmt.Sprintf("%d cases", n)
}

// joinCapped renders at most `limit` entries, summarising the rest.
func joinCapped(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") +
		fmt.Sprintf(", and %d more", len(items)-limit)
}
