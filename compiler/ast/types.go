package ast

import (
	"bytes"
	"strings"
)

// TypeNode is the AST shape of a parsed type expression. These nodes appear
// only as annotations attached to declarations (`local x: T`, parameters,
// return types, type aliases) and as the right-hand side of a type
// assertion (`expr :: T`). They never produce bytecode — the type checker
// is the sole consumer; the bytecode generator ignores them entirely.
//
// String() on every TypeNode renders Luau-syntax-faithful source so the
// AST round-trips through `Program.String()` for debugging and tests.
type TypeNode interface {
	node
	typeNode()
}

// TypePrimitive names a built-in type: number, string, boolean, nil, any,
// unknown. The set is fixed; unknown identifiers in type position are
// parsed as TypeName instead.
type TypePrimitive struct {
	BaseNode
	Name string
}

func (*TypePrimitive) typeNode()              {}
func (t *TypePrimitive) TokenLiteral() string { return t.Token.Literal }
func (t *TypePrimitive) String() string       { return t.Name }

// TypeName references a user-defined type alias by name. Resolution is the
// type checker's job — the parser only records the name. TypeArgs is the
// optional generic instantiation list: `Box<number>` parses to
// TypeName{Name: "Box", TypeArgs: [number]}. Empty for non-generic uses.
type TypeName struct {
	BaseNode
	Name     string
	TypeArgs []TypeNode
}

func (*TypeName) typeNode()              {}
func (t *TypeName) TokenLiteral() string { return t.Token.Literal }
func (t *TypeName) String() string {
	if len(t.TypeArgs) == 0 {
		return t.Name
	}
	parts := make([]string, len(t.TypeArgs))
	for i, a := range t.TypeArgs {
		parts[i] = a.String()
	}
	return t.Name + "<" + strings.Join(parts, ", ") + ">"
}

// TypeOptional is the postfix-`?` sugar: `T?` ≡ `T | nil`. Kept distinct
// from TypeUnion so source round-trips and error messages preserve the
// programmer's notation.
type TypeOptional struct {
	BaseNode
	Inner TypeNode
}

func (*TypeOptional) typeNode()              {}
func (t *TypeOptional) TokenLiteral() string { return t.Token.Literal }
func (t *TypeOptional) String() string       { return t.Inner.String() + "?" }

// TypeUnion is `A | B | C`. Always two or more members; single-member
// "unions" are flattened to the bare member at parse time.
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

// TypeFunction is a function type: `(P1, P2) -> R` or `(P) -> (R1, R2)`.
// IsVararg+VarargType model `...: T` at the parameter list's tail.
// ParamNames is parallel to Params and may contain "" for unnamed slots.
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

// TypeTableField is one named entry in a TypeTable: `name: T`.
type TypeTableField struct {
	Key   string
	Value TypeNode
}

// TypeIndexer models `{[K]: V}` — a homogeneous map-like indexer.
type TypeIndexer struct {
	Key   TypeNode
	Value TypeNode
}

// TypeTable is a structural table type: `{ x: number, y: number }` or
// `{[string]: number}`. Either Fields or Indexer (or both) may be set.
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

// TypeAliasStatement is `type Name = T`. Aliases are top-level (Luau
// allows them only at chunk scope; we follow the same restriction in
// later phases by report-and-skip in the checker — the parser is
// permissive).
type TypeAliasStatement struct {
	BaseNode
	Name string
	// TypeParams is the optional generic parameter list: `type Box<T> = ...`
	// parses to TypeParams: ["T"]. Empty for non-generic aliases.
	TypeParams []string
	Target     TypeNode
}

func (*TypeAliasStatement) statementNode()         {}
func (s *TypeAliasStatement) TokenLiteral() string { return s.Token.Literal }
func (s *TypeAliasStatement) String() string {
	name := s.Name
	if len(s.TypeParams) > 0 {
		name += "<" + strings.Join(s.TypeParams, ", ") + ">"
	}
	return "type " + name + " = " + s.Target.String()
}

// TypeAssertionExpression is `expr :: T` — a programmer-controlled cast.
// The runtime is a no-op; the bytecode generator emits the inner Expr's
// code unchanged. The type checker treats the result as the asserted T.
type TypeAssertionExpression struct {
	BaseNode
	Expr Expression
	Type TypeNode
}

func (*TypeAssertionExpression) expressionNode()        {}
func (e *TypeAssertionExpression) TokenLiteral() string { return e.Token.Literal }
func (e *TypeAssertionExpression) String() string       { return e.Expr.String() + " :: " + e.Type.String() }
