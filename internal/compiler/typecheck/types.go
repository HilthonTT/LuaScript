package typecheck

import (
	"strconv"
	"strings"
)

type Kind int

const (
	KindNumber Kind = iota
	KindString
	KindBoolean
	KindNil
	KindAny
	KindUnknown
	KindNever
	KindFunction
	KindTable
	KindUnion
	KindLiteral
	KindTypeParam
)

type Type struct {
	Kind Kind

	Fn    *FunctionShape
	Table *TableShape
	Union []*Type
	Lit   *LiteralValue

	AliasName string
}

type FunctionShape struct {
	Params     []*Type
	Returns    []*Type
	IsVararg   bool
	VarargType *Type

	TypeParams []string

	Struct *StructCtor
}

type StructCtor struct {
	Name  string
	Shape *TableShape
}

type LiteralValue struct {
	Base Kind
	Str  string
	Num  float64
	Bool bool
	Raw  string
}

type TableShape struct {
	Fields  []TableField
	Indexer *Indexer
}

type TableField struct {
	Key  string
	Type *Type
}

type Indexer struct {
	Key   *Type
	Value *Type
}

var (
	numberT  = &Type{Kind: KindNumber}
	stringT  = &Type{Kind: KindString}
	booleanT = &Type{Kind: KindBoolean}
	nilT     = &Type{Kind: KindNil}
	anyT     = &Type{Kind: KindAny}
	unknownT = &Type{Kind: KindUnknown}
	neverT   = &Type{Kind: KindNever}
)

var primitiveByName = map[string]*Type{
	"number":  numberT,
	"string":  stringT,
	"boolean": booleanT,
	"nil":     nilT,
	"any":     anyT,
	"unknown": unknownT,
	"never":   neverT,
}

func NewUnion(members ...*Type) *Type {
	flat := make([]*Type, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		if m.Kind == KindUnion {
			flat = append(flat, m.Union...)
		} else {
			flat = append(flat, m)
		}
	}
	dedup := flat[:0]
outer:
	for _, m := range flat {
		if m.Kind == KindAny {
			return anyT
		}
		for _, kept := range dedup {
			if Same(m, kept) {
				continue outer
			}
		}
		dedup = append(dedup, m)
	}
	switch len(dedup) {
	case 0:
		return neverT
	case 1:
		return dedup[0]
	}
	return &Type{Kind: KindUnion, Union: dedup}
}

func Optional(t *Type) *Type {
	return NewUnion(t, nilT)
}

func Same(a, b *Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindNumber, KindString, KindBoolean, KindNil,
		KindAny, KindUnknown, KindNever:
		return true
	case KindLiteral:
		return sameLiteral(a.Lit, b.Lit)
	case KindTypeParam:
		return a.AliasName == b.AliasName
	case KindFunction:
		return sameFunction(a.Fn, b.Fn)
	case KindTable:
		return sameTable(a.Table, b.Table)
	case KindUnion:
		return sameUnion(a.Union, b.Union)
	}
	return false
}

func sameLiteral(a, b *LiteralValue) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Base != b.Base {
		return false
	}
	switch a.Base {
	case KindString:
		return a.Str == b.Str
	case KindNumber:
		return a.Num == b.Num
	case KindBoolean:
		return a.Bool == b.Bool
	}
	return false
}

func sameFunction(a, b *FunctionShape) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Params) != len(b.Params) || len(a.Returns) != len(b.Returns) {
		return false
	}
	if a.IsVararg != b.IsVararg {
		return false
	}
	for i := range a.Params {
		if !Same(a.Params[i], b.Params[i]) {
			return false
		}
	}
	for i := range a.Returns {
		if !Same(a.Returns[i], b.Returns[i]) {
			return false
		}
	}
	if a.IsVararg && !Same(orAny(a.VarargType), orAny(b.VarargType)) {
		return false
	}
	return true
}

func sameTable(a, b *TableShape) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	bIdx := make(map[string]*Type, len(b.Fields))
	for _, f := range b.Fields {
		bIdx[f.Key] = f.Type
	}
	for _, f := range a.Fields {
		t, ok := bIdx[f.Key]
		if !ok || !Same(f.Type, t) {
			return false
		}
	}
	switch {
	case a.Indexer == nil && b.Indexer == nil:
		return true
	case a.Indexer == nil || b.Indexer == nil:
		return false
	}
	return Same(a.Indexer.Key, b.Indexer.Key) && Same(a.Indexer.Value, b.Indexer.Value)
}

func sameUnion(a, b []*Type) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
outer:
	for _, x := range a {
		for i, y := range b {
			if !used[i] && Same(x, y) {
				used[i] = true
				continue outer
			}
		}
		return false
	}
	return true
}

func orAny(t *Type) *Type {
	if t == nil {
		return anyT
	}
	return t
}

