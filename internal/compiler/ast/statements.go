package ast

import (
	"bytes"
	"strings"
)

type Block struct {
	BaseNode
	Statements []Statement
	Return     *ReturnStatement
}

func (*Block) statementNode()         {}
func (b *Block) TokenLiteral() string { return b.Token.Literal }
func (b *Block) String() string {
	var out bytes.Buffer
	for _, s := range b.Statements {
		out.WriteString(s.String())
		out.WriteString("\n")
	}
	if b.Return != nil {
		out.WriteString(b.Return.String())
		out.WriteString("\n")
	}
	return out.String()
}

func (b *Block) IsEmpty() bool {
	return len(b.Statements) == 0 && b.Return == nil
}

type AssignStatement struct {
	BaseNode
	Targets []Expression
	Values  []Expression
}

func (*AssignStatement) statementNode()         {}
func (a *AssignStatement) TokenLiteral() string { return a.Token.Literal }
func (a *AssignStatement) String() string {
	var out bytes.Buffer
	out.WriteString(joinExprs(a.Targets, ", "))
	out.WriteString(" = ")
	out.WriteString(joinExprs(a.Values, ", "))
	return out.String()
}

type LocalName struct {
	Name   string
	Attrib string
	Type   TypeNode
}

type LocalStatement struct {
	BaseNode
	Names  []LocalName
	Values []Expression
}

func (*LocalStatement) statementNode()          {}
func (ls *LocalStatement) TokenLiteral() string { return ls.Token.Literal }
func (ls *LocalStatement) String() string {
	var out bytes.Buffer
	out.WriteString("local ")
	parts := make([]string, len(ls.Names))
	for i, n := range ls.Names {
		s := n.Name
		if n.Type != nil {
			s += ": " + n.Type.String()
		}
		if n.Attrib != "" {
			s += " <" + n.Attrib + ">"
		}
		parts[i] = s
	}
	out.WriteString(strings.Join(parts, ", "))
	if len(ls.Values) > 0 {
		out.WriteString(" = ")
		out.WriteString(joinExprs(ls.Values, ", "))
	}
	return out.String()
}

type LocalFunctionStatement struct {
	BaseNode
	Name string
	Func *FunctionExpression
}

func (*LocalFunctionStatement) statementNode()          {}
func (lf *LocalFunctionStatement) TokenLiteral() string { return lf.Token.Literal }
func (lf *LocalFunctionStatement) String() string {
	body := lf.Func.String()
	return "local function " + lf.Name + strings.TrimPrefix(body, "function")
}

type FunctionDeclaration struct {
	BaseNode
	Name         *Identifier
	DottedFields []string
	MethodName   string
	Func         *FunctionExpression
}

func (*FunctionDeclaration) statementNode()          {}
func (fd *FunctionDeclaration) TokenLiteral() string { return fd.Token.Literal }
func (fd *FunctionDeclaration) String() string {
	var out bytes.Buffer
	out.WriteString("function ")
	out.WriteString(fd.Name.Name)
	for _, f := range fd.DottedFields {
		out.WriteString(".")
		out.WriteString(f)
	}
	if fd.MethodName != "" {
		out.WriteString(":")
		out.WriteString(fd.MethodName)
	}
	body := fd.Func.String()
	out.WriteString(strings.TrimPrefix(body, "function"))
	return out.String()
}

type IfClause struct {
	Condition Expression
	Body      *Block
}

type IfStatement struct {
	BaseNode
	Clauses []IfClause
	Else    *Block
}

func (*IfStatement) statementNode()         {}
func (i *IfStatement) TokenLiteral() string { return i.Token.Literal }
func (i *IfStatement) String() string {
	var out bytes.Buffer
	for idx, c := range i.Clauses {
		if idx == 0 {
			out.WriteString("if ")
		} else {
			out.WriteString("elseif ")
		}
		out.WriteString(c.Condition.String())
		out.WriteString(" then\n")
		if c.Body != nil {
			out.WriteString(c.Body.String())
		}
	}
	if i.Else != nil {
		out.WriteString("else\n")
		out.WriteString(i.Else.String())
	}
	out.WriteString("end")
	return out.String()
}

type WhileStatement struct {
	BaseNode
	Condition Expression
	Body      *Block
}

func (*WhileStatement) statementNode()         {}
func (w *WhileStatement) TokenLiteral() string { return w.Token.Literal }
func (w *WhileStatement) String() string {
	var out bytes.Buffer
	out.WriteString("while ")
	out.WriteString(w.Condition.String())
	out.WriteString(" do\n")
	if w.Body != nil {
		out.WriteString(w.Body.String())
	}
	out.WriteString("end")
	return out.String()
}

type RepeatStatement struct {
	BaseNode
	Body      *Block
	Condition Expression
}

