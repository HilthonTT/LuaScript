// Package csv is a require()-able host module for reading and writing CSV,
// the lingua franca of tabular data. It wraps Go's encoding/csv so quoting,
// embedded newlines, and custom delimiters all behave correctly.
//
// API:
//
//	csv.parse(text [, opts])      -> rows
//	csv.stringify(data [, opts])  -> text
//	csv.read(path [, opts])       -> rows
//	csv.write(path, data [, opts])
//
// opts is an optional table:
//
//	header    = false  -- when true, the first row names the columns and each
//	                      data row is returned as a { column = value } map
//	delimiter = ","    -- single-character field separator
//	numbers   = false  -- when true, numeric-looking cells parse to numbers
//
// Without header, rows are returned as an array of string arrays. The
// row/grid <-> string conversions live in pure helpers tested in csv_test.go.
package csv

import (
	encodingcsv "encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/vm"
)

// RegisterCSVPreload installs the loader under package.preload.
func RegisterCSVPreload(v *vm.VM) {
	vm.RegisterPreload(v, "csv", csvLoader)
}

func csvLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 4)
	methods := vm.NewTable(0, 4)

	methods.Set("parse", &vm.GoFunc{Name: "csv:parse", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		text := vm.StringArg("csv.parse", 1, args)
		o := readOpts(args, 2)
		grid, err := parse(text, o.delim)
		if err != nil {
			panic(vm.Errorf("csv.parse: %s", err.Error()))
		}
		return []vm.Value{gridToTable(grid, o)}
	}})

	methods.Set("stringify", &vm.GoFunc{Name: "csv:stringify", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.TableArg("csv.stringify", 1, args)
		o := readOpts(args, 2)
		grid := tableToGrid("csv.stringify", data, o)
		text, err := stringify(grid, o.delim)
		if err != nil {
			panic(vm.Errorf("csv.stringify: %s", err.Error()))
		}
		return []vm.Value{text}
	}})

	methods.Set("read", &vm.GoFunc{Name: "csv:read", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("csv.read", 1, args)
		o := readOpts(args, 2)
		b, err := os.ReadFile(path)
		if err != nil {
			panic(vm.Errorf("csv.read: %s", err.Error()))
		}
		grid, err := parse(string(b), o.delim)
		if err != nil {
			panic(vm.Errorf("csv.read: %s", err.Error()))
		}
		return []vm.Value{gridToTable(grid, o)}
	}})

	methods.Set("write", &vm.GoFunc{Name: "csv:write", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("csv.write", 1, args)
		data := vm.TableArg("csv.write", 2, args)
		o := readOpts(args, 3)
		grid := tableToGrid("csv.write", data, o)
		text, err := stringify(grid, o.delim)
		if err != nil {
			panic(vm.Errorf("csv.write: %s", err.Error()))
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			panic(vm.Errorf("csv.write: %s", err.Error()))
		}
		return nil
	}})

	m.Set("VERSION", "0.1.0")
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return []vm.Value{m}
}

// ---------------------------------------------------------------------------
// Pure string <-> grid conversion
// ---------------------------------------------------------------------------

func parse(text string, delim rune) ([][]string, error) {
	r := encodingcsv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1 // tolerate ragged rows; the caller decides what to do
	return r.ReadAll()
}

func stringify(grid [][]string, delim rune) (string, error) {
	var sb strings.Builder
	w := encodingcsv.NewWriter(&sb)
	w.Comma = delim
	if err := w.WriteAll(grid); err != nil {
		return "", err
	}
	w.Flush()
	return sb.String(), w.Error()
}

// ---------------------------------------------------------------------------
// Lua marshalling
// ---------------------------------------------------------------------------

type options struct {
	header     bool
	numbers    bool
	delim      rune
	columnsRaw []string // ordered column names for header-mode stringify/write
}