func (t *Type) String() string {
	if t == nil {
		return "?"
	}
	if t.AliasName != "" {
		return t.AliasName
	}
	switch t.Kind {
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindBoolean:
		return "boolean"
	case KindNil:
		return "nil"
	case KindAny:
		return "any"
	case KindUnknown:
		return "unknown"
	case KindNever:
		return "never"
	case KindLiteral:
		return formatLiteral(t.Lit)
	case KindTypeParam:
		if t.AliasName != "" {
			return t.AliasName
		}
		return "?T"
	case KindFunction:
		return formatFunction(t.Fn)
	case KindTable:
		return formatTable(t.Table)
	case KindUnion:
		parts := make([]string, len(t.Union))
		for i, m := range t.Union {
			parts[i] = m.String()
		}
		return strings.Join(parts, " | ")
	}
	return "<invalid>"
}

func formatLiteral(l *LiteralValue) string {
	if l == nil {
		return "?"
	}
	if l.Raw != "" {
		return l.Raw
	}
	switch l.Base {
	case KindString:
		return strconv.Quote(l.Str)
	case KindNumber:
		return strconv.FormatFloat(l.Num, 'g', -1, 64)
	case KindBoolean:
		return strconv.FormatBool(l.Bool)
	}
	return "?"
}

func formatFunction(f *FunctionShape) string {
	if f == nil {
		return "function"
	}
	parts := make([]string, 0, len(f.Params)+1)
	for _, p := range f.Params {
		parts = append(parts, p.String())
	}
	if f.IsVararg {
		if f.VarargType != nil {
			parts = append(parts, "...: "+f.VarargType.String())
		} else {
			parts = append(parts, "...")
		}
	}
	var b strings.Builder
	b.WriteString("(")
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(") -> ")
	switch len(f.Returns) {
	case 0:
		b.WriteString("()")
	case 1:
		b.WriteString(f.Returns[0].String())
	default:
		rets := make([]string, len(f.Returns))
		for i, r := range f.Returns {
			rets[i] = r.String()
		}
		b.WriteString("(")
		b.WriteString(strings.Join(rets, ", "))
		b.WriteString(")")
	}
	return b.String()
}

func formatTable(t *TableShape) string {
	if t == nil {
		return "{}"
	}
	parts := make([]string, 0, len(t.Fields)+1)
	if t.Indexer != nil {
		parts = append(parts, "["+t.Indexer.Key.String()+"]: "+t.Indexer.Value.String())
	}
	for _, f := range t.Fields {
		parts = append(parts, f.Key+": "+f.Type.String())
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func NewFunction(params []*Type, returns []*Type, isVararg bool, varargType *Type) *Type {
	return &Type{
		Kind: KindFunction,
		Fn: &FunctionShape{
			Params:     params,
			Returns:    returns,
			IsVararg:   isVararg,
			VarargType: varargType,
		},
	}
}

func NewTable(fields []TableField, indexer *Indexer) *Type {
	return &Type{
		Kind:  KindTable,
		Table: &TableShape{Fields: fields, Indexer: indexer},
	}
}

func NewStringLiteral(v, raw string) *Type {
	return &Type{Kind: KindLiteral, Lit: &LiteralValue{Base: KindString, Str: v, Raw: raw}}
}

func NewNumberLiteral(v float64, raw string) *Type {
	return &Type{Kind: KindLiteral, Lit: &LiteralValue{Base: KindNumber, Num: v, Raw: raw}}
}

func NewBooleanLiteral(v bool, raw string) *Type {
	return &Type{Kind: KindLiteral, Lit: &LiteralValue{Base: KindBoolean, Bool: v, Raw: raw}}
}

func baseKind(t *Type) Kind {
	if t != nil && t.Kind == KindLiteral && t.Lit != nil {
		return t.Lit.Base
	}
	if t == nil {
		return KindAny
	}
	return t.Kind
}

func widen(t *Type) *Type {
	if t == nil {
		return t
	}
	switch t.Kind {
	case KindLiteral:
		if p := primitiveForKind(baseKind(t)); p != nil {
			return p
		}
		return t
	case KindUnion:
		out := make([]*Type, 0, len(t.Union))
		changed := false
		for _, m := range t.Union {
			w := widen(m)
			if w != m {
				changed = true
			}
			out = append(out, w)
		}
		if !changed {
			return t
		}
		return NewUnion(out...)
	}
	return t
}

func literalMembers(t *Type) ([]*LiteralValue, bool) {
	if t == nil {
		return nil, false
	}
	switch t.Kind {
	case KindLiteral:
		if t.Lit == nil {
			return nil, false
		}
		return []*LiteralValue{t.Lit}, true
	case KindUnion:
		out := make([]*LiteralValue, 0, len(t.Union))
		for _, m := range t.Union {
			if m.Kind != KindLiteral || m.Lit == nil {
				return nil, false
			}
			out = append(out, m.Lit)
		}
		return out, len(out) > 0
	}
	return nil, false
}
