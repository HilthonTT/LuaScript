// Package dataframe is a require()-able host module providing a column-
// oriented, pandas-style tabular structure — the central abstraction for
// most data-science work in LuaScript. A DataFrame holds named columns of
// equal length; the methods below cover the everyday verbs: selecting and
// dropping columns, filtering and sorting rows, deriving new columns,
// grouping with aggregation, summary statistics, and CSV interchange.
//
// Construction:
//
//	df = dataframe.new({ age = {30, 25}, name = {"a", "b"} })  -- column map
//	df = dataframe.from_rows({ {age=30, name="a"}, ... })       -- row records
//	df = dataframe.from_csv("people.csv")                       -- header + auto numbers
//
// Every transforming method returns a NEW DataFrame; the receiver is never
// mutated, so chains like df:filter(p):select({"x"}):sort("x") are safe.
package dataframe

import (
	encodingcsv "encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/vm"
)

// RegisterDataFramePreload installs the loader under package.preload.
func RegisterDataFramePreload(v *vm.VM) {
	vm.RegisterPreload(v, "dataframe", dfLoader)
}

func dfLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 4)
	methods := vm.NewTable(0, 4)

	methods.Set("new", &vm.GoFunc{Name: "dataframe.new", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromColumns(vm.TableArg("dataframe.new", 1, args)))}
	}})
	methods.Set("from_rows", &vm.GoFunc{Name: "dataframe.from_rows", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromRows(vm.TableArg("dataframe.from_rows", 1, args)))}
	}})
	methods.Set("from_csv", &vm.GoFunc{Name: "dataframe.from_csv", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("dataframe.from_csv", 1, args)
		numbers := true
		if t, ok := arg(args, 2).(*vm.Table); ok {
			if t.Get("numbers") != nil {
				numbers = vm.IsTruthy(t.Get("numbers"))
			}
		}
		return []vm.Value{wrap(fromCSV(path, numbers))}
	}})

	m.Set("VERSION", "0.1.0")
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return []vm.Value{m}
}

// DataFrame is a set of equal-length named columns held in column order.
// Cell values are raw Lua values (int64 / float64 / string / bool / nil).
type DataFrame struct {
	order []string // column names, in display order
	cols  map[string][]vm.Value
	n     int // row count (length of every column)
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

// fromColumns builds a frame from a Lua map of { name = array }. All columns
// must share a length.
func fromColumns(t *vm.Table) *DataFrame {
	d := &DataFrame{cols: map[string][]vm.Value{}, n: -1}
	for k, v := t.Next(nil); k != nil; k, v = t.Next(k) {
		name, ok := k.(string)
		if !ok {
			panic(vm.Errorf("dataframe.new: column names must be strings, got %s", vm.TypeName(k)))
		}
		arr, ok := v.(*vm.Table)
		if !ok {
			panic(vm.Errorf("dataframe.new: column %q must be an array", name))
		}
		col := tableToColumn(arr)
		if d.n == -1 {
			d.n = len(col)
		} else if len(col) != d.n {
			panic(vm.Errorf("dataframe.new: column %q has %d rows, expected %d", name, len(col), d.n))
		}
		d.order = append(d.order, name)
		d.cols[name] = col
	}
	if d.n == -1 {
		d.n = 0
	}
	return d
}

// fromRows builds a frame from a Lua array of { name = value } records. The
// column order and set are taken from the first row; later rows are read by
// that key set, with missing keys becoming nil.
func fromRows(t *vm.Table) *DataFrame {
	nrows := int(t.Len())
	d := &DataFrame{cols: map[string][]vm.Value{}, n: nrows}
	if nrows == 0 {
		return d
	}
	first, ok := t.Get(int64(1)).(*vm.Table)
	if !ok {
		panic(vm.Errorf("dataframe.from_rows: row 1 must be a table"))
	}
	for k, _ := first.Next(nil); k != nil; k, _ = first.Next(k) {
		name, ok := k.(string)
		if !ok {
			continue // skip non-string keys; this is a record, not an array
		}
		d.order = append(d.order, name)
		d.cols[name] = make([]vm.Value, nrows)
	}
	for i := 1; i <= nrows; i++ {
		row, ok := t.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("dataframe.from_rows: row %d must be a table", i))
		}
		for _, name := range d.order {
			d.cols[name][i-1] = row.Get(name)
		}
	}
	return d
}

