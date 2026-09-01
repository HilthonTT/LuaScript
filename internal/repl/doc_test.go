package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestDocCommand(t *testing.T) {
	tests := []struct {
		line      string
		wantQuery string
		wantOK    bool
	}{
		{"doc", "", true},
		{"doc math", "math", true},
		{"doc math.floor", "math.floor", true},
		{"doc   std.stack  ", "std.stack", true},
		{"doc = 1", "", false},
		{`doc("x")`, "", false},
		{"doctor", "", false},
		{"document.write", "", false},
		{"local doc = 1", "", false},
	}
	for _, tc := range tests {
		query, ok := docCommand(tc.line)
		if ok != tc.wantOK || query != tc.wantQuery {
			t.Errorf("docCommand(%q) = %q, %v; want %q, %v",
				tc.line, query, ok, tc.wantQuery, tc.wantOK)
		}
	}
}

func TestPrintDoc(t *testing.T) {
	var buf bytes.Buffer
	r := &REPL{out: &buf}

	r.printDoc("json")
	if !strings.Contains(buf.String(), "json.encode") {
		t.Errorf("printDoc(json) missing json.encode:\n%s", buf.String())
	}

	buf.Reset()
	r.printDoc("")
	if !strings.Contains(buf.String(), "MODULES") {
		t.Errorf("a bare doc should print the index:\n%s", buf.String())
	}

	buf.Reset()
	r.printDoc("nosuchthing")
	if !strings.Contains(buf.String(), "no documentation") {
		t.Errorf("printDoc on a miss should say so:\n%s", buf.String())
	}
}

func TestPrintDocSuggestsOnTypo(t *testing.T) {
	var buf bytes.Buffer
	r := &REPL{out: &buf}
	r.printDoc("strng")
	if !strings.Contains(buf.String(), "string") {
		t.Errorf("printDoc should suggest string for strng:\n%s", buf.String())
	}
}
