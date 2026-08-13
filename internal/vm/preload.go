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

// PromoteToGlobal loads a preloaded module and binds it to the global of the
// same name, so `io.open(...)` works without a `local io = require("io")`
// line first.
//
// Lua 5.4 ships os / io / utf8 as globals, but this runtime implements them as
// native modules (they live outside internal/vm, which cannot import them), so
// they were reachable only through require. Scripts written against the
// reference manual failed on their first line.
//
// The module is routed through require rather than invoked directly, so
// package.loaded is populated the same way and a later require(name) returns
// this exact table instead of building a second copy. A name with no preload
// entry is a silent no-op: registrars are what put entries there, and a VM
// that skipped one should not fail at startup. An existing global is
// overwritten — that is the point for `io`, where the core stdlib installs a
// two-function placeholder that the full module supersedes.
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
