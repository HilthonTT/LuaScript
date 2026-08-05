//go:build race

package vm

// maxCallDepth — race-detector build.
//
// The guard only works if it fires *before* the Go stack dies, and the race
// detector moves that line. Its instrumentation inflates every frame in the
// dispatch → callClosure → exec → doCall cycle, taking one Lua call from
// roughly 1.3 KB of Go stack to roughly 3.4 KB. The runtime's 1 GB goroutine
// stack limit is hit while doubling a 512 MB stack, so the real ceiling under
// `-race` is around 158k frames — below the 200k production limit, which meant
// `go test -race ./internal/vm/` died with a fatal, uncatchable
// `stack overflow` instead of raising the catchable LuaError that
// TestDeepRecursionInsideTryIsCatchable expects.
//
// 50k keeps the same ~3x margin the production value has (~170 MB of Go stack)
// and is still far above any legitimate non-tail recursion, so the tests that
// exercise the guard behave identically on both builds.
const maxCallDepth = 50_000
