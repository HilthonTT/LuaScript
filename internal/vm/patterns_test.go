package vm

import (
	"testing"
)

func nullCall(_ Value, _ []Value) []Value { return nil }

func TestPatternFindBasic(t *testing.T) {
	cases := []struct {
		s, pat     string
		init       int
		ok         bool
		start, end int
	}{
		{"hello", "ell", 1, true, 2, 4},
		{"hello", "xyz", 1, false, 0, 0},
		{"hello", "^h", 1, true, 1, 1},
		{"hello", "o$", 1, true, 5, 5},
		{"abc123", "%d+", 1, true, 4, 6},
		{"  hi", "%S+", 1, true, 3, 4},
	}
	for _, c := range cases {
		s, e, _, ok := PatternFind(c.s, c.pat, c.init)
		if ok != c.ok || s != c.start || e != c.end {
			t.Errorf("find(%q, %q) = (%d,%d,%v), want (%d,%d,%v)",
				c.s, c.pat, s, e, ok, c.start, c.end, c.ok)
		}
	}
}

func TestPatternCaptures(t *testing.T) {
	_, _, caps, ok := PatternFind("hello world", "(%w+) (%w+)", 1)
	if !ok || len(caps) != 2 {
		t.Fatalf("expected 2 captures, got %d (ok=%v)", len(caps), ok)
	}
	if caps[0] != "hello" || caps[1] != "world" {
		t.Errorf("captures = %v, want [hello world]", caps)
	}
}

func TestPatternPositionCapture(t *testing.T) {
	_, _, caps, _ := PatternFind("abc", "a()b", 1)
	if len(caps) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(caps))
	}
	if caps[0] != int64(2) {
		t.Errorf("position capture = %v, want 2", caps[0])
	}
}

func TestPatternBalanced(t *testing.T) {
	cases := []struct {
		s, pat string
		want   string
		ok     bool
	}{
		{"f(x, y(z))", "%b()", "(x, y(z))", true},
		{"no parens", "%b()", "", false},
		{"[a [b] c]", "%b[]", "[a [b] c]", true},
	}
	for _, c := range cases {
		s, e, _, ok := PatternFind(c.s, c.pat, 1)
		if ok != c.ok {
			t.Errorf("find(%q, %q) ok=%v, want %v", c.s, c.pat, ok, c.ok)
			continue
		}
		if ok && c.s[s-1:e] != c.want {
			t.Errorf("find(%q, %q) = %q, want %q", c.s, c.pat, c.s[s-1:e], c.want)
		}
	}
}

func TestPatternFrontier(t *testing.T) {
	_, _, _, ok := PatternFind(" hello world", "%f[%a]%a+", 1)
	if !ok {
		t.Errorf("frontier pattern failed to match")
	}
}

func TestPatternQuantifiers(t *testing.T) {
	cases := []struct {
		s, pat   string
		matchStr string
	}{
		{"aaab", "a*", "aaa"},
		{"aaab", "a+b", "aaab"},
		{"abc", "a?b", "ab"},
		{"xabcy", ".-c", "xabc"},
	}
	for _, c := range cases {
		s, e, _, ok := PatternFind(c.s, c.pat, 1)
		if !ok {
			t.Errorf("find(%q, %q) failed", c.s, c.pat)
			continue
		}
		got := c.s[s-1 : e]
		if got != c.matchStr {
			t.Errorf("find(%q, %q) = %q, want %q", c.s, c.pat, got, c.matchStr)
		}
	}
}

func TestPatternGSubString(t *testing.T) {
	out, n := PatternGSub("hello world", "o", "0", -1, nullCall)
	if out != "hell0 w0rld" || n != 2 {
		t.Errorf("gsub = (%q, %d), want (hell0 w0rld, 2)", out, n)
	}
}

func TestPatternGSubLimit(t *testing.T) {
	out, n := PatternGSub("aaaa", "a", "b", 2, nullCall)
	if out != "bbaa" || n != 2 {
		t.Errorf("gsub limited = (%q, %d), want (bbaa, 2)", out, n)
	}
}

func TestPatternGSubBackref(t *testing.T) {
	out, _ := PatternGSub("John Smith", "(%w+) (%w+)", "%2 %1", -1, nullCall)
	if out != "Smith John" {
		t.Errorf("gsub backref = %q, want Smith John", out)
	}
}

func TestPatternGSubWithTable(t *testing.T) {
	tbl := NewTable(0, 2)
	tbl.Set("a", "AAA")
	tbl.Set("b", "BBB")
	out, n := PatternGSub("a-b-c", "%a", tbl, -1, nullCall)
	if out != "AAA-BBB-c" || n != 3 {
		t.Errorf("gsub table = (%q, %d), want (AAA-BBB-c, 3)", out, n)
	}
}

func TestPatternGMatchIter(t *testing.T) {
	it := NewGMatchIter("the quick brown fox", "%a+", 1)
	want := []string{"the", "quick", "brown", "fox"}
	for i := 0; ; i++ {
		r := it.Next()
		if r == nil {
			if i != len(want) {
				t.Errorf("gmatch produced %d matches, want %d", i, len(want))
			}
			break
		}
		if i >= len(want) {
			t.Errorf("gmatch produced extra match: %v", r)
			break
		}
		if r[0] != want[i] {
			t.Errorf("gmatch[%d] = %v, want %q", i, r[0], want[i])
		}
	}
}

func TestPatternHasSpecials(t *testing.T) {
	cases := map[string]bool{
		"abc":     false,
		"a.c":     true,
		"hello!":  false,
		"^anchor": true,
		"[set]":   true,
		"end$":    true,
		"%w+":     true,
	}
	for pat, want := range cases {
		if PatternHasSpecials(pat) != want {
			t.Errorf("HasSpecials(%q) = %v, want %v", pat, !want, want)
		}
	}
}
