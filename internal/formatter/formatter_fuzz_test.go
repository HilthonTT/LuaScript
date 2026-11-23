package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzFormat checks that formatting never panics and that a script which
// parses still parses after formatting.
func FuzzFormat(f *testing.F) {
	seeds := []string{
		"",
		"local x = 1 -- c\n",
		"--[[ block ]] local t = { 1, 2, x = 3 }",
		"enum E { A, B(number) }",
		"local s = `x {y} z`",
		"if a then elseif b then else end",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	if entries, err := os.ReadDir(filepath.Join("..", "..", "examples")); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".lsc") {
				continue
			}
			if b, err := os.ReadFile(filepath.Join("..", "..", "examples", e.Name())); err == nil {
				f.Add(string(b))
			}
		}
	}

	f.Fuzz(func(t *testing.T, input string) {
		out, err := Format(input, Options{})
		if err != nil {
			if strings.Contains(err.Error(), "panic during parse") {
				t.Fatalf("parser panicked on %q: %v", input, err)
			}
			return
		}
		if _, err := Format(out, Options{}); err != nil {
			t.Fatalf("formatted output no longer parses: %v\ninput: %q\noutput: %q", err, input, out)
		}
	})
}
