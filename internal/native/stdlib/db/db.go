// Package db is the host-side bridge between .lsc code and Go's database/sql.
//
// The package itself is driver-agnostic: sql.Open takes whatever name is
// registered, so the only thing that decides which databases work is which
// driver packages are blank-imported. Those live in the driver_*.go companions
// — one file per driver, so adding another is a new file and nothing else.
// Shipped by default: PostgreSQL (lib/pq), MySQL/MariaDB (go-sql-driver),
// SQL Server (go-mssqldb) and SQLite (modernc, pure Go). Building with
// -tags luascript_sqlite_cgo swaps the SQLite backend for mattn/go-sqlite3.
//
// What is NOT driver-agnostic, and what this package therefore has to know
// about, is collected in drivers.go: bind-parameter syntax, driver-name
// aliases, and SQLite's in-memory pooling trap.
package db

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterDBPreload installs a single loader entry. The loader is a
// GoFunc that returns the module table — `require` runs it on first
// call, caches the result in `package.loaded`, and returns the cache
// on every subsequent `require("db")`.
func RegisterDBPreload(v *vm.VM) {
	vm.RegisterPreload(v, "db", dbLoader)
}

func dbLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 4)
	mod.Set("open", &vm.GoFunc{Name: "db.open", Fn: dbOpen})
	mod.Set("drivers", &vm.GoFunc{Name: "db.drivers", Fn: dbDrivers})
	mod.Set("placeholder", &vm.GoFunc{Name: "db.placeholder", Fn: dbPlaceholder})
	mod.Set("VERSION", "0.2.0")
	return []vm.Value{mod}
}

// dbDrivers lists the database/sql driver names compiled into this binary.
// With several drivers now in play, a script (or a REPL user) needs a way to
// ask what is actually available rather than discovering it from an open
// failure.
func dbDrivers(_ *vm.VM, _ []vm.Value) []vm.Value {
	out := vm.NewTable(len(sql.Drivers()), 0)
	for _, name := range availableDrivers() {
		out.Append(name)
	}
	return []vm.Value{out}
}

// dbPlaceholder reports how a driver spells its nth bind parameter:
// "?" for MySQL and SQLite, "$1" for Postgres, "@p1" for SQL Server.
//
// This is the one difference that stops the same SQL string running against
// two databases, and it is deliberately reported rather than papered over —
// rewriting a caller's SQL means parsing it, and getting that subtly wrong is
// worse than making the difference visible.
func dbPlaceholder(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg("db.placeholder", 1, args)
	n := vm.OptInt("db.placeholder", 2, args, 1)
	if n < 1 {
		panic(vm.Errorf("db.placeholder: index must be >= 1, got %d", n))
	}
	// Resolve when possible so an alias ("pg") answers for its real driver,
	// but still answer for a driver that is not compiled in — a script may
	// be generating SQL for a database it is not connected to.
	resolved, err := resolveDriver(name)
	if err != nil {
		resolved = strings.ToLower(name)
	}
	return []vm.Value{placeholder(resolved, int(n))}
}

// dbOpen is the constructor: `db.open(driver, dsn)` -> connection table.
//
// The driver name is resolved through resolveDriver, so aliases ("pg",
// "postgresql", "mariadb") and the two SQLite spellings all land on whatever
// this binary actually has.
//
// On error we panic with a vm.LuaError so the .lsc side can catch it with
// pcall(); the successful return value is a connection table whose methods
// close over the underlying *sql.DB.
func dbOpen(_ *vm.VM, args []vm.Value) []vm.Value {
	driverName := vm.StringArg("db.open", 1, args)
	dataSource := vm.StringArg("db.open", 2, args)

	resolved, err := resolveDriver(driverName)
	if err != nil {
		panic(vm.Errorf("db.open: %s", err.Error()))
	}

	handle, err := sql.Open(resolved, dataSource)
	if err != nil {
		panic(vm.Errorf("db.open: %s", err.Error()))
	}
	// A SQLite in-memory database is private to its connection, and *sql.DB
	// is a pool — so a second pooled connection would see an empty database
	// and the script's tables would appear to vanish. Pin the pool to one
	// connection; the VM runs Lua on a single goroutine anyway, so this
	// costs no concurrency that was reachable in the first place.
	if isSQLite(resolved) && isMemoryDSN(dataSource) {
		handle.SetMaxOpenConns(1)
	}
	// sql.Open is lazy — it doesn't actually contact the server. Ping
	// here so the caller learns about a bad DSN immediately rather than
	// on the first query.
	if err := handle.Ping(); err != nil {
		_ = handle.Close()
		panic(vm.Errorf("db.open: %s", err.Error()))
	}
	return []vm.Value{newConn(handle, resolved)}
}