// fromCSV reads a header-rowed CSV file into a frame. With numbers set,
// numeric-looking cells become int64/float64.
func fromCSV(path string, numbers bool) *DataFrame {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(vm.Errorf("dataframe.from_csv: %s", err.Error()))
	}
	r := encodingcsv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	grid, err := r.ReadAll()
	if err != nil {
		panic(vm.Errorf("dataframe.from_csv: %s", err.Error()))
	}
	if len(grid) == 0 {
		return &DataFrame{cols: map[string][]vm.Value{}}
	}
	header := grid[0]
	body := grid[1:]
	d := &DataFrame{cols: map[string][]vm.Value{}, n: len(body), order: append([]string{}, header...)}
	for _, name := range header {
		d.cols[name] = make([]vm.Value, len(body))
	}
	for i, row := range body {
		for j, name := range header {
			var v vm.Value = ""
			if j < len(row) {
				v = parseCell(row[j], numbers)
			}
			d.cols[name][i] = v
		}
	}
	return d
}

func parseCell(s string, numbers bool) vm.Value {
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

func tableToColumn(t *vm.Table) []vm.Value {
	n := int(t.Len())
	col := make([]vm.Value, n)
	for i := 1; i <= n; i++ {
		col[i-1] = t.Get(int64(i))
	}
	return col
}

// ---------------------------------------------------------------------------
// Pure transforms (return new frames; never mutate the receiver)
// ---------------------------------------------------------------------------

func (d *DataFrame) has(name string) bool { _, ok := d.cols[name]; return ok }

func (d *DataFrame) mustHave(site, name string) {
	if !d.has(name) {
		panic(vm.Errorf("%s: no such column %q", site, name))
	}
}

// pick builds a frame with the named columns (which must exist) over the
// given row indices, in the given column order.
func (d *DataFrame) pick(names []string, rows []int) *DataFrame {
	out := &DataFrame{cols: map[string][]vm.Value{}, order: names, n: len(rows)}
	for _, name := range names {
		src := d.cols[name]
		col := make([]vm.Value, len(rows))
		for i, r := range rows {
			col[i] = src[r]
		}
		out.cols[name] = col
	}
	return out
}

func allRows(n int) []int {
	rows := make([]int, n)
	for i := range rows {
		rows[i] = i
	}
	return rows
}

func (d *DataFrame) selectCols(site string, names []string) *DataFrame {
	for _, name := range names {
		d.mustHave(site, name)
	}
	return d.pick(names, allRows(d.n))
}

func (d *DataFrame) dropCols(names []string) *DataFrame {
	drop := map[string]bool{}
	for _, name := range names {
		drop[name] = true
	}
	keep := make([]string, 0, len(d.order))
	for _, name := range d.order {
		if !drop[name] {
			keep = append(keep, name)
		}
	}
	return d.pick(keep, allRows(d.n))
}

func (d *DataFrame) headRows(n int) *DataFrame {
	if n > d.n {
		n = d.n
	}
	if n < 0 {
		n = 0
	}
	return d.pick(d.order, allRows(n))
}

func (d *DataFrame) tailRows(n int) *DataFrame {
	if n > d.n {
		n = d.n
	}
	if n < 0 {
		n = 0
	}
	rows := make([]int, n)
	for i := range rows {
		rows[i] = d.n - n + i
	}
	return d.pick(d.order, rows)
}

// rowRecord returns row i (0-based) as a fresh { name = value } Lua table.
func (d *DataFrame) rowRecord(i int) *vm.Table {
	rec := vm.NewTable(0, len(d.order))
	for _, name := range d.order {
		rec.Set(name, d.cols[name][i])
	}
	return rec
}

// floatColumn returns the column as []float64. ok is false when any cell is
// non-numeric, so callers can decide whether to skip the column.
func (d *DataFrame) floatColumn(name string) ([]float64, bool) {
	src := d.cols[name]
	out := make([]float64, len(src))
	for i, v := range src {
		f, ok := numeric(v)
		if !ok {
			return nil, false
		}
		out[i] = f
	}
	return out, true
}

// ---------------------------------------------------------------------------
// Lua method binding
// ---------------------------------------------------------------------------

// wrap exposes a *DataFrame as a Lua object whose methods are colon-called
// (self is args[0]; the actual arguments start at index 1).
func wrap(d *DataFrame) *vm.Table {
	methods := vm.NewTable(0, 24)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "df:" + name, Fn: fn})
	}

	set("columns", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{stringsToTable(d.order)}
	})
	set("nrows", func(_ *vm.VM, _ []vm.Value) []vm.Value { return []vm.Value{int64(d.n)} })
	set("ncols", func(_ *vm.VM, _ []vm.Value) []vm.Value { return []vm.Value{int64(len(d.order))} })
	set("shape", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(d.n), int64(len(d.order))}
	})

	set("head", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.headRows(optInt(args, 1, 5)))}
	})
	set("tail", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.tailRows(optInt(args, 1, 5)))}
	})

	set("col", func(_ *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("df:col", 2, args)
		d.mustHave("df:col", name)
		return []vm.Value{valuesToTable(d.cols[name])}
	})
	set("row", func(_ *vm.VM, args []vm.Value) []vm.Value {
		i := int(vm.IntArg("df:row", 2, args))
		if i < 1 || i > d.n {
			panic(vm.Errorf("df:row: index %d out of range [1, %d]", i, d.n))
		}
		return []vm.Value{d.rowRecord(i - 1)}
	})
	set("rows", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		out := vm.NewTable(d.n, 0)
		for i := 0; i < d.n; i++ {
			out.Set(int64(i+1), d.rowRecord(i))
		}
		return []vm.Value{out}
	})

	set("select", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.selectCols("df:select", namesArg("df:select", args)))}
	})
	set("drop", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.dropCols(namesArg("df:drop", args)))}
	})
	set("rename", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.rename(vm.TableArg("df:rename", 2, args)))}
	})

	set("filter", func(v *vm.VM, args []vm.Value) []vm.Value {
		fn := funcArg("df:filter", 2, args)
		rows := make([]int, 0, d.n)
		for i := 0; i < d.n; i++ {
			r := v.CallValue(fn, []vm.Value{d.rowRecord(i)}, 1)
			if len(r) > 0 && vm.IsTruthy(r[0]) {
				rows = append(rows, i)
			}
		}
		return []vm.Value{wrap(d.pick(d.order, rows))}
	})

	set("with_column", func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("df:with_column", 2, args)
		fn := funcArg("df:with_column", 3, args)
		col := make([]vm.Value, d.n)
		for i := 0; i < d.n; i++ {
			r := v.CallValue(fn, []vm.Value{d.rowRecord(i)}, 1)
			if len(r) > 0 {
				col[i] = r[0]
			}
		}
		return []vm.Value{wrap(d.withColumn(name, col))}
	})

	set("sort", func(_ *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("df:sort", 2, args)
		d.mustHave("df:sort", name)
		desc := len(args) >= 3 && vm.IsTruthy(args[2])
		return []vm.Value{wrap(d.sortBy(name, desc))}
	})

	set("group_by", func(_ *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("df:group_by", 2, args)
		d.mustHave("df:group_by", name)
		aggs := vm.TableArg("df:group_by", 3, args)
		return []vm.Value{wrap(d.groupBy(name, aggs))}
	})

	set("describe", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrap(d.describe())}
	})

	set("to_table", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		out := vm.NewTable(0, len(d.order))
		for _, name := range d.order {
			out.Set(name, valuesToTable(d.cols[name]))
		}
		return []vm.Value{out}
	})

	set("to_csv", func(_ *vm.VM, args []vm.Value) []vm.Value {
		text := d.toCSV()
		if path, ok := arg(args, 2).(string); ok {
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				panic(vm.Errorf("df:to_csv: %s", err.Error()))
			}
			return nil
		}
		return []vm.Value{text}
	})

	set("show", func(_ *vm.VM, args []vm.Value) []vm.Value {
		fmt.Println(d.render(optInt(args, 1, 20)))
		return nil
	})

	t := vm.NewTable(0, 0)
	mt := vm.NewTable(0, 2)
	mt.Set("__index", methods)
	mt.Set("__tostring", &vm.GoFunc{Name: "df:__tostring", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{d.render(20)}
	}})
	t.SetMetatable(mt)
	return t
}

