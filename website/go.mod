// The media generator (scripts/media) lives in its own module so its
// rendering dependencies never reach the interpreter's go.mod, and so
// `go test ./...` at the repo root skips website/ (and its node_modules).
module github.com/hilthontt/luascript/website

go 1.27

require (
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	golang.org/x/image v0.47.0
)

require (
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
