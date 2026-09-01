package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func TestFormat_Idempotence(t *testing.T) {
	for _, src := range goldenExamples(t) {
		t.Run(src.name, func(t *testing.T) {
			once, err := Format(src.text, Options{})
			if err != nil {
				t.Fatalf("first Format failed: %v", err)
			}
			twice, err := Format(once, Options{})
			if err != nil {
				t.Fatalf("second Format failed: %v", err)
			}
			if once != twice {
				t.Fatalf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
			}
		})
	}
}

func TestFormat_KnownSnippets(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "local_simple",
			in:   "local   x =1\n",
			want: "local x = 1\n",
		},
		{
			name: "if_else",
			in:   "if x>0 then print(1) else print(2) end\n",
			want: "if x > 0 then\n  print(1)\nelse\n  print(2)\nend\n",
		},
		{
			name: "table_short",
			in:   "local p = {x=1, y=2}\n",
			want: "local p = { x = 1, y = 2 }\n",
		},
		{
			name: "for_numeric",
			in:   "for i=1,10 do print(i) end\n",
			want: "for i = 1, 10 do\n  print(i)\nend\n",
		},
		{
			name: "method_call",
			in:   "obj:foo(1,2,3)\n",
			want: "obj:foo(1, 2, 3)\n",
		},
		{
			name: "type_alias",
			in:   "type   Point={x:number,y:number}\n",
			want: "type Point = { x: number, y: number }\n",
		},
		{
			name: "blank_lines_collapsed",
			in:   "local a = 1\n\n\n\nlocal b = 2\n",
			want: "local a = 1\n\nlocal b = 2\n",
		},
		{
			name: "comment_preserved",
			in:   "-- header\nlocal x = 1\n",
			want: "-- header\nlocal x = 1\n",
		},
		{
			name: "mode_directive_preserved",
			in:   "--!strict\nlocal x: number = 1\n",
			want: "--!strict\nlocal x: number = 1\n",
		},
		{
			name: "generic_function_type_params",
			in:   "local function identity<T>(x: T): T return x end\n",
			want: "local function identity<T>(x: T): T\n  return x\nend\n",
		},
		{
			name: "generic_type_alias",
			in:   "type Pair<A,B> = {first:A, second:B}\n",
			want: "type Pair<A, B> = { first: A, second: B }\n",
		},
		{
			name: "tagged_enum_payloads",
			in:   "enum Shape Circle(number), Rect(number, number), Unit end\n",
			want: "enum Shape\n  Circle(number),\n  Rect(number, number),\n  Unit,\nend\n",
		},
		{
			name: "match_arm_return_not_parenthesized",
			in:   "match v do\n1 -> return \"a\" .. b\nend\n",
			want: "match v do\n  1 -> return \"a\" .. b\nend\n",
		},
		{
			name: "match_arm_semicolon_before_string_pattern",
			in:   "match s do\n\"hi\" -> print(\"g\");\n\"bye\" -> print(\"f\")\nend\n",
			want: "match s do\n  \"hi\" -> print(\"g\");\n  \"bye\" -> print(\"f\")\nend\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Format(c.in, Options{})
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if got != c.want {
				t.Fatalf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", c.want, got)
			}
		})
	}
}

func TestFormat_ExamplesParseUnchanged(t *testing.T) {
	for _, ex := range goldenExamples(t) {
		t.Run(ex.name, func(t *testing.T) {
			out, err := Format(ex.text, Options{})
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if _, err := Format(out, Options{}); err != nil {
				t.Fatalf("formatted output failed to re-parse: %v\n%s", err, out)
			}
		})
	}
}

func TestFormat_ExamplesStillCompile(t *testing.T) {
	for _, ex := range goldenExamples(t) {
		t.Run(ex.name, func(t *testing.T) {
			if _, err := compiler.CompileToInstructions(ex.text, parser.NormalMode); err != nil {
				t.Skipf("example does not compile as-is: %v", err)
			}
			out, err := Format(ex.text, Options{})
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if _, err := compiler.CompileToInstructions(out, parser.NormalMode); err != nil {
				t.Fatalf("formatted output failed to compile: %v\n%s", err, out)
			}
		})
	}
}

type goldenFile struct {
	name string
	text string
}

func goldenExamples(t *testing.T) []goldenFile {
	t.Helper()
	dir := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("examples dir unavailable: %v", err)
		return nil
	}
	var out []goldenFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lsc") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out = append(out, goldenFile{name: e.Name(), text: string(b)})
	}
	return out
}