func (d *DataFrame) rename(m *vm.Table) *DataFrame {
	out := &DataFrame{cols: map[string][]vm.Value{}, n: d.n}
	for _, old := range d.order {
		name := old
		if nv, ok := m.Get(old).(string); ok {
			name = nv
		}
		out.order = append(out.order, name)
		out.cols[name] = d.cols[old]
	}
	return out
}

// withColumn returns a copy with name set to col, replacing it if it already
// exists or appending it otherwise.
func (d *DataFrame) withColumn(name string, col []vm.Value) *DataFrame {
	out := &DataFrame{cols: map[string][]vm.Value{}, n: d.n}
	out.order = append(out.order, d.order...)
	for _, k := range d.order {
		out.cols[k] = d.cols[k]
	}
	if !out.has(name) {
		out.order = append(out.order, name)
	}
	out.cols[name] = col
	return out
}

func (d *DataFrame) sortBy(name string, desc bool) *DataFrame {
	col := d.cols[name]
	rows := allRows(d.n)
	sort.SliceStable(rows, func(a, b int) bool {
		c := compareValues(col[rows[a]], col[rows[b]])
		if desc {
			return c > 0
		}
		return c < 0
	})
	return d.pick(d.order, rows)
}

// groupBy splits rows by the distinct values of groupCol (first-seen order)
// and reduces each group per the aggs map { sourceCol = aggName }. Output
// columns are the group column followed by one "<col>_<agg>" column per entry.
func (d *DataFrame) groupBy(groupCol string, aggs *vm.Table) *DataFrame {
	// Collect (outName, sourceCol, agg) in the agg table's insertion order.
	type spec struct{ out, src, agg string }
	var specs []spec
	for k, v := aggs.Next(nil); k != nil; k, v = aggs.Next(k) {
		src, ok := k.(string)
		if !ok {
			continue
		}
		agg, ok := v.(string)
		if !ok {
			panic(vm.Errorf("df:group_by: aggregation for %q must be a string (e.g. \"mean\")", src))
		}
		d.mustHave("df:group_by", src)
		specs = append(specs, spec{out: src + "_" + agg, src: src, agg: agg})
	}

	keyCol := d.cols[groupCol]
	var keys []vm.Value
	groups := map[vm.Value][]int{}
	for i := 0; i < d.n; i++ {
		k := keyCol[i]
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], i)
	}

	out := &DataFrame{cols: map[string][]vm.Value{}, n: len(keys)}
	out.order = append(out.order, groupCol)
	out.cols[groupCol] = append([]vm.Value{}, keys...)
	for _, s := range specs {
		out.order = append(out.order, s.out)
		out.cols[s.out] = make([]vm.Value, len(keys))
	}
	for gi, key := range keys {
		idx := groups[key]
		for _, s := range specs {
			out.cols[s.out][gi] = reduce(d.cols[s.src], idx, s.agg)
		}
	}
	return out
}

