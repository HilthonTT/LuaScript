package db

import (
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

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

func availableDrivers() []string {
	out := append([]string(nil), sql.Drivers()...)
	sort.Strings(out)
	return out
}

type placeholderStyle int

const (
	stylePositional placeholderStyle = iota
	styleDollar
	styleAtP
)

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

func isSQLite(driver string) bool {
	return driver == "sqlite" || driver == "sqlite3"
}

func isMemoryDSN(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.Contains(lower, ":memory:") || strings.Contains(lower, "mode=memory")
}
