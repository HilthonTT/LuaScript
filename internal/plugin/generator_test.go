package plugin

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The generator is the half of this package that runs everywhere, so it is
// tested everywhere — including on Windows, where plugins cannot be loaded.

func TestGenerateSourceIsValidGo(t *testing.T) {
	s := &spec{
		Packages: []pkg{
			{Name: "strings"},
			{Name: "database/sql"},
			{Prefix: "_", Name: "github.com/lib/pq"},
			{Prefix: "sm", Name: "sync"},
		},
		Functions: []function{
			{Pkg: "strings", Name: "ToUpper", As: "ToUpper"},
			{Pkg: "sql", Name: "Open", As: "Open"},
			{Pkg: "sm", Name: "OnceFunc", As: "OnceFunc"},
		},
	}

	src, err := generateSource(s)
	if err != nil {
		t.Fatalf("generateSource: %v", err)
	}

	// The strongest assertion available without a compiler: the emitted file
	// must parse as Go.
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse as Go: %v\n---\n%s", err, src)
	}

	// Regressions for the two keyword substitutions the WIP generator carried
	// over from a sed rename, either of which produces a file that cannot
	// compile.
	if strings.Contains(src, "require(") {
		t.Error("generated Go source contains `require(`; it must use `import (`")
	}
	if strings.Contains(src, "local ") {
		t.Error("generated Go source contains `local `; symbols must be declared with `var`")
	}

	// And for the html/template escaping bug: import paths must keep real quotes.
	if strings.Contains(src, "&#34;") {
		t.Error("generated source is HTML-escaped; use text/template, not html/template")
	}

	for _, want := range []string{
		`"strings"`,
		`_ "github.com/lib/pq"`,
		`sm "sync"`,
		"var ToUpper = strings.ToUpper",
		"var Open = sql.Open",
		"func main() {}",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source missing %q\n---\n%s", want, src)
		}
	}
}

func TestGenerateSourceAliasesSymbol(t *testing.T) {
	s := &spec{
		Packages:  []pkg{{Name: "strings"}},
		Functions: []function{{Pkg: "strings", Name: "ToUpper", As: "Upper"}},
	}
	src, err := generateSource(s)
	if err != nil {
		t.Fatalf("generateSource: %v", err)
	}
	if !strings.Contains(src, "var Upper = strings.ToUpper") {
		t.Errorf("`as` not honoured:\n%s", src)
	}
}

func TestSpecValidate(t *testing.T) {
	base := func(mut func(*spec)) *spec {
		s := &spec{
			Packages:  []pkg{{Name: "strings"}},
			Functions: []function{{Pkg: "strings", Name: "ToUpper", As: "ToUpper"}},
		}
		mut(s)
		return s
	}

	tests := []struct {
		name string
		spec *spec
		want string // substring of the expected error; "" means it must pass
	}{
		{"valid", base(func(*spec) {}), ""},
		{"no packages", base(func(s *spec) { s.Packages = nil }), "no packages"},
		{"no functions", base(func(s *spec) { s.Functions = nil }), "no functions"},
		{
			"unexported symbol",
			base(func(s *spec) { s.Functions[0].Name, s.Functions[0].As = "toUpper", "toUpper" }),
			"unexported",
		},
		{
			// A spec is interpolated into Go source, so a name carrying a
			// quote or a newline must be rejected rather than smuggled in.
			"injection via package name",
			base(func(s *spec) { s.Packages[0].Name = "strings\"\n\nfunc init() { panic(1) }\nvar _ = \"" }),
			"not a valid Go import path",
		},
		{
			"injection via function name",
			base(func(s *spec) { s.Functions[0].Name = "ToUpper; var X = 1" }),
			"not a valid Go identifier",
		},
		{
			"bad import alias",
			base(func(s *spec) { s.Packages[0].Prefix = "not an ident" }),
			"not a valid import alias",
		},
		{
			"duplicate symbol",
			base(func(s *spec) {
				s.Functions = append(s.Functions, function{Pkg: "strings", Name: "ToUpper", As: "ToUpper"})
			}),
			"duplicate plugin symbol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("expected valid spec, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestExternalImports(t *testing.T) {
	s := &spec{Packages: []pkg{
		{Name: "strings"},
		{Name: "database/sql"},
		{Prefix: "_", Name: "github.com/lib/pq"},
		{Name: "gopkg.in/yaml.v3"},
	}}
	got := externalImports(s)
	want := []string{"github.com/lib/pq", "gopkg.in/yaml.v3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