// describe builds a summary frame: one "statistic" column naming each measure
// and one column per numeric source column holding the values. Non-numeric
// columns are skipped.
func (d *DataFrame) describe() *DataFrame {
	statNames := []string{"count", "mean", "std", "min", "25%", "50%", "75%", "max"}
	out := &DataFrame{cols: map[string][]vm.Value{}, n: len(statNames)}
	out.order = append(out.order, "statistic")
	out.cols["statistic"] = make([]vm.Value, len(statNames))
	for i, s := range statNames {
		out.cols["statistic"][i] = s
	}
	for _, name := range d.order {
		xs, ok := d.floatColumn(name)
		if !ok || len(xs) == 0 {
			continue
		}
		s := sortedCopy(xs)
		out.order = append(out.order, name)
		out.cols[name] = []vm.Value{
			int64(len(xs)),
			meanf(xs),
			stddevf(xs),
			s[0],
			quantile(s, 0.25),
			quantile(s, 0.5),
			quantile(s, 0.75),
			s[len(s)-1],
		}
	}
	return out
}

func (d *DataFrame) toCSV() string {
	var sb strings.Builder
	w := encodingcsv.NewWriter(&sb)
	w.Write(d.order)
	row := make([]string, len(d.order))
	for i := 0; i < d.n; i++ {
		for j, name := range d.order {
			row[j] = vm.ToString(d.cols[name][i])
		}
		w.Write(row)
	}
	w.Flush()
	return sb.String()
}

