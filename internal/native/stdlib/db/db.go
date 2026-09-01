package db

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

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

func dbDrivers(_ *vm.VM, _ []vm.Value) []vm.Value {
	out := vm.NewTable(len(sql.Drivers()), 0)
	for _, name := range availableDrivers() {
		out.Append(name)
	}
	return []vm.Value{out}
}

func dbPlaceholder(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg("db.placeholder", 1, args)
	n := vm.OptInt("db.placeholder", 2, args, 1)
	if n < 1 {
		panic(vm.Errorf("db.placeholder: index must be >= 1, got %d", n))
	}
	resolved, err := resolveDriver(name)
	if err != nil {
		resolved = strings.ToLower(name)
	}
	return []vm.Value{placeholder(resolved, int(n))}
}

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
	if isSQLite(resolved) && isMemoryDSN(dataSource) {
		handle.SetMaxOpenConns(1)
	}
	if err := handle.Ping(); err != nil {
		_ = handle.Close()
		panic(vm.Errorf("db.open: %s", err.Error()))
	}
	return []vm.Value{newConn(handle, resolved)}
}

func newConn(handle *sql.DB, driver string) *vm.Table {
	conn := vm.NewTable(0, 2)
	conn.Set("driver", driver)

	methods := vm.NewTable(0, 6)

	methods.Set("exec", &vm.GoFunc{Name: "db:exec", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		query := vm.StringArg("db:exec", 2, a)
		bindArgs := toDriverArgs(a[2:])

		res, err := handle.Exec(query, bindArgs...)
		if err != nil {
			panic(vm.Errorf("db:exec: %s", err.Error()))
		}
		n, _ := res.RowsAffected()
		id, _ := res.LastInsertId()
		return []vm.Value{int64(n), int64(id)}
	}})

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
		colTypes := columnTypeNames(rows, len(cols))

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

func toDriverArgs(vals []vm.Value) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case nil, bool, int64, float64, string:
			out[i] = x
		default:
			out[i] = vm.ToString(v)
		}
	}
	return out
}

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
	return goToLua(v)
}

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
		return vm.ToString(x)
	}
}
