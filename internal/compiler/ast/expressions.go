package ast

import (
	"bytes"
	"strconv"
	"strings"
)

// NilLiteral represents the `nil` keyword used as an expression.
type NilLiteral struct {
	BaseNode
}

func (*NilLiteral) expressionNode()        {}
func (n *NilLiteral) TokenLiteral() string { return n.Token.Literal }
func (*NilLiteral) String() string         { return "nil" }

// BooleanLiteral represents `true` or `false`.
type BooleanLiteral struct {
	BaseNode
	Value bool
}

func (*BooleanLiteral) expressionNode()        {}
func (b *BooleanLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BooleanLiteral) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// IntegerLiteral represents a Lua 5.3+ integer subtype value.
type IntegerLiteral struct {
	BaseNode
	Value int64
}

func (*IntegerLiteral) expressionNode()         {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return strconv.FormatInt(il.Value, 10) }

// FloatLiteral represents a Lua float subtype value.
type FloatLiteral struct {
	BaseNode
	Value float64
}

func (*FloatLiteral) expressionNode()         {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return strconv.FormatFloat(fl.Value, 'g', -1, 64) }

// StringLiteral represents a string literal. IsLong is true when the source
// used the `[[ ... ]]` long-bracket form.
type StringLiteral struct {
	BaseNode
	Value  string
	IsLong bool
}

