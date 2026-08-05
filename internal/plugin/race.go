//go:build !race

package plugin

// raceEnabled reports whether this binary was built with the race detector.
//
// A Go plugin must be built with the same build configuration as the process
// loading it — `-race` changes `internal/race`, so a race-enabled host opening
// a plugin built without it is rejected by plugin.Open with "plugin was built
// with a different version of package internal/race". It is the same class of
// constraint as the Go version pinned in goMod. buildPlugin therefore forwards
// `-race` to the plugin build and keeps race artifacts in a separate cache
// directory. Unused on platforms that compile backend_stub.go.
const raceEnabled = false
