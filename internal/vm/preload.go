package vm

// RegisterPreload installs a host loader under `package.preload[name]`.
// The first `require(name)` invokes `loader` and caches the result in
// `package.loaded`; later requires return the cache.
//
// Every native module went through the same 12-line dance of fetching
// package / preload / creating a GoFunc — this helper centralizes it so
// each module's Register* function is a one-liner.
//
// If `package` is missing (the loader subsystem hasn't been wired into
// this VM), the call is a silent no-op — matching the pre-existing
// behavior of each native module's Register*Preload.
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