func readOpts(args []vm.Value, n int) options {
	o := options{delim: ','}
	if n < 1 || n > len(args) || args[n-1] == nil {
		return o
	}
	t, ok := args[n-1].(*vm.Table)
	if !ok {
		return o
	}
	o.header = vm.IsTruthy(t.Get("header"))
	o.numbers = vm.IsTruthy(t.Get("numbers"))
	if s, ok := t.Get("delimiter").(string); ok {
		d := []rune(s)
		if len(d) != 1 {
			panic(vm.Errorf("csv: delimiter must be a single character, got %q", s))
		}
		o.delim = d[0]
	}
	if cols, ok := t.Get("columns").(*vm.Table); ok {
		nc := int(cols.Len())
		o.columnsRaw = make([]string, nc)
		for i := 1; i <= nc; i++ {
			s, ok := cols.Get(int64(i)).(string)
			if !ok {
				panic(vm.Errorf("csv: opts.columns[%d] must be a string", i))
			}
			o.columnsRaw[i-1] = s
		}
	}
	return o
}

// gridToTable converts the parsed grid into a Lua value: an array of
// {col=value} maps when header is set, otherwise an array of string arrays.
func gridToTable(grid [][]string, o options) *vm.Table {
	if !o.header {
		out := vm.NewTable(len(grid), 0)
		for i, row := range grid {
			out.Set(int64(i+1), rowToArray(row, o.numbers))
		}
		return out
	}
	if len(grid) == 0 {
		return vm.NewTable(0, 0)
	}
	cols := grid[0]
	out := vm.NewTable(len(grid)-1, 0)
	for i, row := range grid[1:] {
		rec := vm.NewTable(0, len(cols))
		for j, name := range cols {
			if j < len(row) {
				rec.Set(name, cell(row[j], o.numbers))
			} else {
				rec.Set(name, "") // short row: pad missing trailing fields
			}
		}
		out.Set(int64(i+1), rec)
	}
	return out
}

func rowToArray(row []string, numbers bool) *vm.Table {
	t := vm.NewTable(len(row), 0)
	for j, v := range row {
		t.Set(int64(j+1), cell(v, numbers))
	}
	return t
}

// cell returns the field as a number when numbers is set and it parses
// cleanly, otherwise as the raw string. Integers stay integers.
func cell(s string, numbers bool) vm.Value {
	if !numbers {
		return s
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// tableToGrid converts a Lua value back into a string grid. With header set,
// data is an array of {col=value} maps and the column order is taken from
// opts.columns (required, so the output order is deterministic). Otherwise
// data is an array of arrays.
func tableToGrid(site string, data *vm.Table, o options) [][]string {
	n := int(data.Len())
	if o.header {
		cols := columnsOpt(site, o)
		grid := make([][]string, 0, n+1)
		grid = append(grid, cols)
		for i := 1; i <= n; i++ {
			rec, ok := data.Get(int64(i)).(*vm.Table)
			if !ok {
				panic(vm.Errorf("%s: row %d must be a table when header is set", site, i))
			}
			row := make([]string, len(cols))
			for j, name := range cols {
				row[j] = vm.ToString(rec.Get(name))
			}
			grid = append(grid, row)
		}
		return grid
	}

	grid := make([][]string, 0, n)
	for i := 1; i <= n; i++ {
		row, ok := data.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: row %d must be an array", site, i))
		}
		m := int(row.Len())
		cells := make([]string, m)
		for j := 1; j <= m; j++ {
			cells[j-1] = vm.ToString(row.Get(int64(j)))
		}
		grid = append(grid, cells)
	}
	return grid
}

// columnsOpt is needed only for header-mode stringify/write: it is passed via
// a separate field because maps have no inherent order.
func columnsOpt(site string, o options) []string {
	if o.columnsRaw == nil {
		panic(vm.Errorf("%s: opts.columns is required when header is set (an ordered array of column names)", site))
	}
	return o.columnsRaw
}
