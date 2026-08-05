//go:build !race

package vm

// maxCallDepth bounds the number of nested Lua call frames. Every LuaScript
// call consumes real Go stack (there is no tail-call elimination), so runaway
// or infinite recursion would otherwise blow the goroutine stack and trigger a
// fatal, pcall-uncatchable `stack overflow`. Empirically the Go stack overflows
// somewhere past ~400k frames on the reference build; 200k leaves a comfortable
// margin while sitting far above any legitimate non-tail recursion. Hitting it
// raises an ordinary (catchable) LuaError, matching Lua 5.4's "stack overflow".
//
// The limit is build-tagged because the safe value depends on how much Go stack
// one Lua call costs, and the race detector changes that — see calldepth_race.go.
const maxCallDepth = 200_000
