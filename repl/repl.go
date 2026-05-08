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

type replState int

const (
	stateReady   replState = iota // empty buffer, expecting a fresh chunk
	stateWaiting                  // accumulating across continuation prompts
)

type REPL struct {
	engine *engine
	cmds   []string
	state  replState
	rl     *readline.Instance
	vm     *vm.VM

	in  io.Reader
	out io.Writer
}

func (r *REPL) Start() {
	fmt.Fprint(r.out, Logo)
	fmt.Fprintf(r.out, "  Sakura REPL %s\n", version.Version)
	fmt.Fprintln(r.out, "Type 'help' for commands, Ctrl+D to exit.")

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

func NewREPL(v *vm.VM, in io.Reader, out io.Writer) *REPL {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:                 promptReady,
		HistoryFile:            os.ExpandEnv("$HOME/.sakura_repl_history"),
		AutoComplete:           newCompleter(),
		InterruptPrompt:        "\nInterrupted",
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
		in:     in,
		out:    out,
	}
}

func (r *REPL) Reset() {
	r.cmds = nil
	r.state = stateReady
}

func (r *REPL) runREPL() {
	defer r.rl.Close()

	for {
		line, err := r.rl.Readline()
		if err != nil { // EOF or interrupt
			fmt.Fprintln(r.out, "\nGoodbye! 🌸")
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch line {
		case cmdExit, cmdQuit:
			fmt.Fprintln(r.out, "Goodbye! 🌸")
			return
		case cmdHelp:
			r.printHelp()
			continue
		case cmdClear:
			fmt.Fprintln(r.out, "\033[H\033[2J")
			continue
		case cmdReset:
			fmt.Fprintln(r.out, "(REPL state reset)")
			continue
		}

		r.processInput(line)
	}
}

func (r *REPL) processInput(input string) {
	var src strings.Builder
	src.WriteString(input)

	// Try as expression first (most common REPL use case)
	exprSrc := input
	if !looksLikeStatement(input) {
		exprSrc = "return " + input
	}

	// First try: as expression
	if chunks, err := r.engine.compile(exprSrc); err == nil {
		if results, runErr := r.engine.runMainWithResults(chunks[0]); runErr == nil {
			r.printResults(results)
			return
		}
	}

	// Second try: as full statement / block
	chunks, cerr := r.engine.compile(input)
	if cerr != nil {
		if r.handleIncompleteInput(cerr, &src) {
			return // continue reading more lines
		}
		fmt.Fprintf(os.Stderr, "sakura: %v\n", cerr)
		return
	}

	if err := r.engine.runMain(chunks[0]); err != nil {
		fmt.Fprintf(os.Stderr, "sakura: %v\n", err)
	}
}

// handleIncompleteInput returns true if we should continue reading more input
func (r *REPL) handleIncompleteInput(err error, src *strings.Builder) bool {
	var perr *parserrors.Error
	if !asParserErr(err, &perr) || !perr.IsEOF() {
		return false
	}

	// Incomplete input → enter continuation mode
	fmt.Fprint(r.out, contPrompt)

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
				fmt.Fprintf(os.Stderr, "sakura: %v\n", runErr)
			}
			return true
		}

		if !isIncompleteError(cerr) {
			fmt.Fprintf(os.Stderr, "sakura: %v\n", cerr)
			return true
		}

		// Still incomplete
		fmt.Fprint(r.out, contPrompt)
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
	fmt.Fprintln(r.out, "=>", strings.Join(parts, "\t"))
}

func (r *REPL) printHelp() {
	fmt.Print(Logo)
	fmt.Printf("  Version: %s\n\n", version.Version)
	fmt.Println("REPL Commands:")
	fmt.Println("  help     - show this help")
	fmt.Println("  exit     - exit REPL (Ctrl+D also works)")
	fmt.Println("  reset    - reset REPL state")
	fmt.Println()
	fmt.Println("For full CLI options run: sakura --help")
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