func (*StringLiteral) expressionNode()         {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string {
	if sl.IsLong {
		return "[[" + sl.Value + "]]"
	}
	return strconv.Quote(sl.Value)
}

// VarargExpression represents the `...` expression inside a vararg function.
type VarargExpression struct {
	BaseNode
}

func (*VarargExpression) expressionNode()        {}
func (v *VarargExpression) TokenLiteral() string { return v.Token.Literal }
func (*VarargExpression) String() string         { return "..." }

// Identifier names a local, global, or parameter.
type Identifier struct {
	BaseNode
	Name string
}

func (*Identifier) expressionNode()        {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Name }

// TypedParam is one entry in a FunctionExpression's parameter list. Type is
// nil for unannotated parameters; the type checker treats those as `any` in
// gradual mode and as an implicit-any error in strict mode.
type TypedParam struct {
	Name *Identifier
	Type TypeNode
}

// FunctionExpression is `function(params) body end`. IsVararg is true when
// the parameter list ends in `...`. ReturnTypes carries the optional
// `: T` / `: (T1, T2)` annotation between `)` and the body. VarargType is
// the optional `...: T` annotation. All type fields are nil-safe.
type FunctionExpression struct {
	BaseNode
	TypeParams  []string // generic parameters `<T, U>`; empty for a non-generic function
	Params      []TypedParam
	IsVararg    bool
	VarargType  TypeNode
	ReturnTypes []TypeNode
	Body        *Block
}

func (*FunctionExpression) expressionNode()         {}
func (fe *FunctionExpression) TokenLiteral() string { return fe.Token.Literal }
func (fe *FunctionExpression) String() string {
	var out bytes.Buffer
	out.WriteString("function(")
	parts := make([]string, 0, len(fe.Params)+1)
	for _, p := range fe.Params {
		if p.Type != nil {
			parts = append(parts, p.Name.String()+": "+p.Type.String())
		} else {
			parts = append(parts, p.Name.String())
		}
	}
	if fe.IsVararg {
		if fe.VarargType != nil {
			parts = append(parts, "...: "+fe.VarargType.String())
		} else {
			parts = append(parts, "...")
		}
	}
	out.WriteString(strings.Join(parts, ", "))
	out.WriteString(")")
	if len(fe.ReturnTypes) > 0 {
		out.WriteString(": ")
		switch len(fe.ReturnTypes) {
		case 1:
			out.WriteString(fe.ReturnTypes[0].String())
		default:
			rets := make([]string, len(fe.ReturnTypes))
			for i, r := range fe.ReturnTypes {
				rets[i] = r.String()
			}
			out.WriteString("(")
			out.WriteString(strings.Join(rets, ", "))
			out.WriteString(")")
		}
	}
	out.WriteString("\n")
	if fe.Body != nil {
		out.WriteString(fe.Body.String())
	}
	out.WriteString("end")
	return out.String()
}

// TableField is a single entry in a TableConstructor.
//
//	Key == nil                              -> array entry  ({ v })
//	Key is *Identifier && !IsBracketed       -> record entry ({ name = v })
//	IsBracketed                             -> keyed entry  ({ [expr] = v })
type TableField struct {
	Key         Expression
	Value       Expression
	IsBracketed bool
}

// TableConstructor is `{ ... }` — Lua's only aggregate literal.
type TableConstructor struct {
	BaseNode
	Fields []TableField
}

func (*TableConstructor) expressionNode()         {}
func (tc *TableConstructor) TokenLiteral() string { return tc.Token.Literal }
func (tc *TableConstructor) String() string {
	var out bytes.Buffer
	out.WriteString("{")
	for i, f := range tc.Fields {
		if i > 0 {
			out.WriteString(", ")
		} else {
			out.WriteString(" ")
		}
		switch {
		case f.Key == nil:
			out.WriteString(f.Value.String())
		case f.IsBracketed:
			out.WriteString("[")
			out.WriteString(f.Key.String())
			out.WriteString("] = ")
			out.WriteString(f.Value.String())
		default:
			out.WriteString(f.Key.String())
			out.WriteString(" = ")
			out.WriteString(f.Value.String())
		}
	}
	if len(tc.Fields) > 0 {
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

// IndexExpression is `obj[index]` or, when IsDot is true, `obj.name` — in
// the dot form Index is a *StringLiteral whose Value is the field name.
type IndexExpression struct {
	BaseNode
	Object Expression
	Index  Expression
	IsDot  bool
}

func (*IndexExpression) expressionNode()         {}
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ie.Object.String())
	if ie.IsDot {
		if s, ok := ie.Index.(*StringLiteral); ok {
			out.WriteString(".")
			out.WriteString(s.Value)
			return out.String()
		}
	}
	out.WriteString("[")
	out.WriteString(ie.Index.String())
	out.WriteString("]")
	return out.String()
}

// CallExpression is `f(args)`. The parser also folds `f"str"` and `f{tbl}`
// into this shape with a single-element Args slice.
type CallExpression struct {
	BaseNode
	Func Expression
	Args []Expression
}

func (*CallExpression) expressionNode()         {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ce.Func.String())
	out.WriteString("(")
	out.WriteString(joinExprs(ce.Args, ", "))
	out.WriteString(")")
	return out.String()
}

// MethodCallExpression is `obj:method(args)` — distinct from CallExpression
// because the colon form passes `obj` as an implicit first argument.
type MethodCallExpression struct {
	BaseNode
	Object Expression
	Method string
	Args   []Expression
}

func (*MethodCallExpression) expressionNode()         {}
func (mc *MethodCallExpression) TokenLiteral() string { return mc.Token.Literal }
func (mc *MethodCallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(mc.Object.String())
	out.WriteString(":")
	out.WriteString(mc.Method)
	out.WriteString("(")
	out.WriteString(joinExprs(mc.Args, ", "))
	out.WriteString(")")
	return out.String()
}

// BinaryExpression covers every Lua 5.4 binary operator:
//
//   - - * / // % ^ ..  ==  ~=  <  >  <=  >=  and  or  &  |  ~  <<  >>
type BinaryExpression struct {
	BaseNode
	Op    string
	Left  Expression
	Right Expression
}

func (*BinaryExpression) expressionNode()         {}
func (be *BinaryExpression) TokenLiteral() string { return be.Token.Literal }
func (be *BinaryExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(be.Left.String())
	out.WriteString(" ")
	out.WriteString(be.Op)
	out.WriteString(" ")
	out.WriteString(be.Right.String())
	out.WriteString(")")
	return out.String()
}

