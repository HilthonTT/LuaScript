package db

import (
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// This file is everything the module knows about *which* database it is
// talking to. The driver packages themselves are blank-imported from the
// driver_*.go companions; nothing here imports them, so the resolution and
// placeholder logic is testable without a database.

// driverCandidates maps a name a script might reasonably write to the
// database/sql driver names that could serve it, in preference order.
//
// The indirection exists because the registered name is the driver author's
// choice, not ours, and two SQLite drivers ship under different names:
// mattn/go-sqlite3 registers "sqlite3", modernc.org/sqlite registers "sqlite".
// A script that says "sqlite" should work against whichever one this binary
// was built with, so the lookup resolves against what is actually registered
// rather than hard-coding either.
//
// Aliases are resolved here rather than through sql.Register because
// registering the same driver twice under two names panics.
var driverCandidates = map[string][]string{
	"postgres":   {"postgres", "pgx"},
	"postgresql": {"postgres", "pgx"},
	"pg":         {"postgres", "pgx"},
	"mysql":      {"mysql"},
	"mariadb":    {"mysql"},
	"sqlite":     {"sqlite3", "sqlite"},
	"sqlite3":    {"sqlite3", "sqlite"},
	"sqlserver":  {"sqlserver", "mssql"},
	"mssql":      {"sqlserver", "mssql"},
}

// resolveDriver maps a user-supplied driver name onto a registered
// database/sql driver. An exact match always wins, so a driver this package
// has never heard of — one a host registered itself — still works.
func resolveDriver(name string) (string, error) {
	registered := sql.Drivers()
	if slices.Contains(registered, name) {
		return name, nil
	}
	for _, candidate := range driverCandidates[strings.ToLower(name)] {
		if slices.Contains(registered, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unknown driver %q (available: %s)", name, strings.Join(availableDrivers(), ", "))
}

// availableDrivers lists the registered driver names, sorted.
func availableDrivers() []string {
	out := append([]string(nil), sql.Drivers()...)
	sort.Strings(out)
	return out
}

// placeholderStyle is how a driver spells the nth bind parameter. This is the
// one syntactic difference that stops the same SQL string from running against
// two databases, so the module reports it rather than leaving callers to guess.
type placeholderStyle int

const (
	stylePositional placeholderStyle = iota // ?   — mysql, sqlite
	styleDollar                             // $1  — postgres
	styleAtP                                // @p1 — sqlserver
)

// styleFor returns the placeholder style for a resolved driver name. Unknown
// drivers get the positional style, which is by far the most common.
func styleFor(driver string) placeholderStyle {
	switch driver {
	case "postgres", "pgx":
		return styleDollar
	case "sqlserver", "mssql":
		return styleAtP
	default:
		return stylePositional
	}
}

// placeholder renders the nth (1-based) bind parameter for a driver.
func placeholder(driver string, n int) string {
	switch styleFor(driver) {
	case styleDollar:
		return "$" + strconv.Itoa(n)
	case styleAtP:
		return "@p" + strconv.Itoa(n)
	default:
		return "?"
	}
}

// isSQLite reports whether a resolved driver name is one of the SQLite
// drivers, which need the single-connection treatment below.
func isSQLite(driver string) bool {
	return driver == "sqlite" || driver == "sqlite3"
}

// isMemoryDSN reports whether a SQLite DSN names an in-memory database.
//
// This matters because *sql.DB is a connection pool and every SQLite
// in-memory connection gets its own private database. A pool of two would
// silently hand a script an empty database on the second query — the classic
// "my table vanished" bug. dbOpen pins such handles to one connection.
func isMemoryDSN(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.Contains(lower, ":memory:") || strings.Contains(lower, "mode=memory")
}
