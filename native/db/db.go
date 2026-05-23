// Package db is the host-side bridge between sakura code and Go's
// database/sql. The package itself is driver-agnostic — drivers are
// registered via blank imports in companion files gated by build tags.
// To add a new driver (e.g. sqlite, mysql) drop a `driver_<name>.go`
// next to this file with a `//go:build` line and a `_ "..."` import.
// To omit the default Postgres driver, build with `-tags sakura_no_postgres`.
package db

import (
	"database/sql"
	"time"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterDBPreload installs a single loader entry. The loader is a
// GoFunc that returns the module table — `require` runs it on first
// call, caches the result in `package.loaded`, and returns the cache
// on every subsequent `require("db")`.
func RegisterDBPreload(v *vm.VM) {
	vm.RegisterPreload(v, "db", dbLoader)
}

func dbLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("open", &vm.GoFunc{Name: "db.open", Fn: dbOpen})
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

// dbOpen is the constructor: `db.open(driver, dsn)` -> connection table.
//
// Wraps Go's database/sql; `lib/pq` is registered via the blank import
// above, so driver = "postgres" + a libpq DSN works out of the box.
// Other drivers (mysql, sqlite, …) just need their own blank import.
//
// On error we panic with a vm.LuaError so the sakura side can catch it
// with pcall(); successful return value is a connection table whose
// methods close over the underlying *sql.DB.
func dbOpen(_ *vm.VM, args []vm.Value) []vm.Value {
	driverName := vm.StringArg("db.open", 1, args)
	dataSource := vm.StringArg("db.open", 2, args)

	handle, err := sql.Open(driverName, dataSource)
	if err != nil {
		panic(vm.Errorf("db.open: %s", err.Error()))
	}
	// sql.Open is lazy — it doesn't actually contact the server. Ping
	// here so the caller learns about a bad DSN immediately rather than
	// on the first query.
	if err := handle.Ping(); err != nil {
		_ = handle.Close()
		panic(vm.Errorf("db.open: %s", err.Error()))
	}
	return []vm.Value{newConn(handle)}
}

// newConn builds a fresh connection table per call. Methods are GoFuncs
// that close over `handle`, so the *sql.DB never leaks into Lua memory
// and can only be reached through the methods we expose here.
func newConn(handle *sql.DB) *vm.Table {
	conn := vm.NewTable(0, 1)

	methods := vm.NewTable(0, 4)

	methods.Set("exec", &vm.GoFunc{Name: "db:exec", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		// a[0] = self (ignored — handle is captured), a[1] = SQL,
		// a[2:] = bind parameters.
		query := vm.StringArg("db:exec", 2, a)
		bindArgs := toDriverArgs(a[2:])

		res, err := handle.Exec(query, bindArgs...)
		if err != nil {
			panic(vm.Errorf("db:exec: %s", err.Error()))
		}
		// RowsAffected is best-effort across drivers; surface 0 on
		// "not supported" rather than blowing up.
		n, _ := res.RowsAffected()
		return []vm.Value{int64(n)}
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
				row.Set(name, goToLua(scan[i]))
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

// toDriverArgs converts the bind-parameter slice from sakura-side
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

// goToLua maps the values that database/sql's Scan(*any) hands back
// into the runtime's Value subset. RawBytes / []byte come back as
// strings (the common case for text columns); time.Time becomes an
// RFC3339 string so sakura code has a stable shape without a date
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
