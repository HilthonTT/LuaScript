package typecheck

func assignable(from, to *Type) bool {
	if from == nil || to == nil {
		return true
	}

	if from.Kind == KindAny || to.Kind == KindAny {
		return true
	}

	if from.Kind == KindTypeParam || to.Kind == KindTypeParam {
		if from.Kind == KindTypeParam && to.Kind == KindTypeParam {
			return from.AliasName == to.AliasName
		}
		return true
	}

	if from.Kind == KindNever {
		return true
	}

	if to.Kind == KindUnknown {
		return true
	}
	if from.Kind == KindUnknown {
		return false
	}

	if from.Kind == KindUnion {
		for _, m := range from.Union {
			if !assignable(m, to) {
				return false
			}
		}
		return true
	}

	if to.Kind == KindUnion {
		for _, m := range to.Union {
			if assignable(from, m) {
				return true
			}
		}
		return false
	}

	if from.Kind == KindLiteral {
		if to.Kind == KindLiteral {
			return sameLiteral(from.Lit, to.Lit)
		}
		return baseKind(from) == to.Kind
	}
	if to.Kind == KindLiteral {
		return false
	}

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

func assignableFunction(from, to *FunctionShape) bool {
	if from == nil || to == nil {
		return from == to
	}

	switch {
	case from.IsVararg:
		if len(from.Params) > len(to.Params) {
			return false
		}
		for i, p := range from.Params {
			if !assignable(to.Params[i], p) {
				return false
			}
		}
		va := orAny(from.VarargType)
		for _, p := range to.Params[len(from.Params):] {
			if !assignable(p, va) {
				return false
			}
		}
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
			if !assignable(to.Params[i], p) {
				return false
			}
		}
		if to.IsVararg {
			return false
		}
	}

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
			if from.Indexer != nil &&
				assignable(stringT, from.Indexer.Key) &&
				assignable(from.Indexer.Value, f.Type) {
				continue
			}
			return false
		}
		if !Same(got, f.Type) {
			return false
		}
	}

	if to.Indexer != nil {
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