// UnaryExpression covers `-`, `not`, `#`, `~`.
type UnaryExpression struct {
	BaseNode
	Op      string
	Operand Expression
}

func (*UnaryExpression) expressionNode()         {}
func (ue *UnaryExpression) TokenLiteral() string { return ue.Token.Literal }
func (ue *UnaryExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ue.Op)
	if ue.Op == "not" {
		out.WriteString(" ")
	}
	out.WriteString(ue.Operand.String())
	out.WriteString(")")
	return out.String()
}

// ParenExpression is a parenthesized expression. In Lua this is semantically
// meaningful: it adjusts a multi-value result down to exactly one value.
type ParenExpression struct {
	BaseNode
	Inner Expression
}

func (*ParenExpression) expressionNode()         {}
func (pe *ParenExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *ParenExpression) String() string       { return "(" + pe.Inner.String() + ")" }

func joinExprs(exprs []Expression, sep string) string {
	parts := make([]string, len(exprs))
	for i, e := range exprs {
		parts[i] = e.String()
	}
	return strings.Join(parts, sep)
}

// WildcardPattern is `_` — matches anything, binds nothing. It is an error
// for any arm after a wildcard to be reachable.
type WildcardPattern struct {
	BaseNode
}

func (*WildcardPattern) patternNode()           {}
func (w *WildcardPattern) TokenLiteral() string { return w.Token.Literal }
func (*WildcardPattern) String() string         { return "_" }

// LiteralPattern matches by structural equality against a constant. Value
// must be one of *NilLiteral, *BooleanLiteral, *IntegerLiteral,
// *FloatLiteral, *StringLiteral — the parser is responsible for that
// constraint; the AST does not enforce it via the type system to avoid
// inventing a duplicate "literal expression" interface.
type LiteralPattern struct {
	BaseNode
	Value Expression
}

func (*LiteralPattern) patternNode()            {}
func (lp *LiteralPattern) TokenLiteral() string { return lp.Token.Literal }
func (lp *LiteralPattern) String() string       { return lp.Value.String() }

// BindingPattern matches anything and binds it to Name in the arm body
// (and in any guard). Prefer WildcardPattern when no binding is needed —
// a leading-underscore name is still a binding, not a wildcard.
type BindingPattern struct {
	BaseNode
	Name *Identifier
}

func (*BindingPattern) patternNode()            {}
func (bp *BindingPattern) TokenLiteral() string { return bp.Token.Literal }
func (bp *BindingPattern) String() string       { return bp.Name.String() }

// MatchArm is one `pattern [if guard] -> body` clause. Token points at
// the first token of the pattern so the typechecker can report
// arm-specific errors (non-exhaustive, unreachable, type mismatch).
type MatchArm struct {
	BaseNode
	Pattern Pattern
	Guard   Expression // optional `if expr` between pattern and `->`; nil when absent
	Body    Expression
}

// MatchExpression is `match subject { p1 -> e1, p2 if g -> e2, _ -> e3 }`.
// It is exhaustive: every possible value of Subject must be covered by some
// arm (a trailing `_ -> ...` is the usual way to satisfy that).
type MatchExpression struct {
	BaseNode
	Subject Expression
	Arms    []MatchArm
}

func (*MatchExpression) expressionNode()         {}
func (me *MatchExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MatchExpression) String() string {
	var out bytes.Buffer
	out.WriteString("match ")
	out.WriteString(me.Subject.String())
	out.WriteString(" { ")
	for i, arm := range me.Arms {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(arm.Pattern.String())
		if arm.Guard != nil {
			out.WriteString(" if ")
			out.WriteString(arm.Guard.String())
		}
		out.WriteString(" -> ")
		out.WriteString(arm.Body.String())
	}
	out.WriteString(" }")
	return out.String()
}
