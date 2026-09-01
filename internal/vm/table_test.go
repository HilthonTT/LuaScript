package vm

import (
	"fmt"
	"testing"
)

func collectKeys(t *Table, deleteAsWeGo bool) []Value {
	var out []Value
	var k Value
	for {
		nk, _ := t.Next(k)
		if nk == nil {
			return out
		}
		out = append(out, nk)
		if deleteAsWeGo {
			t.Set(nk, nil)
		}
		k = nk
	}
}

func TestTableNextDeleteCurrentKeyDuringIteration(t *testing.T) {
	tbl := NewTable(0, 0)
	const n = 100
	for i := 0; i < n; i++ {
		tbl.Set(fmt.Sprintf("k%d", i), int64(i))
	}
	seen := collectKeys(tbl, true)
	if len(seen) != n {
		t.Fatalf("visited %d keys during delete-as-you-go iteration, want %d", len(seen), n)
	}
	if rest := collectKeys(tbl, false); len(rest) != 0 {
		t.Fatalf("table should be empty after deleting every key, %d left", len(rest))
	}
}

func TestTableReinsertAfterDeleteCompacts(t *testing.T) {
	tbl := NewTable(0, 0)
	const n = 64
	for i := 0; i < n; i++ {
		tbl.Set(fmt.Sprintf("k%d", i), int64(i))
	}
	for i := 0; i < n; i += 2 {
		tbl.Set(fmt.Sprintf("k%d", i), nil)
	}
	for i := 0; i < n; i++ {
		tbl.Set(fmt.Sprintf("new%d", i), int64(i))
	}
	want := n/2 + n
	seen := map[Value]bool{}
	for _, k := range collectKeys(tbl, false) {
		if seen[k] {
			t.Fatalf("key %v visited twice", k)
		}
		seen[k] = true
	}
	if len(seen) != want {
		t.Fatalf("visited %d distinct keys, want %d", len(seen), want)
	}
	if got := len(tbl.keys); got != want {
		t.Fatalf("keys slice not compacted: len %d, want %d (dead=%d)", got, want, tbl.dead)
	}
}

func TestTableDeleteThenReinsertSameKey(t *testing.T) {
	tbl := NewTable(0, 0)
	tbl.Set("a", int64(1))
	tbl.Set("b", int64(2))
	tbl.Set("a", nil)
	tbl.Set("a", int64(3))
	if got := tbl.Get("a"); got != int64(3) {
		t.Fatalf("t.a = %v, want 3", got)
	}
	count := 0
	for _, k := range collectKeys(tbl, false) {
		if k == "a" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("key 'a' visited %d times, want 1", count)
	}
}

func TestTableNextSkipsLeadingTombstone(t *testing.T) {
	tbl := NewTable(0, 0)
	tbl.Set("first", int64(1))
	tbl.Set("second", int64(2))
	tbl.Set("first", nil)
	k, v := tbl.Next(nil)
	if k != "second" || v != int64(2) {
		t.Fatalf("Next(nil) = %v, %v; want second, 2", k, v)
	}
}
