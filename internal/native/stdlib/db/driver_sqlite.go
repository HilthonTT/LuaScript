//go:build !luascript_sqlite_cgo

package db

// SQLite, pure Go — registers the driver name "sqlite".
//
//	db.open("sqlite", "app.db")
//	db.open("sqlite", ":memory:")
//
// This is the default SQLite backend because it needs no cgo and therefore no
// C toolchain, which matters most on Windows. It is a machine translation of
// the SQLite C source, so it is larger and somewhat slower than the cgo
// binding; build with -tags luascript_sqlite_cgo to swap in mattn/go-sqlite3
// instead (see driver_sqlite_cgo.go).
//
// Scripts should say "sqlite" and let resolveDriver pick whichever backend
// this binary was built with.
import _ "modernc.org/sqlite"