func (*RepeatStatement) statementNode()         {}
func (r *RepeatStatement) TokenLiteral() string { return r.Token.Literal }
func (r *RepeatStatement) String() string {
	var out bytes.Buffer
	out.WriteString("repeat\n")
	if r.Body != nil {
		out.WriteString(r.Body.String())
	}
	out.WriteString("until ")
	out.WriteString(r.Condition.String())
	return out.String()
}

type NumericForStatement struct {
	BaseNode
	Name  string
	Start Expression
	Limit Expression
	Step  Expression
	Body  *Block
}

func (*NumericForStatement) statementNode()         {}
func (f *NumericForStatement) TokenLiteral() string { return f.Token.Literal }
func (f *NumericForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for ")
	out.WriteString(f.Name)
	out.WriteString(" = ")
	out.WriteString(f.Start.String())
	out.WriteString(", ")
	out.WriteString(f.Limit.String())
	if f.Step != nil {
		out.WriteString(", ")
		out.WriteString(f.Step.String())
	}
	out.WriteString(" do\n")
	if f.Body != nil {
		out.WriteString(f.Body.String())
	}
	out.WriteString("end")
	return out.String()
}

type GenericForStatement struct {
	BaseNode
	Names []string
	Exprs []Expression
	Body  *Block
}

func (*GenericForStatement) statementNode()         {}
func (f *GenericForStatement) TokenLiteral() string { return f.Token.Literal }
func (f *GenericForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for ")
	out.WriteString(strings.Join(f.Names, ", "))
	out.WriteString(" in ")
	out.WriteString(joinExprs(f.Exprs, ", "))
	out.WriteString(" do\n")
	if f.Body != nil {
		out.WriteString(f.Body.String())
	}
	out.WriteString("end")
	return out.String()
}

type DoStatement struct {
	BaseNode
	Body *Block
}

func (*DoStatement) statementNode()         {}
func (d *DoStatement) TokenLiteral() string { return d.Token.Literal }
func (d *DoStatement) String() string {
	var out bytes.Buffer
	out.WriteString("do\n")
	if d.Body != nil {
		out.WriteString(d.Body.String())
	}
	out.WriteString("end")
	return out.String()
}

type ReturnStatement struct {
	BaseNode
	Values []Expression
}

func (*ReturnStatement) statementNode()          {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	if len(rs.Values) == 0 {
		return "return"
	}
	return "return " + joinExprs(rs.Values, ", ")
}

type BreakStatement struct {
	BaseNode
}

func (*BreakStatement) statementNode()         {}
func (b *BreakStatement) TokenLiteral() string { return b.Token.Literal }
func (*BreakStatement) String() string         { return "break" }

type ContinueStatement struct {
	BaseNode
}

func (*ContinueStatement) statementNode()         {}
func (c *ContinueStatement) TokenLiteral() string { return c.Token.Literal }
func (*ContinueStatement) String() string         { return "continue" }

type GotoStatement struct {
	BaseNode
	Label string
}

func (*GotoStatement) statementNode()         {}
func (g *GotoStatement) TokenLiteral() string { return g.Token.Literal }
func (g *GotoStatement) String() string       { return "goto " + g.Label }

type LabelStatement struct {
	BaseNode
	Name string
}

func (*LabelStatement) statementNode()         {}
func (l *LabelStatement) TokenLiteral() string { return l.Token.Literal }
func (l *LabelStatement) String() string       { return "::" + l.Name + "::" }

type ExpressionStatement struct {
	BaseNode
	Expression Expression
}

func (*ExpressionStatement) statementNode()          {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression == nil {
		return ""
	}
	return es.Expression.String()
}

type EnumStatement struct {
	BaseNode
	Name     *Identifier
	Variants []*EnumVariantDef
}

type EnumVariantDef struct {
	Name    string
	Payload []TypeNode
}

func (es *EnumStatement) IsTagged() bool {
	for _, v := range es.Variants {
		if len(v.Payload) > 0 {
			return true
		}
	}
	return false
}

func (*EnumStatement) statementNode()          {}
func (es *EnumStatement) TokenLiteral() string { return es.Token.Literal }
func (es *EnumStatement) String() string {
	var out bytes.Buffer
	out.WriteString("enum ")
	out.WriteString(es.Name.String())
	for i, v := range es.Variants {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(v.Name)
		if len(v.Payload) > 0 {
			out.WriteString("(")
			for j, p := range v.Payload {
				if j > 0 {
					out.WriteString(", ")
				}
				out.WriteString(p.String())
			}
			out.WriteString(")")
		}
	}
	out.WriteString("end")
	return out.String()
}

type DeferStatement struct {
	BaseNode
	Call Expression
}

