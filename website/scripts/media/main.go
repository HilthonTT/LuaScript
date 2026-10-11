// Command media renders the website's terminal GIFs, screenshots, plots and
// logo PNGs. Every program output shown is captured from a real run of the
// interpreter, so the recordings cannot drift from the language.
//
//	cd website && go run ./scripts/media            # everything
//	go run ./scripts/media -only run,types          # a subset
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	repoDir string
	outDir  string
	binPath string
	workDir string
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "media:", err)
		os.Exit(1)
	}
}

func main() {
	flag.StringVar(&repoDir, "repo", "..", "LuaScript repository root")
	flag.StringVar(&outDir, "out", "public", "website public directory")
	flag.StringVar(&binPath, "bin", "", "luascript binary (built from -repo when empty)")
	only := flag.String("only", "", "comma-separated subset: "+strings.Join(jobNames(), ","))
	flag.Parse()

	var err error
	repoDir, err = filepath.Abs(repoDir)
	check(err)
	outDir, err = filepath.Abs(outDir)
	check(err)
	check(os.MkdirAll(filepath.Join(outDir, "media"), 0o755))

	workDir, err = os.MkdirTemp("", "luascript-media-")
	check(err)
	defer os.RemoveAll(workDir)

	if binPath == "" {
		binPath = filepath.Join(workDir, "luascript")
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/luascript")
		cmd.Dir, cmd.Stdout, cmd.Stderr = repoDir, os.Stdout, os.Stderr
		check(cmd.Run())
	}

	want := map[string]bool{}
	for _, n := range strings.Split(*only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	f := loadFonts()
	for _, j := range jobs {
		if len(want) > 0 && !want[j.name] {
			continue
		}
		fmt.Println("media:", j.name)
		j.run(f)
	}
}

type job struct {
	name string
	run  func(*fonts)
}

var jobs = []job{
	{"logo", func(*fonts) { renderLogos() }},
	{"run", sceneRun},
	{"types", sceneTypes},
	{"match", sceneMatch},
	{"repl", sceneREPL},
	{"test", sceneTest},
	{"traceback", sceneTraceback},
	{"plots", func(*fonts) { renderPlots() }},
}

func jobNames() []string {
	var n []string
	for _, j := range jobs {
		n = append(n, j.name)
	}
	return n
}

func media(name string) string { return filepath.Join(outDir, "media", name) }

// luascript runs the interpreter in dir and returns combined output with
// Windows path separators normalized, so recordings match on every OS.
func luascript(dir string, args ...string) string {
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=")
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	_ = cmd.Run() // a failing script is often the point of the scene
	out := strings.ReplaceAll(b.String(), "\r\n", "\n")
	out = strings.ReplaceAll(out, `\`, "/")
	// Paths into the scratch directory read as the prompt's ~/demo.
	return strings.ReplaceAll(out, filepath.ToSlash(dir)+"/", "~/demo/")
}

func writeFile(dir, name, content string) {
	p := filepath.Join(dir, name)
	check(os.MkdirAll(filepath.Dir(p), 0o755))
	check(os.WriteFile(p, []byte(content), 0o644))
}

func sceneDir(name string) string {
	d := filepath.Join(workDir, name)
	check(os.MkdirAll(d, 0o755))
	return d
}

// ---- shared choreography ------------------------------------------------

func prompt() Line { return Line{{Text: "~/demo ", FG: cDim}, {Text: "$ ", FG: cPurple, Bold: true}} }

// typeCommand types cmd at a shell prompt, one character per frame.
func typeCommand(rec *Recorder, t *Term, cmd string) {
	t.Print(prompt())
	t.Caret = true
	rec.Snap(350)
	for i, r := range cmd {
		t.Append(Span{Text: string(r), FG: cText})
		ms := 45
		if i%5 == 3 {
			ms = 75
		}
		rec.Snap(ms)
	}
	rec.Snap(300)
	t.Caret = false
}

var sgr = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// ansiLines converts program output into styled lines, honoring the SGR
// colors the REPL and test runner emit.
func ansiLines(out string, base Span) []Line {
	var lines []Line
	for _, raw := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		cur := base
		var l Line
		pos := 0
		for _, m := range sgr.FindAllStringSubmatchIndex(raw, -1) {
			if m[0] > pos {
				l = append(l, Span{Text: raw[pos:m[0]], FG: cur.FG, Bold: cur.Bold})
			}
			for _, code := range strings.Split(raw[m[2]:m[3]], ";") {
				switch code {
				case "", "0":
					cur = base
				case "1":
					cur.Bold = true
				case "2":
					cur.FG = cDim
				case "31", "91":
					cur.FG = cRed
				case "32", "92":
					cur.FG = cOK
				case "33", "93":
					cur.FG = cYellow
				case "34", "94":
					cur.FG = cBlue
				case "35", "95":
					cur.FG = cMagenta
				case "36", "96":
					cur.FG = cCyan
				}
			}
			pos = m[1]
		}
		if pos < len(raw) {
			l = append(l, Span{Text: raw[pos:], FG: cur.FG, Bold: cur.Bold})
		}
		lines = append(lines, l)
	}
	return lines
}

// printOutput reveals output a line at a time, the way a fast program does.
func printOutput(rec *Recorder, t *Term, out string, colorize func(string) color.RGBA) {
	for _, l := range ansiLines(out, Span{FG: cText}) {
		if colorize != nil && len(l) == 1 {
			l[0].FG = colorize(l[0].Text)
		}
		t.Print(l)
		rec.Snap(60)
	}
}

func errorColor(s string) color.RGBA {
	if strings.HasPrefix(s, "luascript:") || strings.HasPrefix(s, "type-error") {
		return cRed
	}
	return cText
}

// typeCode types source into the editor, a few characters per frame, with
// a short pause at each line break.
func typeCode(rec *Recorder, e *Editor, src string, perFrame int) {
	e.Caret = true
	for _, line := range strings.Split(strings.TrimRight(src, "\n"), "\n") {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		e.Lines = append(e.Lines, line[:indent]) // editors auto-indent
		e.Row, e.Col = len(e.Lines)-1, indent
		rest := []rune(line[indent:])
		for i := 0; i < len(rest); i += perFrame {
			j := min(i+perFrame, len(rest))
			e.Lines[e.Row] += string(rest[i:j])
			e.Col += j - i
			rec.Snap(28)
		}
		rec.Snap(70)
	}
}

func newEditorWindow(f *fonts, title, file string, editorRows, termRows int) (*Window, *Recorder) {
	h := chromeH + 32 + 10 + editorRows*lineHeight + 12 + 30 + 8 + termRows*lineHeight + 14
	w := &Window{W: 980, H: h, Title: title, f: f,
		Editor: &Editor{File: file, Rows: editorRows},
		Term:   &Term{Rows: termRows},
	}
	return w, NewRecorder(w)
}

// ---- scenes ---------------------------------------------------------------

func sceneRun(f *fonts) {
	src := `type Planet = { name: string, moons: number }

local planets: { Planet } = {
    { name = "Earth", moons = 1 },
    { name = "Mars", moons = 2 },
    { name = "Jupiter", moons = 95 },
}

local function describe(p: Planet): string
    local noun = if p.moons == 1 then "moon" else "moons"
    return ` + "`{p.name} has {p.moons} {noun}`" + `
end

for _, p in ipairs(planets) do
    print(describe(p))
end
`
	dir := sceneDir("run")
	writeFile(dir, "planets.lsc", src)
	out := luascript(dir, "planets.lsc")

	w, rec := newEditorWindow(f, "planets.lsc — luascript", "planets.lsc", 16, 6)
	rec.Snap(400)
	typeCode(rec, w.Editor, src, 3)
	rec.Snap(700)
	w.Editor.Caret = false
	typeCommand(rec, w.Term, "luascript planets.lsc")
	printOutput(rec, w.Term, out, nil)
	w.Term.Print(prompt())
	w.Term.Caret = true
	rec.Snap(4000)
	rec.Save(media("run.gif"))
}

func sceneTypes(f *fonts) {
	src := `--!strict
local function area(w: number, h: number): number
    return w * h
end

print(area(3, 4))
print(area(3, "4"))
`
	fixed := strings.Replace(src, `area(3, "4")`, `area(3, 4.5)`, 1)
	dir := sceneDir("types")
	writeFile(dir, "area.lsc", src)
	bad := luascript(dir, "area.lsc")
	writeFile(dir, "area.lsc", fixed)
	good := luascript(dir, "area.lsc")

	w, rec := newEditorWindow(f, "area.lsc — luascript", "area.lsc", 8, 7)
	rec.Snap(400)
	typeCode(rec, w.Editor, src, 3)
	rec.Snap(600)
	w.Editor.Caret = false
	typeCommand(rec, w.Term, "luascript area.lsc")
	printOutput(rec, w.Term, bad, errorColor)
	w.Editor.Marked = 7
	rec.Snap(2600)

	// Fix the call in place: delete `"4"`, type `4.5`.
	e := w.Editor
	e.Caret, e.Row = true, 6
	line := e.Lines[6]
	at := strings.Index(line, `"4"`)
	e.Col = at + 3
	rec.Snap(500)
	for k := 0; k < 3; k++ {
		line = line[:e.Col-1] + line[e.Col:]
		e.Col--
		e.Lines[6] = line
		rec.Snap(90)
	}
	e.Marked = 0
	for _, r := range "4.5" {
		line = line[:e.Col] + string(r) + line[e.Col:]
		e.Col++
		e.Lines[6] = line
		rec.Snap(110)
	}
	rec.Snap(600)
	e.Caret = false
	w.Term.Print(Line{})
	typeCommand(rec, w.Term, "luascript area.lsc")
	printOutput(rec, w.Term, good, nil)
	w.Term.Print(prompt())
	w.Term.Caret = true
	rec.Snap(3500)
	rec.Save(media("types.gif"))
}

func sceneMatch(f *fonts) {
	head := `enum Shape
    Circle(number),
    Rect(number, number),
    Unit,
end

local function area(s: Shape): number
    match s do
        Shape.Circle(r)  -> return 3.14159 * r * r
        Shape.Rect(w, h) -> return w * h
`
	tail := `    end
    return -1
end

print(area(Shape.Circle(2)), area(Shape.Rect(3, 4)))
`
	unitArm := "        Shape.Unit       -> return 0\n"
	dir := sceneDir("match")
	writeFile(dir, "shapes.lsc", head+tail)
	bad := luascript(dir, "shapes.lsc")
	writeFile(dir, "shapes.lsc", head+unitArm+tail)
	good := luascript(dir, "shapes.lsc")

	w, rec := newEditorWindow(f, "shapes.lsc — luascript", "shapes.lsc", 15, 6)
	rec.Snap(400)
	typeCode(rec, w.Editor, head+tail, 3)
	rec.Snap(600)
	w.Editor.Caret = false
	typeCommand(rec, w.Term, "luascript shapes.lsc")
	printOutput(rec, w.Term, bad, errorColor)
	w.Editor.Marked = 8
	rec.Snap(2600)

	// Add the missing arm after line 10.
	e := w.Editor
	e.Marked = 0
	e.Lines = append(e.Lines[:10], append([]string{""}, e.Lines[10:]...)...)
	e.Row, e.Col, e.Caret = 10, 0, true
	rec.Snap(400)
	arm := strings.TrimRight(unitArm, "\n")
	e.Lines[10] = arm[:8]
	e.Col = 8
	for _, r := range arm[8:] {
		e.Lines[10] += string(r)
		e.Col++
		rec.Snap(40)
	}
	rec.Snap(600)
	e.Caret = false
	w.Term.Print(Line{})
	typeCommand(rec, w.Term, "luascript shapes.lsc")
	printOutput(rec, w.Term, good, nil)
	w.Term.Print(prompt())
	w.Term.Caret = true
	rec.Snap(3500)
	rec.Save(media("match.gif"))
}

// sceneREPL replays a REPL session. Readline needs a real console, so each
// value is computed by running the same code through the interpreter and is
// then printed the way the REPL's printResults does (`=> value`).
func sceneREPL(f *fonts) {
	type step struct {
		input string
		expr  bool
	}
	steps := []step{
		{`local langs = { "lua", "luau", "luascript" }`, false},
		{`#langs`, true},
		{"`hello from {langs[#langs]}`", true},
		{`local function fib(n: number): number return if n < 2 then n else fib(n - 1) + fib(n - 2) end`, false},
		{`fib(25)`, true},
		{`math.sqrt(true)`, true},
	}
	dir := sceneDir("repl")

	results := map[int]string{}
	var prog strings.Builder
	for i, s := range steps {
		if !s.expr {
			prog.WriteString(s.input + "\n")
			continue
		}
		// The REPL compiles `return <input>` as a chunk of its own, so a
		// type error is reported against that chunk alone (line 1).
		writeFile(dir, "probe.lsc", prog.String()+"return "+s.input+"\n")
		if out := luascript(dir, "probe.lsc"); strings.HasPrefix(out, "luascript: ") {
			writeFile(dir, "probe.lsc", "return "+s.input+"\n")
			out = luascript(dir, "probe.lsc")
			results[i] = "!" + strings.TrimPrefix(strings.TrimSpace(out), "luascript: ")
			continue
		}
		writeFile(dir, "probe.lsc", prog.String()+"print(tostring("+s.input+"))\n")
		results[i] = strings.TrimSpace(luascript(dir, "probe.lsc"))
	}

	banner := luascriptBanner()
	w := &Window{W: 980, H: chromeH + 8 + 20*lineHeight + 14, Title: "luascript — REPL", f: f,
		Term: &Term{Rows: 20}}
	rec := NewRecorder(w)
	t := w.Term
	typeCommand(rec, t, "luascript")
	for _, l := range banner {
		t.Print(l)
	}
	rec.Snap(900)
	replPrompt := func() Line { return Line{{Text: "luascript » ", FG: cMagenta}} }
	for i, s := range steps {
		t.Print(replPrompt())
		t.Caret = true
		rec.Snap(400)
		for k, r := range []rune(s.input) {
			t.Append(Span{Text: string(r), FG: cText})
			if k%2 == 1 {
				rec.Snap(38)
			}
		}
		rec.Snap(350)
		t.Caret = false
		if res, ok := results[i]; ok {
			if strings.HasPrefix(res, "!") {
				t.Print(Line{{Text: "type-error:", FG: cRed}, {Text: " " + res[1:], FG: cText}})
			} else {
				t.Print(Line{{Text: "=>", FG: cCyan}, {Text: " " + res, FG: cText}})
			}
		}
		rec.Snap(700)
	}
	t.Print(replPrompt())
	t.Caret = true
	rec.Snap(3500)
	rec.Save(media("repl.gif"))
}

// luascriptBanner reproduces the REPL's start screen from its source.
func luascriptBanner() []Line {
	b, err := os.ReadFile(filepath.Join(repoDir, "internal", "repl", "constants.go"))
	check(err)
	s := string(b)
	start := strings.Index(s, "const Logo = `") + len("const Logo = `")
	logo := s[start : start+strings.Index(s[start:], "`")]
	var lines []Line
	for _, l := range strings.Split(strings.Trim(logo, "\n"), "\n") {
		lines = append(lines, plain(l, cPurple))
	}
	v := strings.TrimSpace(luascript(workDir, "-v"))
	if m := regexp.MustCompile(`Version: ([^,]+)`).FindStringSubmatch(v); m != nil {
		v = m[1]
	}
	lines = append(lines,
		Line{{Text: "  luascript REPL " + v, FG: cText, Bold: true}, {Text: " — a Lua-flavored language on a stack VM", FG: cText}},
		plain("  Type 'help' for commands · Ctrl+C cancels input · Ctrl+D exits", cDim),
		Line{},
	)
	return lines
}

func sceneTest(f *fonts) {
	out := luascript(repoDir, "test", "-v", "examples/tests")
	w := &Window{W: 980, H: chromeH + 8 + 32*lineHeight + 14, Title: "luascript test", f: f,
		Term: &Term{Rows: 32}}
	rec := NewRecorder(w)
	typeCommand(rec, w.Term, "luascript test -v examples/tests")
	for _, l := range ansiLines(out, Span{FG: cText}) {
		txt := l.text()
		trim := strings.TrimSpace(txt)
		switch {
		case strings.HasPrefix(trim, "ok "):
			i := strings.Index(txt, "ok")
			l = Line{{Text: txt[:i]}, {Text: "ok", FG: cOK, Bold: true}, {Text: txt[i+2:], FG: cText}}
		case strings.HasPrefix(trim, "skip"):
			i := strings.Index(txt, "skip")
			l = Line{{Text: txt[:i]}, {Text: "skip", FG: cYellow, Bold: true}, {Text: txt[i+4:], FG: cDim}}
		case trim == "PASS":
			l = Line{{Text: "PASS", FG: cOK, Bold: true}}
		case strings.HasSuffix(trim, ".lsc"):
			l = Line{{Text: txt, FG: cPurple, Bold: true}}
		case strings.Contains(trim, "passed"):
			l = Line{{Text: txt, FG: cDim}}
		}
		w.Term.Print(l)
		rec.Snap(55)
	}
	w.Term.Print(prompt())
	w.Term.Caret = true
	rec.Snap(4000)
	rec.Save(media("test.gif"))
}

// sceneTraceback is a still: an uncaught error crossing a module boundary.
func sceneTraceback(f *fonts) {
	dir := sceneDir("traceback")
	writeFile(dir, "lib/parse.lsc", `local Parser = {}

function Parser.field(record, name)
    return record[name].value
end

function Parser.record(rows)
    return Parser.field(rows[1], "id")
end

return Parser
`)
	writeFile(dir, "app.lsc", `local Parser = require("lib.parse")

local rows = { { name = { value = "Ada" } } }
print("parsing", #rows, "row(s)")

local id = Parser.record(rows)
print(id)
`)
	out := luascript(dir, "app.lsc")
	w := &Window{W: 980, H: chromeH + 8 + 9*lineHeight + 14, Title: "luascript app.lsc", f: f,
		Term: &Term{Rows: 9}}
	rec := NewRecorder(w)
	w.Term.Print(append(prompt(), Span{Text: "luascript app.lsc", FG: cText}))
	for _, l := range ansiLines(out, Span{FG: cText}) {
		txt := l.text()
		switch {
		case strings.HasPrefix(txt, "luascript:"):
			l = Line{{Text: "luascript:", FG: cRed, Bold: true}, {Text: txt[len("luascript:"):], FG: cText}}
		case strings.HasPrefix(txt, "stack traceback"):
			l = Line{{Text: txt, FG: cYellow}}
		case strings.HasPrefix(txt, "\t"):
			parts := strings.SplitN(strings.TrimPrefix(txt, "\t"), ": ", 2)
			l = Line{{Text: "    " + parts[0], FG: cCyan}}
			if len(parts) == 2 {
				l = append(l, Span{Text: ": " + parts[1], FG: cDim})
			}
		}
		w.Term.Print(l)
	}
	w.Term.Print(prompt())
	w.Term.Caret = true
	rec.Snap(100)
	savePNG(rec.prev, media("traceback.png"))
}

// renderPlots runs the plot example and keeps its four charts.
func renderPlots() {
	dir := sceneDir("plots")
	src, err := os.ReadFile(filepath.Join(repoDir, "examples", "48_plot_module.lsc"))
	check(err)
	writeFile(dir, "plots.lsc", string(src))
	out := luascript(dir, "plots.lsc")
	for _, kind := range []string{"line", "bar", "hist", "scatter"} {
		b, err := os.ReadFile(filepath.Join(dir, "scratch_"+kind+".svg"))
		if err != nil {
			check(fmt.Errorf("plot example did not write scratch_%s.svg:\n%s", kind, out))
		}
		check(os.WriteFile(media("plot-"+kind+".svg"), b, 0o644))
	}
}
