package db

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"
)

// These are the parts of multi-driver support that need no database: name
// resolution, bind-parameter syntax, and the value coercion that makes one
// driver's results look like another's.

func TestResolveDriverPrefersAnExactMatch(t *testing.T) {
	// Every driver this binary ships must resolve to itself, whatever else
	// the alias table says.
	for _, name := range sql.Drivers() {
		got, err := resolveDriver(name)
		if err != nil {
			t.Errorf("resolveDriver(%q) failed: %v", name, err)
			continue
		}
		if got != name {
			t.Errorf("resolveDriver(%q) = %q, want the exact name back", name, got)
		}
	}
}

func TestResolveDriverAliases(t *testing.T) {
	cases := []struct {
		alias string
		want  string
	}{
		{"pg", "postgres"},
		{"postgresql", "postgres"},
		{"POSTGRES", "postgres"},
		{"mariadb", "mysql"},
		// go-mssqldb registers BOTH "sqlserver" and "mssql", and they are
		// not interchangeable: "sqlserver" parses URL-style DSNs, "mssql"
		// the legacy ADO-style ones. Exact match therefore wins here, which
		// is what a caller who typed the legacy name wants.
		{"mssql", "mssql"},
		{"MSSQL", "sqlserver"},
	}
	for _, tc := range cases {
		got, err := resolveDriver(tc.alias)
		if err != nil {
			t.Errorf("resolveDriver(%q) failed: %v", tc.alias, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveDriver(%q) = %q, want %q", tc.alias, got, tc.want)
		}
	}
}

// TestResolveDriverPicksWhicheverSQLiteIsBuiltIn is the point of the
// candidate list: "sqlite" and "sqlite3" both work regardless of which
// backend the build tag selected.
func TestResolveDriverPicksWhicheverSQLiteIsBuiltIn(t *testing.T) {
	for _, name := range []string{"sqlite", "sqlite3"} {
		got, err := resolveDriver(name)
		if err != nil {
			t.Fatalf("resolveDriver(%q) failed: %v", name, err)
		}
		if !isSQLite(got) {
			t.Errorf("resolveDriver(%q) = %q, want a SQLite driver", name, got)
		}
		if !slices.Contains(sql.Drivers(), got) {
			t.Errorf("resolveDriver(%q) = %q, which is not registered", name, got)
		}
	}
}

func TestResolveDriverRejectsUnknownAndNamesAlternatives(t *testing.T) {
	_, err := resolveDriver("oracle")
	if err == nil {
		t.Fatal("resolveDriver accepted an unregistered driver")
	}
	// The message has to be actionable: a user who guessed wrong needs to
	// see what this binary does have.
	for _, want := range []string{"oracle", "postgres"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestPlaceholderStylePerDriver(t *testing.T) {
	cases := []struct {
		driver string
		n      int
		want   string
	}{
		{"postgres", 1, "$1"},
		{"postgres", 12, "$12"},
		{"pgx", 3, "$3"},
		{"mysql", 1, "?"},
		{"mysql", 7, "?"},
		{"sqlite", 2, "?"},
		{"sqlite3", 2, "?"},
		{"sqlserver", 1, "@p1"},
		{"mssql", 4, "@p4"},
		// An unknown driver gets the commonest style rather than an error:
		// the caller is building SQL, not opening a connection.
		{"somethingelse", 1, "?"},
	}
	for _, tc := range cases {
		if got := placeholder(tc.driver, tc.n); got != tc.want {
			t.Errorf("placeholder(%q, %d) = %q, want %q", tc.driver, tc.n, got, tc.want)
		}
	}
}

func TestIsMemoryDSN(t *testing.T) {
	memory := []string{":memory:", "file::memory:?cache=shared", "file:x?mode=memory", "MODE=MEMORY"}
	disk := []string{"app.db", "file:app.db?cache=shared", "/var/lib/app.sqlite", ""}
	for _, dsn := range memory {
		if !isMemoryDSN(dsn) {
			t.Errorf("isMemoryDSN(%q) = false, want true", dsn)
		}
	}
	for _, dsn := range disk {
		if isMemoryDSN(dsn) {
			t.Errorf("isMemoryDSN(%q) = true, want false", dsn)
		}
	}
}

// TestCoerceColumnRestoresNumbersFromBytes covers the MySQL shape: the driver
// returns wire bytes for every column, and without the declared type a number
// column would reach Lua as a string.
func TestCoerceColumnRestoresNumbersFromBytes(t *testing.T) {
	cases := []struct {
		dbType string
		raw    any
		want   any
	}{
		{"INT", []byte("42"), int64(42)},
		{"BIGINT", []byte("-9007199254740993"), int64(-9007199254740993)},
		{"TINYINT", []byte("1"), int64(1)},
		{"SERIAL", []byte("7"), int64(7)},
		{"DOUBLE", []byte("3.5"), 3.5},
		{"FLOAT", []byte("-0.25"), -0.25},
		{"BOOL", []byte("1"), true},
		{"BOOLEAN", []byte("f"), false},
		// Text stays text.
		{"VARCHAR", []byte("42"), "42"},
		{"TEXT", []byte("hello"), "hello"},
		// Exact types keep full precision as strings rather than losing
		// digits to float64.
		{"DECIMAL", []byte("12345678901234567890.123"), "12345678901234567890.123"},
		{"NUMERIC", []byte("0.10"), "0.10"},
		// A declared type whose bytes don't parse falls back rather than
		// inventing a value.
		{"INT", []byte("not a number"), "not a number"},
		// No type information: the untyped mapping.
		{"", []byte("42"), "42"},
	}
	for _, tc := range cases {
		got := coerceColumn(tc.dbType, tc.raw)
		if got != tc.want {
			t.Errorf("coerceColumn(%q, %q) = %#v, want %#v", tc.dbType, tc.raw, got, tc.want)
		}
	}
}

// TestCoerceColumnLeavesTypedValuesAlone: drivers that already return proper
// Go types (lib/pq, the SQLite drivers) must pass through untouched.
func TestCoerceColumnLeavesTypedValuesAlone(t *testing.T) {
	when := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		dbType string
		in     any
		want   any
	}{
		{"INT", int64(42), int64(42)},
		{"DOUBLE", 3.5, 3.5},
		{"BOOL", true, true},
		{"TEXT", "hello", "hello"},
		{"INT", nil, nil},
		{"TIMESTAMP", when, when.Format(time.RFC3339Nano)},
	}
	for _, tc := range cases {
		if got := coerceColumn(tc.dbType, tc.in); got != tc.want {
			t.Errorf("coerceColumn(%q, %#v) = %#v, want %#v", tc.dbType, tc.in, got, tc.want)
		}
	}
}

func TestAvailableDriversIsSorted(t *testing.T) {
	got := availableDrivers()
	if !slices.IsSorted(got) {
		t.Errorf("availableDrivers() = %v, want sorted", got)
	}
	if len(got) != len(sql.Drivers()) {
		t.Errorf("availableDrivers() dropped entries: %v vs %v", got, sql.Drivers())
	}
}

// TestExpectedDriversAreCompiledIn guards the point of this whole change: the
// bundled interpreter must actually be able to reach these databases.
func TestExpectedDriversAreCompiledIn(t *testing.T) {
	for _, name := range []string{"postgres", "mysql", "sqlserver", "sqlite"} {
		if _, err := resolveDriver(name); err != nil {
			t.Errorf("driver %q is not available: %v", name, err)
		}
	}
}
