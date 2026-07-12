// Package typecheck is the Luau-style gradual type checker for luascript.
//
// The pass runs between parser and bytecode generator. It consumes the AST
// (compiler/ast), builds an internal Type representation, walks the
// program building a scoped type environment, and emits a list of typed
// errors. The bytecode generator never sees Type values — types are erased
// before codegen, so the runtime is unchanged.
//
// Design intent: Luau-aligned. Untyped code is implicitly `any` (gradual
// escape hatch). `--!strict` tightens; `--!nocheck` skips this package
// entirely (the compiler glue handles that gate).
package typecheck

import (
	"strings"
)

// Kind tags the variant of Type. Only one of {Fn, Table, Union} is
// populated, gated by Kind.
type Kind int

const (
	KindNumber Kind = iota
	KindString
	KindBoolean
	KindNil
	// KindAny is the gradual escape hatch: bidirectionally compatible with
	// every other type. Code that doesn't carry annotations is `any` by
	// default, which is why the checker is lenient on un-typed Lua code.
	KindAny
	// KindUnknown is the safe top: any value flows in, but `unknown` only
	// flows back out via narrowing or explicit assertion. Refinements
	// aren't in v1, so most uses of `unknown` will require `:: T` to be
	// useful.
	KindUnknown
	// KindNever is the bottom: assigns to anything, nothing assigns to it.
	// Used as a placeholder for unresolvable aliases and for the result
	// type of `error("...")`.
	KindNever
	KindFunction
	KindTable
	KindUnion
	// KindTypeParam is a generic type variable (`T` inside `function f<T>`).
	// Within a generic body it behaves gradually — assignable to and from
	// anything, so opaque type-variable code never produces spurious errors.
	// Precision comes from *instantiation*: call-site inference substitutes a
	// concrete type for the variable and re-checks the result. The variable's
	// name is carried in AliasName for display and identity.
	KindTypeParam
)

// Type is the internal type representation. Constructed by the checker
// from AST type nodes; never reaches the bytecode generator.
type Type struct {
	Kind Kind

	// Populated by Kind:
	Fn    *FunctionShape // KindFunction
	Table *TableShape    // KindTable
	Union []*Type        // KindUnion (always ≥ 2 members; flattened, deduped)

	// AliasName preserves the user-written alias for diagnostics. Set when
	// resolving a TypeName; the structural Kind/Fn/Table fields are still
	// the source of truth for assignability.
	AliasName string
}

// FunctionShape is the parameter/return signature of a function type.
// IsVararg + VarargType model `(T, ...: U) -> R`. VarargType nil with
// IsVararg true means an untyped vararg (treated as any).
type FunctionShape struct {
	Params     []*Type
	Returns    []*Type
	IsVararg   bool
	VarargType *Type

	// TypeParams names the function's generic parameters (`f<T, U>`). Non-
	// empty for a generic function; call-site inference resolves them.
	TypeParams []string

	// Struct, when non-nil, marks this function as a struct constructor.
	// The checker then also accepts the single-table "named" call form
	// (`Point{ x = 1, y = 2 }`) in addition to the positional Params form.
	// Field is the ordered struct field list used to validate the named
	// form. FieldNames parallels Params for named-arg checking.
	Struct *StructCtor
}

// StructCtor carries the extra shape a struct constructor needs to validate
// the brace/named call form. Shape is the structural table each instance
// conforms to (also what `Name` resolves to as a type alias).
type StructCtor struct {
	Name  string
	Shape *TableShape
}

// TableShape is a structural table type. Fields are ordered for stable
// diagnostics. Indexer covers `{[K]: V}` and the array-shorthand `{T}`.
type TableShape struct {
	Fields  []TableField
	Indexer *Indexer
}

// TableField is one named entry in a TableShape.
type TableField struct {
	Key  string
	Type *Type
}

// Indexer covers the `{[K]: V}` form. nil if the table only has named
// fields.
type Indexer struct {
	Key   *Type
	Value *Type
}

// Singleton primitive types. Cheap pointer-equality where the same kind
// shows up repeatedly. Constructors below also reuse them.
var (
	numberT  = &Type{Kind: KindNumber}
	stringT  = &Type{Kind: KindString}
	booleanT = &Type{Kind: KindBoolean}
	nilT     = &Type{Kind: KindNil}
	anyT     = &Type{Kind: KindAny}
	unknownT = &Type{Kind: KindUnknown}
	neverT   = &Type{Kind: KindNever}
)

// primitiveByName maps a Luau-style primitive type name to its singleton.
// The set is closed; identifiers outside this set are treated as alias
// references during AST → Type conversion.
var primitiveByName = map[string]*Type{
	"number":  numberT,
	"string":  stringT,
	"boolean": booleanT,
	"nil":     nilT,
	"any":     anyT,
	"unknown": unknownT,
	"never":   neverT,
}

// PrimitiveByName returns the singleton primitive Type for `name`, or nil
// if `name` is not one of the closed-set primitives.
func PrimitiveByName(name string) *Type {
	return primitiveByName[name]
}

// NewUnion builds a union of `members`, flattening nested unions, deduping
// equal types, and short-circuiting to a singleton when the simplification
// leaves only one member. If `any` appears in the simplified set the result
// is `any` (the gradual sink absorbs everything).
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

// Optional builds `T | nil` via NewUnion.
func Optional(t *Type) *Type {
	return NewUnion(t, nilT)
}

// Same reports whether two types are structurally identical. Used by
// dedup, by invariant comparisons (e.g. table-field width subtyping at
// shared keys), and by alias-of-itself checks.
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
	// Field order isn't semantic; compare as sets keyed by name.
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
	// Order-insensitive: each member of a must equal some member of b.
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

// String renders a type in Luau-syntax-faithful form. AliasName takes
// priority so users see the name they wrote, not the structural expansion.
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

// NewFunction is a small convenience for stdlib_types.go.
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

// NewTable builds a TableShape Type. Either fields or indexer (or both)
// may be supplied; fields nil/empty + indexer nil makes an empty table.
func NewTable(fields []TableField, indexer *Indexer) *Type {
	return &Type{
		Kind:  KindTable,
		Table: &TableShape{Fields: fields, Indexer: indexer},
	}
}

// AnyT, NilT, NumberT, StringT, BooleanT, UnknownT, NeverT expose the
// singleton primitives. Used by package-external callers (stdlib_types.go,
// the REPL banner).
func AnyT() *Type {
	return anyT
}

func NilT() *Type {
	return nilT
}

func NumberT() *Type {
	return numberT
}

func StringT() *Type {
	return stringT
}

func BooleanT() *Type {
	return booleanT
}

func UnknownT() *Type {
	return unknownT
}

func NeverT() *Type {
	return neverT
}
