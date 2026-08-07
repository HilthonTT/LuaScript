package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/bccache"
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	parserrors "github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/typecheck"
	"github.com/hilthontt/luascript/internal/docs"
	"github.com/hilthontt/luascript/internal/version"
	"github.com/hilthontt/luascript/internal/vm"
)

// lineReader is the subset of *readline.Instance the REPL loop needs; a
// plain buffered reader satisfies it when readline can't grab the terminal.
type lineReader interface {
	Readline() (string, error)
	SetPrompt(prompt string)
	Close() error
}

// plainLineReader is the degraded-mode input path used when readline fails
// to initialize (no controlling tty, redirected stdin, exotic consoles).
// No history or completion — just prompt + line.
type plainLineReader struct {
	sc     *bufio.Scanner
	out    io.Writer
	prompt string
}

func (p *plainLineReader) Readline() (string, error) {
	fmt.Fprint(p.out, p.prompt)
	if !p.sc.Scan() {
		if err := p.sc.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return p.sc.Text(), nil
}

func (p *plainLineReader) SetPrompt(prompt string) { p.prompt = prompt }

func (p *plainLineReader) Close() error { return nil }

type REPL struct {
	engine *engine
	rl     lineReader
	vm     *vm.VM

	// postInits run, in order, against every VM the REPL creates (the
	// initial one from NewREPL, the fresh one in RunFile, and the
	// rebuilt one behind `:reset`). This is the seam main.go uses to
	// register native modules — repl can't import native packages
	// directly because those packages already import vm.
	postInits []func(*vm.VM)

	out io.Writer
}

// AddPostInit appends a hook that runs against every VM this REPL
// owns. The hook is also applied to the current VM immediately so
// callers don't need to invoke it separately. Multiple calls compose
// — every hook runs in the order it was added.
func (r *REPL) AddPostInit(fn func(*vm.VM)) {
	if fn == nil {
		return
	}
	r.postInits = append(r.postInits, fn)
	if r.vm != nil {
		fn(r.vm)
	}
}

// runPostInits applies every registered hook to v, in order.
func (r *REPL) runPostInits(v *vm.VM) {
	for _, fn := range r.postInits {
		fn(v)
	}
}

func (r *REPL) Start() {
	fmt.Fprint(r.out, Logo)
	fmt.Fprintf(r.out, "  %sluascript REPL %s%s — a Lua-flavored language on a stack VM\n",
		colorBold, version.Version, colorReset)
	fmt.Fprintf(r.out, "  %sType 'help' for commands · Ctrl+C cancels input · Ctrl+D exits%s\n\n",
		colorDim, colorReset)

	r.runREPL()
}

func (r *REPL) RunFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}
	// Compile through the bytecode cache: an unchanged script skips the
	// whole front-end (lex → parse → typecheck → fold → codegen) on
	// re-runs. Disable with LUASCRIPT_NOCACHE=1. The returned main chunk
	// reaches nested function protos through its Protos table at runtime.
	main, err := bccache.CompileCached(string(src))
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}
	// Name the chunk after the file so runtime errors and tracebacks point
	// at it. Done after compilation rather than inside it because a cache
	// hit returns a chunk keyed on content, which two paths can share.
	main.SetSource(path)
	v := vm.New()
	// Let `require` resolve modules sitting next to the script, not just
	// ones under the process's cwd.
	if abs, aerr := filepath.Abs(path); aerr == nil {
		v.AddScriptDir(filepath.Dir(abs))
	}
	r.runPostInits(v)
	if err := v.Run(main); err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}
}

// NewREPL builds a REPL bound to v. The `in` argument is accepted for API
// stability but unused: input is read through the readline instance, which
// drives its own terminal handle.
func NewREPL(v *vm.VM, in io.Reader, out io.Writer) *REPL {
	historyFile := ""
	if home, err := os.UserHomeDir(); err == nil {
		historyFile = filepath.Join(home, ".lsc_repl_history")
	}

	var rl lineReader
	inst, err := readline.NewEx(&readline.Config{
		Prompt:                 promptReady,
		HistoryFile:            historyFile,
		AutoComplete:           newCompleter(),
		InterruptPrompt:        "\nInterrupted (Ctrl+D to exit)",
		EOFPrompt:              "exit",
		HistorySearchFold:      true,
		DisableAutoSaveHistory: false,
	})
	if err != nil {
		// Degrade to a dumb line reader rather than crashing on the first
		// Readline call — no history/completion, but the REPL still works.
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize readline: %v\n", err)
		if in == nil {
			in = os.Stdin
		}
		rl = &plainLineReader{sc: bufio.NewScanner(in), out: out, prompt: promptReady}
	} else {
		rl = inst
	}

	return &REPL{
		engine: newEngine(v),
		rl:     rl,
		vm:     v,
		out:    out,
	}
}

