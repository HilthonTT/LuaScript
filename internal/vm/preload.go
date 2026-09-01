package vm

func RegisterPreload(v *VM, name string, loader func(*VM, []Value) []Value) {
	pkg, ok := v.Globals.Get("package").(*Table)
	if !ok {
		return
	}
	preload, ok := pkg.Get("preload").(*Table)
	if !ok {
		preload = NewTable(0, 4)
		pkg.Set("preload", preload)
	}
	preload.Set(name, &GoFunc{Name: "preload." + name, Fn: loader})
}

func PromoteToGlobal(v *VM, name string) {
	pkg, ok := v.Globals.Get("package").(*Table)
	if !ok {
		return
	}
	preload, ok := pkg.Get("preload").(*Table)
	if !ok || preload.Get(name) == nil {
		return
	}
	results := builtinRequire(v, []Value{name})
	if len(results) > 0 && results[0] != nil {
		v.Globals.Set(name, results[0])
	}
}