// newConn builds a fresh connection table per call. Methods are GoFuncs
// that close over `handle`, so the *sql.DB never leaks into Lua memory
// and can only be reached through the methods we expose here.
//
// driver is the resolved database/sql name; it is exposed on the connection
// so a script that has to care about dialect differences can branch on it.
func newConn(handle *sql.DB, driver string) *vm.Table {
	conn := vm.NewTable(0, 2)
	conn.Set("driver", driver)

	methods := vm.NewTable(0, 6)

	methods.Set("exec", &vm.GoFunc{Name: "db:exec", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		// a[0] = self (ignored — handle is captured), a[1] = SQL,
		// a[2:] = bind parameters.
		query := vm.StringArg("db:exec", 2, a)
		bindArgs := toDriverArgs(a[2:])

		res, err := handle.Exec(query, bindArgs...)
		if err != nil {
			panic(vm.Errorf("db:exec: %s", err.Error()))
		}
		// Both results are best-effort across drivers: Postgres supports
		// neither LastInsertId (use RETURNING) nor, for some statements,
		// RowsAffected. Surface 0 on "not supported" rather than blowing
		// up, so one dialect's limitation is not an error in a script that
		// doesn't use the value.
		n, _ := res.RowsAffected()
		id, _ := res.LastInsertId()
		return []vm.Value{int64(n), int64(id)}
	}})

	// placeholder(n) — the same answer as db.placeholder, without the caller
	// having to remember which driver this connection ended up on.
	methods.Set("placeholder", &vm.GoFunc{Name: "db:placeholder", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		n := vm.OptInt("db:placeholder", 2, a, 1)
		if n < 1 {
			panic(vm.Errorf("db:placeholder: index must be >= 1, got %d", n))
		}
		return []vm.Value{placeholder(driver, int(n))}
	}})

	methods.Set("query", &vm.GoFunc{Name: "db:query", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		query := vm.StringArg("db:query", 2, a)
		bindArgs := toDriverArgs(a[2:])

		rows, err := handle.Query(query, bindArgs...)
		if err != nil {
			panic(vm.Errorf("db:query: %s", err.Error()))
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			panic(vm.Errorf("db:query columns: %s", err.Error()))
		}
		// Column types are what let a MySQL result look like every other
		// driver's: MySQL's text protocol returns nearly everything as raw
		// bytes, so without the declared type a SELECT of an INT column
		// would reach Lua as a string. Best-effort — a driver that doesn't
		// implement ColumnTypes just gets the untyped mapping.
		colTypes := columnTypeNames(rows, len(cols))

		// Scan into a fresh `[]any` per row; *any is the
		// lowest-common-denominator scan target across drivers.
		out := vm.NewTable(0, 0)
		idx := int64(1)
		for rows.Next() {
			scan := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range scan {
				ptrs[i] = &scan[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				panic(vm.Errorf("db:query scan: %s", err.Error()))
			}

			row := vm.NewTable(0, len(cols))
			for i, name := range cols {
				row.Set(name, coerceColumn(colTypes[i], scan[i]))
			}
			out.Set(idx, row)
			idx++
		}
		if err := rows.Err(); err != nil {
			panic(vm.Errorf("db:query iter: %s", err.Error()))
		}
		return []vm.Value{out}
	}})

	methods.Set("ping", &vm.GoFunc{Name: "db:ping", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if err := handle.Ping(); err != nil {
			panic(vm.Errorf("db:ping: %s", err.Error()))
		}
		return nil
	}})

	methods.Set("close", &vm.GoFunc{Name: "db:close", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		// database/sql tolerates Close after Close, so no need to
		// guard with a "closed" flag here.
		if err := handle.Close(); err != nil {
			panic(vm.Errorf("db:close: %s", err.Error()))
		}
		return nil
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	conn.SetMetatable(mt)
	return conn
}

// toDriverArgs converts the bind-parameter slice from.lsc-side
// values into the `any` slice database/sql expects. Lua's value types
// (int64, float64, string, bool, nil) are all valid driver.Value
// types, so the conversion is mostly a pass-through.
func toDriverArgs(vals []vm.Value) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case nil, bool, int64, float64, string:
			out[i] = x
		default:
			// Fall back to string representation for tables /
			// functions / userdata; the driver will reject what
			// it can't handle and we'll surface the error.
			out[i] = vm.ToString(v)
		}
	}
	return out
}

