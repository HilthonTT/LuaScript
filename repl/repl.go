package repl

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/parser"
	parserrors "github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

type REPL struct {
	engine *engine
	rl     *readline.Instance
	vm     *vm.VM

	out io.Writer
}

func (r *REPL) Start() {
	fmt.Fprint(r.out, Logo)
	fmt.Fprintf(r.out, "  %sSakura REPL %s%s — a Lua-flavored language on a stack VM\n",
		colorBold, version.Version, colorReset)
	fmt.Fprintf(r.out, "  %sType 'help' for commands · Ctrl+C cancels input · Ctrl+D exits%s\n\n",
		colorDim, colorReset)

	r.runREPL()
}

func (r *REPL) RunFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		os.Exit(1)
	}
	chunks, err := compiler.CompileToInstructions(string(src), parser.NormalMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		os.Exit(1)
	}
	v := vm.New()
	// chunks[0] is the main chunk; nested function protos follow and are
	// reached through the main chunk's Protos table at runtime.
	if err := v.Run(chunks[0]); err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		os.Exit(1)
	}
}

// NewREPL builds a REPL bound to v. The `in` argument is accepted for API
// stability but unused: input is read through the readline instance, which
// drives its own terminal handle.
func NewREPL(v *vm.VM, _ io.Reader, out io.Writer) *REPL {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:                 promptReady,
		HistoryFile:            os.ExpandEnv("$HOME/.sakura_repl_history"),
		AutoComplete:           newCompleter(),
		InterruptPrompt:        "\nInterrupted (Ctrl+D to exit)",
		EOFPrompt:              "exit",
		HistorySearchFold:      true,
		DisableAutoSaveHistory: false,
	})
	if err != nil {
		// fallback
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize readline: %v\n", err)
	}

	v.InitForREPL()

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
			r.vm.InitForREPL()
			r.engine = newEngine(r.vm)
			fmt.Fprintf(r.out, "%s(REPL state reset — globals cleared)%s\n",
				colorDim, colorReset)
			continue
		}

		r.processInput(line)
	}
}

func (r *REPL) bye() {
	fmt.Fprintf(r.out, "\n%sGoodbye! 🌸%s\n", colorDim, colorReset)
}

func (r *REPL) printError(err error) {
	fmt.Fprintf(os.Stderr, "%ssakura:%s %v\n", colorErr, colorReset, err)
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
	for i, v := range results {
		parts[i] = vm.ToString(v)
	}
	fmt.Fprintf(r.out, "%s=>%s %s\n", colorOK, colorReset, strings.Join(parts, "\t"))
}

func (r *REPL) printHelp() {
	fmt.Fprint(r.out, Logo)
	fmt.Fprintf(r.out, "  %sSakura REPL %s%s\n\n", colorBold, version.Version, colorReset)

	fmt.Fprintf(r.out, "%sCommands%s\n", colorBold, colorReset)
	rows := []struct{ name, desc string }{
		{cmdHelp, "show this help"},
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

	fmt.Fprintf(r.out, "\n%sFor CLI options:%s sakura --help\n\n", colorDim, colorReset)
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
