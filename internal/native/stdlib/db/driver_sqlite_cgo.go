//go:build luascript_sqlite_cgo

package db

// SQLite via cgo — registers the driver name "sqlite3".
//
//	go build -tags luascript_sqlite_cgo ./cmd/luascript
//
// Replaces the pure-Go backend in driver_sqlite.go rather than joining it:
// two SQLite implementations in one binary is pure weight, and resolveDriver
// maps the name "sqlite" onto whichever one is registered.
//
// This cannot be the default. mattn/go-sqlite3 has no non-cgo fallback — with
// CGO_ENABLED=0 the build fails outright with "build constraints exclude all
// Go files", so an unconditional import would break every toolchain-less
// build. Same reason the ui module is tag-gated: needing a C compiler is not
// something the default build can assume.
import _ "github.com/mattn/go-sqlite3"