// columnTypeNames reads the declared database type of each column, upper-cased
// for matching. Returns a slice of empty strings when the driver does not
// support ColumnTypes, so callers need no second code path.
func columnTypeNames(rows *sql.Rows, n int) []string {
	out := make([]string, n)
	types, err := rows.ColumnTypes()
	if err != nil {
		return out
	}
	for i := range out {
		if i < len(types) {
			out[i] = strings.ToUpper(types[i].DatabaseTypeName())
		}
	}
	return out
}

// coerceColumn maps one scanned value to a Lua value, consulting the column's
// declared type when the driver handed back raw bytes.
//
// Only []byte is reinterpreted, and only for types where the textual form has
// an exact numeric reading. That is the whole MySQL problem: lib/pq and the
// SQLite drivers already return int64/float64 for numeric columns, while
// go-sql-driver returns the wire bytes, so the same query would produce
// numbers on one database and strings on another.
//
// DECIMAL and NUMERIC are deliberately left as strings. They are exact
// types — Postgres NUMERIC also arrives as bytes — and float64 cannot hold
// them without silently losing digits, which is worse than a string a script
// can convert explicitly.
func coerceColumn(dbType string, v any) vm.Value {
	raw, isBytes := v.([]byte)
	if !isBytes || dbType == "" {
		return goToLua(v)
	}
	text := string(raw)
	switch dbType {
	case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "INTEGER", "BIGINT",
		"INT2", "INT4", "INT8", "SERIAL", "BIGSERIAL", "SMALLSERIAL",
		"YEAR", "UNSIGNED BIGINT", "UNSIGNED INT":
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n
		}
	case "FLOAT", "DOUBLE", "REAL", "FLOAT4", "FLOAT8", "DOUBLE PRECISION":
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return f
		}
	case "BOOL", "BOOLEAN":
		switch text {
		case "1", "t", "true", "TRUE":
			return true
		case "0", "f", "false", "FALSE":
			return false
		}
	}
	// Unrecognised type, or bytes that don't parse as the declared type:
	// fall through to the untyped mapping rather than guessing.
	return goToLua(v)
}

// goToLua maps the values that database/sql's Scan(*any) hands back
// into the runtime's Value subset. RawBytes / []byte come back as
// strings (the common case for text columns); time.Time becomes an
// RFC3339 string so.lsc code has a stable shape without a date
// type.
func goToLua(v any) vm.Value {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int64:
		return x
	case float64:
		return x
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		// Last-resort: stringify so the user sees *something*
		// instead of an opaque Go value leaking into Lua space.
		return vm.ToString(x)
	}
}