func (*DeferStatement) statementNode()          {}
func (ds *DeferStatement) TokenLiteral() string { return ds.Token.Literal }
func (ds *DeferStatement) String() string {
	return "defer " + ds.Call.String()
}

type StructField struct {
	Name string
	Type TypeNode
}

type StructStatement struct {
	BaseNode
	Name       *Identifier
	TypeParams []string
	Fields     []StructField
}

func (*StructStatement) statementNode()          {}
func (ss *StructStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *StructStatement) String() string {
	var out bytes.Buffer
	out.WriteString("struct ")
	out.WriteString(ss.Name.String())
	if len(ss.TypeParams) > 0 {
		out.WriteString("<")
		out.WriteString(strings.Join(ss.TypeParams, ", "))
		out.WriteString(">")
	}
	out.WriteString(" {")
	for i, f := range ss.Fields {
		if i > 0 {
			out.WriteString(",")
		}
		out.WriteString(" ")
		out.WriteString(f.Name)
		out.WriteString(": ")
		out.WriteString(f.Type.String())
	}
	if len(ss.Fields) > 0 {
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

type TryCatchStatement struct {
	BaseNode
	Try      *Block
	CatchVar *Identifier
	Catch    *Block
}

func (*TryCatchStatement) statementNode()          {}
func (tc *TryCatchStatement) TokenLiteral() string { return tc.Token.Literal }
func (tc *TryCatchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("try ")
	out.WriteString(tc.Try.String())
	out.WriteString(" catch ")
	if tc.CatchVar != nil {
		out.WriteString(tc.CatchVar.String())
		out.WriteString(" ")
	}
	out.WriteString("do ")
	out.WriteString(tc.Catch.String())
	out.WriteString(" end")
	return out.String()
}

type ThrowStatement struct {
	BaseNode
	Value Expression
}

func (*ThrowStatement) statementNode()          {}
func (ts *ThrowStatement) TokenLiteral() string { return ts.Token.Literal }
func (ts *ThrowStatement) String() string {
	return "throw " + ts.Value.String()
}

type MatchPatternKind int

const (
	MatchValue MatchPatternKind = iota
	MatchWildcard
	MatchTyped
	MatchDestructurePos
	MatchDestructureNamed
)

type MatchFieldBind struct {
	Field string
	Bind  string
}

type MatchPattern struct {
	Kind       MatchPatternKind
	Values     []Expression
	Bind       string
	Type       TypeNode
	Tag        string
	PosBinds   []string
	NamedBinds []MatchFieldBind
}

func (mp *MatchPattern) Binders() []string {
	var out []string
	add := func(name string) {
		if name != "" && name != "_" {
			out = append(out, name)
		}
	}
	switch mp.Kind {
	case MatchTyped:
		add(mp.Bind)
	case MatchDestructurePos:
		for _, b := range mp.PosBinds {
			add(b)
		}
	case MatchDestructureNamed:
		for _, nb := range mp.NamedBinds {
			add(nb.Bind)
		}
	}
	return out
}

func (mp *MatchPattern) String() string {
	var out bytes.Buffer
	switch mp.Kind {
	case MatchValue:
		for i, v := range mp.Values {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(v.String())
		}
	case MatchWildcard:
		out.WriteString("_")
	case MatchTyped:
		name := mp.Bind
		if name == "" {
			name = "_"
		}
		out.WriteString(name)
		out.WriteString(": ")
		out.WriteString(mp.Type.String())
	case MatchDestructurePos:
		out.WriteString(mp.Tag)
		out.WriteString("(")
		for i, b := range mp.PosBinds {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(b)
		}
		out.WriteString(")")
	case MatchDestructureNamed:
		out.WriteString(mp.Tag)
		out.WriteString("{ ")
		for i, nb := range mp.NamedBinds {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(nb.Field)
			out.WriteString(" = ")
			out.WriteString(nb.Bind)
		}
		out.WriteString(" }")
	}
	return out.String()
}

type MatchStmtArm struct {
	BaseNode
	Pattern MatchPattern
	Guard   Expression
	Body    Statement
}

type MatchStatement struct {
	BaseNode
	Subject Expression
	Arms    []MatchStmtArm
}

func (*MatchStatement) statementNode()          {}
func (ms *MatchStatement) TokenLiteral() string { return ms.Token.Literal }
func (ms *MatchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("match ")
	out.WriteString(ms.Subject.String())
	out.WriteString(" do ")
	for i := range ms.Arms {
		arm := &ms.Arms[i]
		if i > 0 {
			out.WriteString(" ")
		}
		out.WriteString(arm.Pattern.String())
		if arm.Guard != nil {
			out.WriteString(" if ")
			out.WriteString(arm.Guard.String())
		}
		out.WriteString(" -> ")
		out.WriteString(arm.Body.String())
	}
	out.WriteString(" end")
	return out.String()
}
