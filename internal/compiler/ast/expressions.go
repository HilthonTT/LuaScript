package ast

import (
	"bytes"
	"strconv"
	"strings"
)

type NilLiteral struct {
	BaseNode
}

func (*NilLiteral) expressionNode()        {}
func (n *NilLiteral) TokenLiteral() string { return n.Token.Literal }
func (*NilLiteral) String() string         { return "nil" }

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

type IntegerLiteral struct {
	BaseNode
	Value int64
}

func (*IntegerLiteral) expressionNode()         {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return strconv.FormatInt(il.Value, 10) }

type FloatLiteral struct {
	BaseNode
	Value float64
}

func (*FloatLiteral) expressionNode()         {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return strconv.FormatFloat(fl.Value, 'g', -1, 64) }

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

type VarargExpression struct {
	BaseNode
}

func (*VarargExpression) expressionNode()        {}
func (v *VarargExpression) TokenLiteral() string { return v.Token.Literal }
func (*VarargExpression) String() string         { return "..." }

type Identifier struct {
	BaseNode
	Name string
}

func (*Identifier) expressionNode()        {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Name }

type TypedParam struct {
	Name    *Identifier
	Type    TypeNode
	Default Expression
}

type FunctionExpression struct {
	BaseNode
	TypeParams  []string
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
		s := p.Name.String()
		if p.Type != nil {
			s += ": " + p.Type.String()
		}
		if p.Default != nil {
			s += " = " + p.Default.String()
		}
		parts = append(parts, s)
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

type TableField struct {
	Key         Expression
	Value       Expression
	IsBracketed bool
}

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

type IfExprClause struct {
	Condition Expression
	Value     Expression
}

type IfExpression struct {
	BaseNode
	Clauses []IfExprClause
	Else    Expression
}

func (*IfExpression) expressionNode()         {}
func (ie *IfExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IfExpression) String() string {
	var out bytes.Buffer
	for i, c := range ie.Clauses {
		if i == 0 {
			out.WriteString("if ")
		} else {
			out.WriteString(" elseif ")
		}
		out.WriteString(c.Condition.String())
		out.WriteString(" then ")
		out.WriteString(c.Value.String())
	}
	out.WriteString(" else ")
	out.WriteString(ie.Else.String())
	return out.String()
}

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
