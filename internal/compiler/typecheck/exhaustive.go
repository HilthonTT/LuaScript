package typecheck

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

const maxReportedCases = 4

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

type matchCase struct {
	key   string
	label string
}

func (c *checker) domainOf(t *Type) ([]matchCase, string) {
	if t == nil {
		return nil, ""
	}

	if t.AliasName != "" {
		if variants, ok := c.taggedEnums[t.AliasName]; ok {
			out := make([]matchCase, len(variants))
			for i, v := range variants {
				out[i] = matchCase{key: "tag:" + v, label: t.AliasName + "." + v}
			}
			return out, fmt.Sprintf("enum %q", t.AliasName)
		}
	}

	if t.Kind == KindBoolean {
		return []matchCase{
			{key: literalKey(&LiteralValue{Base: KindBoolean, Bool: true}), label: "true"},
			{key: literalKey(&LiteralValue{Base: KindBoolean, Bool: false}), label: "false"},
		}, "a boolean"
	}

	lits, ok := literalMembers(t)
	if !ok {
		return nil, ""
	}
	if len(lits) < 2 && t.AliasName == "" {
		return nil, ""
	}
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

func (c *checker) coveredCases(arms []ast.MatchStmtArm, subject *Type) map[string]bool {
	covered := map[string]bool{}
	for i := range arms {
		arm := &arms[i]
		if arm.Guard != nil {
			continue
		}
		switch arm.Pattern.Kind {
		case ast.MatchDestructurePos, ast.MatchDestructureNamed:
			covered["tag:"+arm.Pattern.Tag] = true
		case ast.MatchValue:
			for _, v := range arm.Pattern.Values {
				if lit := c.staticLiteralType(v); lit != nil && lit.Lit != nil {
					covered[literalKey(lit.Lit)] = true
					continue
				}
				if seg, ok := pathTail(v); ok {
					covered["tag:"+seg] = true
				}
			}
		case ast.MatchTyped:
			if arm.Pattern.Type != nil && assignable(subject, c.resolveAST(arm.Pattern.Type)) {
				return c.allCovered(subject)
			}
		}
	}
	return covered
}

func (c *checker) allCovered(subject *Type) map[string]bool {
	domain, _ := c.domainOf(subject)
	covered := make(map[string]bool, len(domain))
	for _, d := range domain {
		covered[d.key] = true
	}
	return covered
}

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

func joinCapped(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") +
		fmt.Sprintf(", and %d more", len(items)-limit)
}