func (r *REPL) runREPL() {
	defer r.rl.Close()

	for {
		line, err := r.rl.Readline()
		if err != nil { // EOF or interrupt at top level
			r.bye()
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// `doc <topic>` is a command rather than an expression: it takes an
		// argument, so it cannot be matched by the exact-match switch below.
		if query, ok := docCommand(line); ok {
			r.printDoc(query)
			continue
		}

		switch line {
		case cmdExit, cmdQuit:
			r.bye()
			return
		case cmdHelp:
			r.printHelp()
			continue
		case cmdClear:
			fmt.Fprint(r.out, "\033[H\033[2J")
			continue
		case cmdReset:
			r.vm = vm.New()
			r.runPostInits(r.vm)
			r.engine = newEngine(r.vm)
			fmt.Fprintf(r.out, "%s(REPL state reset — globals cleared)%s\n",
				colorDim, colorReset)
			continue
		}

		r.processInput(line)
	}
}

func (r *REPL) bye() {
	fmt.Fprintf(r.out, "\n%sGoodbye!%s\n", colorDim, colorReset)
}

func (r *REPL) printError(err error) {
	// Surface type-check errors on a per-line basis with a distinct
	// `type-error:` prefix so users can tell them apart from runtime
	// errors. Runtime/parse errors keep the standard `luascript:` prefix.
	if te, ok := err.(*typecheck.TypeErrors); ok {
		for _, e := range te.Errors {
			fmt.Fprintf(os.Stderr, "%stype-error:%s %s\n",
				colorErr, colorReset, e.Format())
		}
		return
	}
	fmt.Fprintf(os.Stderr, "%sluascript:%s %v\n", colorErr, colorReset, err)
}

func (r *REPL) processInput(input string) {
	var src strings.Builder
	src.WriteString(input)

	// For inputs that don't look like a statement, try wrapping with
	// `return` so bare expressions print their value. If the wrapped form
	// fails to *compile*, fall through to a normal statement compile (so
	// things like `x = 5` still work). If it compiles but fails at
	// runtime, surface that error directly — re-running as a statement
	// would double-execute side effects of the same chunk.
	if !looksLikeStatement(input) {
		if chunks, err := r.engine.compile("return " + input); err == nil {
			results, runErr := r.engine.runMainWithResults(chunks[0])
			if runErr != nil {
				r.printError(runErr)
				return
			}
			r.printResults(results)
			return
		}
	}

	// Compile as a full statement / block.
	chunks, cerr := r.engine.compile(input)
	if cerr != nil {
		if r.handleIncompleteInput(cerr, &src) {
			return // continue reading more lines
		}
		r.printError(cerr)
		return
	}

	if err := r.engine.runMain(chunks[0]); err != nil {
		r.printError(err)
	}
}

// handleIncompleteInput returns true if we should continue reading more input
func (r *REPL) handleIncompleteInput(err error, src *strings.Builder) bool {
	var perr *parserrors.Error
	if !asParserErr(err, &perr) || !perr.IsEOF() {
		return false
	}

	// Incomplete input → enter continuation mode. Set the readline prompt
	// so the line-edit state stays aligned (drawing contPrompt manually
	// via Fprint leaves readline unaware of the column).
	r.rl.SetPrompt(contPrompt)
	defer r.rl.SetPrompt(promptReady)

	for {
		line, err := r.rl.Readline()
		if err != nil {
			return false
		}

		src.WriteByte('\n')
		src.WriteString(line)

		chunks, cerr := r.engine.compile(src.String())
		if cerr == nil {
			// Successfully completed
			if runErr := r.engine.runMain(chunks[0]); runErr != nil {
				r.printError(runErr)
			}
			return true
		}

		if !isIncompleteError(cerr) {
			r.printError(cerr)
			return true
		}
		// Still incomplete — readline already shows contPrompt for the
		// next call.
	}
}

func isIncompleteError(err error) bool {
	var perr *parserrors.Error
	return asParserErr(err, &perr) && perr.IsEOF()
}

// Simple heuristic: if it starts with common statement keywords, treat as statement
func looksLikeStatement(line string) bool {
	t := strings.TrimLeft(line, " \t")
	for _, kw := range []string{"if", "for", "while", "function", "local", "do", "repeat"} {
		if strings.HasPrefix(t, kw) {
			return true
		}
	}
	return false
}

func (r *REPL) printResults(results []vm.Value) {
	if len(results) == 0 {
		return
	}
	parts := make([]string, len(results))
	for i, val := range results {
		parts[i] = vm.ToStringMM(r.vm, val)
	}
	fmt.Fprintf(r.out, "%s=>%s %s\n", colorOK, colorReset, strings.Join(parts, "\t"))
}

// docCommand recognises the `doc [topic]` REPL command and returns the
// topic asked for — empty for a bare `doc`, which prints the index. It
// deliberately does NOT match `doc(...)` or `doc = 1`: those are ordinary
// luascript, and a user who defines their own `doc` should keep it.
func docCommand(line string) (query string, ok bool) {
	if line == cmdDoc {
		return "", true
	}
	rest, found := strings.CutPrefix(line, cmdDoc+" ")
	if !found {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.ContainsAny(rest, "=(){}\"'") {
		return "", false
	}
	return rest, true
}

// printDoc renders a stdlib page inside the REPL — the same pages
// `luascript doc` prints, from the same registry. A bare `doc` shows the
// index; `doc math.floor` shows one entry.
func (r *REPL) printDoc(query string) {
	opts := docs.Options{Width: 80, Color: true}
	if query == "" {
		fmt.Fprint(r.out, docs.RenderIndex(opts))
		return
	}
	topic, entry, ok := docs.Lookup(query)
	if !ok {
		fmt.Fprintf(r.out, "%sno documentation for %q%s\n", colorErr, query, colorReset)
		if sugg := docs.Suggest(query); len(sugg) > 0 {
			fmt.Fprintf(r.out, "%sdid you mean: %s%s\n",
				colorDim, strings.Join(sugg, ", "), colorReset)
		}
		return
	}
	if entry != nil {
		fmt.Fprint(r.out, docs.RenderEntry(topic, entry, opts))
		return
	}
	fmt.Fprint(r.out, docs.RenderTopic(topic, opts))
}

func (r *REPL) printHelp() {
	fmt.Fprint(r.out, Logo)
	fmt.Fprintf(r.out, "  %sluascript REPL %s%s\n\n", colorBold, version.Version, colorReset)

	fmt.Fprintf(r.out, "%sCommands%s\n", colorBold, colorReset)
	rows := []struct{ name, desc string }{
		{cmdHelp, "show this help"},
		{cmdDoc + " <topic>", "stdlib reference; bare " + cmdDoc + " lists every topic"},
		{cmdExit + ", " + cmdQuit, "exit the REPL"},
		{cmdReset, "rebuild the VM (clears all globals and user state)"},
		{cmdClear, "clear the screen"},
	}
	for _, row := range rows {
		fmt.Fprintf(r.out, "  %s%-14s%s %s\n", colorBold, row.name, colorReset, row.desc)
	}

	fmt.Fprintf(r.out, "\n%sKey bindings%s\n", colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s%-14s%s cancel current input\n", colorBold, "Ctrl+C", colorReset)
	fmt.Fprintf(r.out, "  %s%-14s%s exit the REPL\n", colorBold, "Ctrl+D", colorReset)
	fmt.Fprintf(r.out, "  %s%-14s%s search history (readline)\n", colorBold, "Ctrl+R", colorReset)

	fmt.Fprintf(r.out, "\n%sTips%s\n", colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s•%s bare expressions print their result:  %s1+2%s  → %s3%s\n",
		colorDim, colorReset, colorBold, colorReset, colorOK, colorReset)
	fmt.Fprintf(r.out, "  %s•%s incomplete input opens a continuation prompt (%s   …%s)\n",
		colorDim, colorReset, colorErr, colorReset)
	fmt.Fprintf(r.out, "  %s•%s top-level %slocal x = v%s persists across REPL inputs\n",
		colorDim, colorReset, colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s•%s Luau-style types are checked: %slocal x: number = 1%s, %stype P = {x: number}%s\n",
		colorDim, colorReset, colorBold, colorReset, colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s•%s start a chunk with %s--!nocheck%s to skip type checking\n",
		colorDim, colorReset, colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s•%s %smatch%s evaluates by pattern:  %smatch n do 1 -> \"one\" _ -> \"many\" end%s\n",
		colorDim, colorReset, colorBold, colorReset, colorBold, colorReset)
	fmt.Fprintf(r.out, "  %s•%s compound assignment is supported:  %s+= -= *= /= &= |= <<= >>=%s\n",
		colorDim, colorReset, colorBold, colorReset)

	fmt.Fprintf(r.out, "\n%sFor CLI options:%s luascript --help\n\n", colorDim, colorReset)
}

func (r *REPL) DisassembleFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}
	chunks, err := compiler.CompileToInstructions(string(src), parser.NormalMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}

	fmt.Print(bytecode.Disassemble(chunks))
}

func newCompleter() *readline.PrefixCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(luaKeywords))
	for _, k := range luaKeywords {
		items = append(items, readline.PcItem(k))
	}
	return readline.NewPrefixCompleter(items...)
}

func asParserErr(err error, target **parserrors.Error) bool {
	for err != nil {
		if e, ok := err.(*parserrors.Error); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
