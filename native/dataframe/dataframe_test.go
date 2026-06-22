package dataframe

import (
	"math"
	"testing"

	"github.com/hilthontt/luascript/vm"
)

// frame builds a DataFrame directly from Go data for testing the pure
// transforms without going through the Lua marshalling layer.
func frame(order []string, cols map[string][]vm.Value) *DataFrame {
	n := 0
	if len(order) > 0 {
		n = len(cols[order[0]])
	}
	return &DataFrame{order: order, cols: cols, n: n}
}

func TestSelectAndDrop(t *testing.T) {
	d := frame([]string{"a", "b", "c"}, map[string][]vm.Value{
		"a": {int64(1), int64(2)},
		"b": {int64(3), int64(4)},
		"c": {int64(5), int64(6)},
	})
	sel := d.selectCols("test", []string{"c", "a"})
	if len(sel.order) != 2 || sel.order[0] != "c" || sel.order[1] != "a" {
		t.Fatalf("select order wrong: %v", sel.order)
	}
	dr := d.dropCols([]string{"b"})
	if len(dr.order) != 2 || dr.has("b") {
		t.Fatalf("drop failed: %v", dr.order)
	}
}

func TestSortBy(t *testing.T) {
	d := frame([]string{"x"}, map[string][]vm.Value{
		"x": {int64(3), int64(1), int64(2)},
	})
	asc := d.sortBy("x", false)
	got := []int64{asc.cols["x"][0].(int64), asc.cols["x"][1].(int64), asc.cols["x"][2].(int64)}
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("ascending sort wrong: %v", got)
	}
	desc := d.sortBy("x", true)
	if desc.cols["x"][0].(int64) != 3 {
		t.Fatalf("descending sort wrong: %v", desc.cols["x"])
	}
}

func TestHeadTail(t *testing.T) {
	d := frame([]string{"x"}, map[string][]vm.Value{
		"x": {int64(1), int64(2), int64(3), int64(4), int64(5)},
	})
	if h := d.headRows(2); h.n != 2 || h.cols["x"][0].(int64) != 1 {
		t.Fatalf("head wrong: %v", h.cols["x"])
	}
	if tl := d.tailRows(2); tl.n != 2 || tl.cols["x"][0].(int64) != 4 {
		t.Fatalf("tail wrong: %v", tl.cols["x"])
	}
}

func TestGroupBy(t *testing.T) {
	d := frame([]string{"dept", "salary"}, map[string][]vm.Value{
		"dept":   {"eng", "eng", "sales", "sales"},
		"salary": {int64(100), int64(200), int64(50), int64(70)},
	})
	aggs := vm.NewTable(0, 2)
	aggs.Set("salary", "mean")
	g := d.groupBy("dept", aggs)
	if g.n != 2 {
		t.Fatalf("expected 2 groups, got %d", g.n)
	}
	// First-seen order: eng then sales.
	if g.cols["dept"][0] != "eng" || g.cols["dept"][1] != "sales" {
		t.Fatalf("group order wrong: %v", g.cols["dept"])
	}
	mean0 := g.cols["salary_mean"][0].(float64)
	mean1 := g.cols["salary_mean"][1].(float64)
	if math.Abs(mean0-150) > 1e-9 || math.Abs(mean1-60) > 1e-9 {
		t.Fatalf("group means wrong: %v %v", mean0, mean1)
	}
}

func TestDescribe(t *testing.T) {
	d := frame([]string{"x", "name"}, map[string][]vm.Value{
		"x":    {float64(1), float64(2), float64(3), float64(4)},
		"name": {"a", "b", "c", "d"},
	})
	desc := d.describe()
	// "name" is non-numeric and must be skipped; only statistic + x remain.
	if len(desc.order) != 2 || desc.order[1] != "x" {
		t.Fatalf("describe columns wrong: %v", desc.order)
	}
	// statistic[1] is "mean"; mean of 1..4 is 2.5.
	if desc.cols["x"][1].(float64) != 2.5 {
		t.Fatalf("describe mean wrong: %v", desc.cols["x"][1])
	}
}

func TestCompareValues(t *testing.T) {
	if compareValues(int64(1), int64(2)) != -1 {
		t.Fatal("1 < 2 failed")
	}
	if compareValues("b", "a") != 1 {
		t.Fatal("b > a failed")
	}
	if compareValues(int64(1), "a") != -1 {
		t.Fatal("number should sort before string")
	}
}
