package typecheck

// assignable reports whether a value of type `from` can flow into a slot
// of type `to`. Aligned with Luau's gradual rules:
//
//   - `any` is the gradual escape hatch: bidirectionally compatible.
//   - `unknown` is the safe top: anything assigns to unknown; unknown only
//     flows back via narrowing or `:: T` (refinements aren't in v1).
//   - `never` is the bottom: assigns to anything; nothing assigns to it.
//   - Primitives match by identical kind only — no implicit number↔string
//     coercion (matches Lua's strict-equality semantics).
//   - Optional `T?` ≡ `T | nil`; `nil` flows to any union containing `nil`.
//   - Union from-side: every member of `from` must flow to `to`.
//   - Union to-side: there must exist some member of `to` that `from`
//     flows to.
//   - Functions: contravariant params, covariant returns. Arity must match
//     unless the supertype is vararg (extra args are absorbed there).
//   - Tables: width subtyping. Supertype's fields must be a subset of
//     subtype's; shared fields are invariantly equal. Indexer-on-supertype
//     requires the indexer's value to be assignable from each subtype
//     field's value.
func assignable(from, to *Type) bool {
	if from == nil || to == nil {
		// Defensive: missing type info shouldn't crash the checker. Treat
		// missing as `any` — gradual fallback.
		return true
	}

	// `any` (gradual) — compatible in both directions.
	if from.Kind == KindAny || to.Kind == KindAny {
		return true
	}

	// A generic type variable is opaque inside its body: treat it gradually
	// (compatible both directions) so type-variable code never mis-reports,
	// EXCEPT that two *named* variables must match by name so `T` and `U`
	// stay distinct.
	if from.Kind == KindTypeParam || to.Kind == KindTypeParam {
		if from.Kind == KindTypeParam && to.Kind == KindTypeParam {
			return from.AliasName == to.AliasName
		}
		return true
	}

	// `never` flows to everything.
	if from.Kind == KindNever {
		return true
	}

	// `unknown` accepts everything (top); only `unknown` flows back out.
	if to.Kind == KindUnknown {
		return true
	}
	if from.Kind == KindUnknown {
		return false
	}

	// Union on the from-side: every member must flow.
	if from.Kind == KindUnion {
		for _, m := range from.Union {
			if !assignable(m, to) {
				return false
			}
		}
		return true
	}

	// Union on the to-side: some member must accept.
	if to.Kind == KindUnion {
		for _, m := range to.Union {
			if assignable(from, m) {
				return true
			}
		}
		return false
	}

	// Same primitive kind.
	if from.Kind == to.Kind {
		switch from.Kind {
		case KindNumber, KindString, KindBoolean, KindNil:
			return true
		case KindFunction:
			return assignableFunction(from.Fn, to.Fn)
		case KindTable:
			return assignableTable(from.Table, to.Table)
		}
	}

	return false
}

// assignableFunction checks contravariant params and covariant returns.
// Arity rules: subtype's param count must equal supertype's (the function
// is invoked through `to`, so callers pass `to.Params`-many args; `from`
// must accept exactly that many, unless `from` is vararg in which case it
// accepts any tail).
func assignableFunction(from, to *FunctionShape) bool {
	if from == nil || to == nil {
		return from == to
	}

	// Param count.
	switch {
	case from.IsVararg:
		// from accepts >=0 trailing args via vararg; the fixed prefix must
		// not exceed to's param count, and the named prefix must accept
		// to's same-position params.
		if len(from.Params) > len(to.Params) {
			return false
		}
		for i, p := range from.Params {
			if !assignable(to.Params[i], p) { // contravariant
				return false
			}
		}
		// remaining to.Params consumed by from's vararg
		va := orAny(from.VarargType)
		for _, p := range to.Params[len(from.Params):] {
			if !assignable(p, va) {
				return false
			}
		}
		// to's vararg type must flow to from's vararg type.
		if to.IsVararg {
			if !assignable(orAny(to.VarargType), va) {
				return false
			}
		}
	default:
		if len(from.Params) != len(to.Params) {
			return false
		}
		for i, p := range from.Params {
			if !assignable(to.Params[i], p) { // contravariant
				return false
			}
		}
		// to is vararg but from isn't → from can't absorb the trailing
		// args, so reject.
		if to.IsVararg {
			return false
		}
	}

	// Returns: covariant. If from returns more values than to expects,
	// that's fine (extra results are discarded by Lua's calling
	// convention). The reverse is unsafe: callers expecting N results
	// from `to` can't be fed by a `from` that returns fewer.
	//
	// An empty return list means "no declared returns", which for
	// unannotated functions is "unknown", not "returns nothing" —
	// typeOfCall already treats their call results as `any`. Mirror that
	// gradual stance so `function(a, b) return a > b end` flows into a
	// slot typed `(any, any) -> boolean`.
	if len(from.Returns) == 0 {
		return true
	}
	if len(from.Returns) < len(to.Returns) {
		return false
	}
	for i, r := range to.Returns {
		if !assignable(from.Returns[i], r) {
			return false
		}
	}
	return true
}

// assignableTable enforces width subtyping at the field level: every
// field declared on the supertype must exist on the subtype, with an
// invariantly-equal value type. Indexer rules:
//
//   - If `to` has an indexer: every field on `from` whose key isn't
//     covered by `to`'s named fields must have a value assignable to
//     the indexer's value, and any indexer-on-`from` must agree.
//   - If `to` has no indexer: extra fields on `from` are silently
//     accepted (width subtyping).
func assignableTable(from, to *TableShape) bool {
	if from == nil || to == nil {
		return from == to
	}

	fromIdx := make(map[string]*Type, len(from.Fields))
	for _, f := range from.Fields {
		fromIdx[f.Key] = f.Type
	}

	for _, f := range to.Fields {
		got, ok := fromIdx[f.Key]
		if !ok {
			// Subtype is missing a required field. As a fallback, allow
			// `from`'s indexer to satisfy the slot.
			if from.Indexer != nil &&
				assignable(stringT, from.Indexer.Key) &&
				assignable(from.Indexer.Value, f.Type) {
				continue
			}
			return false
		}
		if !Same(got, f.Type) {
			// Invariance at shared keys. We could relax to bivariance for
			// `any`-touching cases, but Same already accounts for `any`
			// via the structural check at this level. Strict for v1.
			return false
		}
	}

	if to.Indexer != nil {
		// Every from-field whose key isn't covered by `to`'s named fields
		// must satisfy the indexer's value type.
		toFieldNames := make(map[string]bool, len(to.Fields))
		for _, f := range to.Fields {
			toFieldNames[f.Key] = true
		}
		for _, f := range from.Fields {
			if toFieldNames[f.Key] {
				continue
			}
			if !assignable(f.Type, to.Indexer.Value) {
				return false
			}
		}
		if from.Indexer != nil {
			if !assignable(from.Indexer.Key, to.Indexer.Key) ||
				!assignable(from.Indexer.Value, to.Indexer.Value) {
				return false
			}
		}
	}
	return true
}