// render produces an aligned, padded text table capped at maxRows data rows.
func (d *DataFrame) render(maxRows int) string {
	if len(d.order) == 0 {
		return "DataFrame(empty)"
	}
	shown := d.n
	if shown > maxRows {
		shown = maxRows
	}
	// Build the string grid (header + shown rows) and per-column widths.
	widths := make([]int, len(d.order))
	cells := make([][]string, shown+1)
	cells[0] = append([]string{}, d.order...)
	for j, h := range cells[0] {
		widths[j] = len(h)
	}
	for i := 0; i < shown; i++ {
		row := make([]string, len(d.order))
		for j, name := range d.order {
			s := vm.ToString(d.cols[name][i])
			row[j] = s
			if len(s) > widths[j] {
				widths[j] = len(s)
			}
		}
		cells[i+1] = row
	}

	var sb strings.Builder
	writeRow := func(r []string) {
		for j, s := range r {
			if j > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(s)
			sb.WriteString(strings.Repeat(" ", widths[j]-len(s)))
		}
		sb.WriteByte('\n')
	}
	writeRow(cells[0])
	var rule []string
	for _, w := range widths {
		rule = append(rule, strings.Repeat("-", w))
	}
	writeRow(rule)
	for i := 1; i <= shown; i++ {
		writeRow(cells[i])
	}
	if d.n > shown {
		fmt.Fprintf(&sb, "... (%d rows x %d cols)\n", d.n, len(d.order))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ---------------------------------------------------------------------------
// Aggregation / numeric helpers
// ---------------------------------------------------------------------------

// reduce applies a named aggregation to the rows of col selected by idx.
func reduce(col []vm.Value, idx []int, agg string) vm.Value {
	switch agg {
	case "count":
		return int64(len(idx))
	case "first":
		if len(idx) == 0 {
			return nil
		}
		return col[idx[0]]
	case "last":
		if len(idx) == 0 {
			return nil
		}
		return col[idx[len(idx)-1]]
	}
	xs := make([]float64, 0, len(idx))
	for _, i := range idx {
		if f, ok := numeric(col[i]); ok {
			xs = append(xs, f)
		}
	}
	if len(xs) == 0 {
		return nil
	}
	switch agg {
	case "sum":
		return sumf(xs)
	case "mean", "avg":
		return meanf(xs)
	case "min":
		return minf(xs)
	case "max":
		return maxf(xs)
	case "median":
		return quantile(sortedCopy(xs), 0.5)
	case "std":
		return stddevf(xs)
	case "var":
		return variancef(xs)
	default:
		panic(vm.Errorf("df:group_by: unknown aggregation %q (use sum, mean, min, max, median, std, var, count, first, last)", agg))
	}
}

func numeric(v vm.Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}

func sumf(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
}

func meanf(xs []float64) float64 { return sumf(xs) / float64(len(xs)) }

func variancef(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := meanf(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return ss / float64(len(xs)-1)
}

func stddevf(xs []float64) float64 { return math.Sqrt(variancef(xs)) }

func minf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func sortedCopy(xs []float64) []float64 {
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	return s
}

func quantile(s []float64, q float64) float64 {
	n := len(s)
	if n == 1 {
		return s[0]
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	frac := pos - float64(lo)
	return s[lo]*(1-frac) + s[hi]*frac
}

// compareValues orders two cell values: numbers numerically, strings
// lexically, with numbers sorting before strings and nil sorting last.
func compareValues(a, b vm.Value) int {
	af, aNum := numeric(a)
	bf, bNum := numeric(b)
	switch {
	case aNum && bNum:
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	case aNum: // number before non-number
		return -1
	case bNum:
		return 1
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	default:
		return strings.Compare(vm.ToString(a), vm.ToString(b))
	}
}

// ---------------------------------------------------------------------------
// Lua marshalling helpers
// ---------------------------------------------------------------------------

func arg(args []vm.Value, i int) vm.Value {
	if i < 1 || i > len(args) {
		return nil
	}
	return args[i-1]
}

func optInt(args []vm.Value, methodArg int, dflt int) int {
	// methodArg is 1-based among the *method* args (so position 2 overall,
	// after self at index 1). Callers pass 1 for "the first real argument".
	v := arg(args, methodArg+1)
	if i, ok := vm.ToInteger(v); ok {
		return int(i)
	}
	return dflt
}

func funcArg(site string, n int, args []vm.Value) vm.Value {
	v := arg(args, n)
	switch v.(type) {
	case *vm.Closure, *vm.GoFunc:
		return v
	default:
		panic(vm.Errorf("%s: argument #%d must be a function, got %s", site, n-1, vm.TypeName(v)))
	}
}

// namesArg reads the column-name list a method like select/drop expects as its
// first real argument (an array of strings).
func namesArg(site string, args []vm.Value) []string {
	t := vm.TableArg(site, 2, args)
	n := int(t.Len())
	out := make([]string, n)
	for i := 1; i <= n; i++ {
		s, ok := t.Get(int64(i)).(string)
		if !ok {
			panic(vm.Errorf("%s: column list entry %d must be a string", site, i))
		}
		out[i-1] = s
	}
	return out
}

func stringsToTable(xs []string) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	for i, s := range xs {
		t.Set(int64(i+1), s)
	}
	return t
}

func valuesToTable(xs []vm.Value) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	for i, v := range xs {
		t.Set(int64(i+1), v)
	}
	return t
}
