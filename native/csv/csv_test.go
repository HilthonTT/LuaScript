package csv

import (
	"reflect"
	"testing"
)

func TestParseBasic(t *testing.T) {
	grid, err := parse("a,b,c\n1,2,3\n4,5,6\n", ',')
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"a", "b", "c"}, {"1", "2", "3"}, {"4", "5", "6"}}
	if !reflect.DeepEqual(grid, want) {
		t.Fatalf("got %v, want %v", grid, want)
	}
}

func TestParseQuotedAndDelimiter(t *testing.T) {
	grid, err := parse("name;note\nAda;\"hello, world\"\n", ';')
	if err != nil {
		t.Fatal(err)
	}
	if grid[1][1] != "hello, world" {
		t.Fatalf("quoted field not preserved: %q", grid[1][1])
	}
}

func TestStringifyRoundTrip(t *testing.T) {
	grid := [][]string{{"x", "y"}, {"1", "two, with comma"}}
	text, err := stringify(grid, ',')
	if err != nil {
		t.Fatal(err)
	}
	back, err := parse(text, ',')
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, grid) {
		t.Fatalf("round trip mismatch: %v vs %v", back, grid)
	}
}

func TestCellNumberParsing(t *testing.T) {
	if v := cell("42", true); v != int64(42) {
		t.Fatalf("expected int64(42), got %T(%v)", v, v)
	}
	if v := cell("3.14", true); v != 3.14 {
		t.Fatalf("expected 3.14, got %v", v)
	}
	if v := cell("hello", true); v != "hello" {
		t.Fatalf("expected string passthrough, got %v", v)
	}
	if v := cell("42", false); v != "42" {
		t.Fatalf("expected raw string when numbers off, got %v", v)
	}
}
