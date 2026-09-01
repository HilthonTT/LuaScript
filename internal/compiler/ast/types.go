package ast

import (
	"bytes"
	"strings"
)

type TypeNode interface {
	node
	typeNode()
}

type TypePrimitive struct {
	BaseNode
	Name string
}

func (*TypePrimitive) typeNode()              {}
func (t *TypePrimitive) TokenLiteral() string { return t.Token.Literal }
func (t *TypePrimitive) String() string       { return t.Name }

type TypeLiteralKind int

const (
	LiteralString TypeLiteralKind = iota
	LiteralNumber
	LiteralBoolean
)

type TypeLiteral struct {
	BaseNode
	Kind TypeLiteralKind
	Str  string
	Num  float64
	Bool bool
	Raw  string
}

func (*TypeLiteral) typeNode()              {}
func (t *TypeLiteral) TokenLiteral() string { return t.Token.Literal }
func (t *TypeLiteral) String() string       { return t.Raw }

type TypeName struct {
	BaseNode
	Name string
}

func (*TypeName) typeNode()              {}
func (t *TypeName) TokenLiteral() string { return t.Token.Literal }
func (t *TypeName) String() string       { return t.Name }

type TypeApplication struct {
	BaseNode
	Name string
	Args []TypeNode
}

func (*TypeApplication) typeNode()              {}
func (t *TypeApplication) TokenLiteral() string { return t.Token.Literal }
func (t *TypeApplication) String() string {
	parts := make([]string, len(t.Args))
	for i, a := range t.Args {
		parts[i] = a.String()
	}
	return t.Name + "<" + strings.Join(parts, ", ") + ">"
}

type TypeOptional struct {
	BaseNode
	Inner TypeNode
}

func (*TypeOptional) typeNode()              {}
func (t *TypeOptional) TokenLiteral() string { return t.Token.Literal }
func (t *TypeOptional) String() string       { return t.Inner.String() + "?" }

type TypeUnion struct {
	BaseNode
	Members []TypeNode
}

func (*TypeUnion) typeNode()              {}
func (t *TypeUnion) TokenLiteral() string { return t.Token.Literal }
func (t *TypeUnion) String() string {
	parts := make([]string, len(t.Members))
	for i, m := range t.Members {
		parts[i] = m.String()
	}
	return strings.Join(parts, " | ")
}

type TypeFunction struct {
	BaseNode
	ParamNames []string
	Params     []TypeNode
	Returns    []TypeNode
	IsVararg   bool
	VarargType TypeNode
}

func (*TypeFunction) typeNode()              {}
func (t *TypeFunction) TokenLiteral() string { return t.Token.Literal }
func (t *TypeFunction) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	parts := make([]string, 0, len(t.Params)+1)
	for i, p := range t.Params {
		if i < len(t.ParamNames) && t.ParamNames[i] != "" {
			parts = append(parts, t.ParamNames[i]+": "+p.String())
		} else {
			parts = append(parts, p.String())
		}
	}
	if t.IsVararg {
		if t.VarargType != nil {
			parts = append(parts, "...: "+t.VarargType.String())
		} else {
			parts = append(parts, "...")
		}
	}
	out.WriteString(strings.Join(parts, ", "))
	out.WriteString(") -> ")
	switch len(t.Returns) {
	case 0:
		out.WriteString("()")
	case 1:
		out.WriteString(t.Returns[0].String())
	default:
		rets := make([]string, len(t.Returns))
		for i, r := range t.Returns {
			rets[i] = r.String()
		}
		out.WriteString("(")
		out.WriteString(strings.Join(rets, ", "))
		out.WriteString(")")
	}
	return out.String()
}

type TypeTableField struct {
	Key   string
	Value TypeNode
}

type TypeIndexer struct {
	Key   TypeNode
	Value TypeNode
}

type TypeTable struct {
	BaseNode
	Fields  []TypeTableField
	Indexer *TypeIndexer
}

func (*TypeTable) typeNode()              {}
func (t *TypeTable) TokenLiteral() string { return t.Token.Literal }
func (t *TypeTable) String() string {
	var out bytes.Buffer
	out.WriteString("{")
	parts := make([]string, 0, len(t.Fields)+1)
	if t.Indexer != nil {
		parts = append(parts, "["+t.Indexer.Key.String()+"]: "+t.Indexer.Value.String())
	}
	for _, f := range t.Fields {
		parts = append(parts, f.Key+": "+f.Value.String())
	}
	if len(parts) > 0 {
		out.WriteString(" ")
		out.WriteString(strings.Join(parts, ", "))
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

type TypeAliasStatement struct {
	BaseNode
	Name       string
	TypeParams []string
	Target     TypeNode
}

func (*TypeAliasStatement) statementNode()         {}
func (s *TypeAliasStatement) TokenLiteral() string { return s.Token.Literal }
func (s *TypeAliasStatement) String() string {
	head := "type " + s.Name
	if len(s.TypeParams) > 0 {
		head += "<" + strings.Join(s.TypeParams, ", ") + ">"
	}
	return head + " = " + s.Target.String()
}

type TypeAssertionExpression struct {
	BaseNode
	Expr Expression
	Type TypeNode
}

func (*TypeAssertionExpression) expressionNode()        {}
func (e *TypeAssertionExpression) TokenLiteral() string { return e.Token.Literal }
func (e *TypeAssertionExpression) String() string       { return e.Expr.String() + " :: " + e.Type.String() }
