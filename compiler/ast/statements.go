package ast

import (
	"bytes"
	"strings"
)

// Block is a sequence of statements optionally terminated by a return.
// Lua's grammar requires `return` to be the last statement of a block, so we
// model it as a separate field rather than as just another Statement.
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

// IsEmpty reports whether the block has no statements and no return.
func (b *Block) IsEmpty() bool {
	return len(b.Statements) == 0 && b.Return == nil
}

// AssignStatement is `varlist = explist`. Each Target must be an Identifier
// (global/local reference) or an IndexExpression.
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

// LocalName is one entry in a `local` declaration. Attrib is "" for a plain
// local, or "const" / "close" for the Lua 5.4 attributes (`<const>`, `<close>`).
// Type is the optional Luau-style annotation (`local x: number`); nil if
// unannotated.
type LocalName struct {
	Name   string
	Attrib string
	Type   TypeNode
}

// LocalStatement is `local namelist [= explist]`.
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

// LocalFunctionStatement is `local function Name funcbody`.
type LocalFunctionStatement struct {
	BaseNode
	Name string
	Func *FunctionExpression
}

func (*LocalFunctionStatement) statementNode()          {}
func (lf *LocalFunctionStatement) TokenLiteral() string { return lf.Token.Literal }
func (lf *LocalFunctionStatement) String() string {
	// FunctionExpression renders as "function(params) body end"; splice the
	// name in after "function".
	body := lf.Func.String()
	return "local function " + lf.Name + strings.TrimPrefix(body, "function")
}

// FunctionDeclaration is `function funcname funcbody`. funcname has the form
//
//	Name {`.` Name} [`:` Name]
//
// modeled here as a base Identifier, zero-or-more dotted fields, and an
// optional MethodName for the colon form (`function t.a.b:m()`). When
// MethodName != "" the implicit `self` parameter is conventionally inserted
// at codegen time.
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

// IfClause is one `if` or `elseif` branch.
type IfClause struct {
	Condition Expression
	Body      *Block
}

// IfStatement is `if c then ... {elseif c then ...} [else ...] end`.
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

// WhileStatement is `while cond do block end`.
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

// RepeatStatement is `repeat block until cond`. Note: `cond` is evaluated in
// the scope of locals declared in `Body`.
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

// NumericForStatement is `for Name = e1, e2 [, e3] do block end`. Step is nil
// when omitted.
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

// GenericForStatement is `for namelist in explist do block end`.
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

// DoStatement is `do block end` — a bare scoping block.
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

// ReturnStatement is `return [explist] [;]`. Lua only allows it as the last
// statement of a block; see Block.Return.
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

// BreakStatement is `break`.
type BreakStatement struct {
	BaseNode
}

func (*BreakStatement) statementNode()         {}
func (b *BreakStatement) TokenLiteral() string { return b.Token.Literal }
func (*BreakStatement) String() string         { return "break" }

// GotoStatement is `goto Name` (Lua 5.2+).
type GotoStatement struct {
	BaseNode
	Label string
}

func (*GotoStatement) statementNode()         {}
func (g *GotoStatement) TokenLiteral() string { return g.Token.Literal }
func (g *GotoStatement) String() string       { return "goto " + g.Label }

// LabelStatement is `::Name::` — a goto target.
type LabelStatement struct {
	BaseNode
	Name string
}

func (*LabelStatement) statementNode()         {}
func (l *LabelStatement) TokenLiteral() string { return l.Token.Literal }
func (l *LabelStatement) String() string       { return "::" + l.Name + "::" }

// ExpressionStatement wraps a top-level expression. The Lua grammar only
// permits function and method calls in this position; the parser is
// responsible for rejecting anything else.
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

// EnumStatement:
// enum Color
//
//	RED,
//	GREEN,
//	BLUE
//
// END
type EnumStatement struct {
	BaseNode
	Name     *Identifier
	Variants []*EnumVariantDef
}

type EnumVariantDef struct {
	Name   string
	Fields []string // Empty for simple variants
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
	}
	out.WriteString("end")
	return out.String()
}

// DeferStatement is `defer <call>` — the call runs when the enclosing function
// unwinds (normal return, fall-off-end, or an error caught by pcall), in
// last-in-first-out order. Call is the bare function/method call expression;
// the bytecode generator wraps it in a zero-arg closure so it executes lazily
// at frame teardown.
type DeferStatement struct {
	BaseNode
	Call Expression
}

func (*DeferStatement) statementNode()          {}
func (ds *DeferStatement) TokenLiteral() string { return ds.Token.Literal }
func (ds *DeferStatement) String() string {
	return "defer " + ds.Call.String()
}
