// REPL — interactive Lua-style read/eval/print loop, modelled on goby's
// `igb` package (https://github.com/goby-lang/goby/blob/master/igb/repl.go).
//
// The package follows goby's two-struct split: `engine` mirrors goby's
// `iVM` and owns the execution state (VM + persistent generator), while
// `REPL` mirrors `iGb` and owns the input buffer, FSM-style state, and
// the readline session. Splitting them clarifies what survives a Reset
// (engine state — globals, generator) versus what gets cleared (input
// buffer, current state).
//
// Input handling mirrors the reference `lua` interpreter: each completed
// turn is first compiled as `return <input>;` so a bare expression prints
// its value, falling back to compiling it as a statement chunk if the
// expression form doesn't parse. A parser EOF error means "still typing"
// and the loop accumulates more input under a continuation prompt.
package vm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/mattn/go-colorable"

	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/bytecode"
	"github.com/hilthontt/sakura-lang/compiler/parser"
	parserrors "github.com/hilthontt/sakura-lang/compiler/parser/errors"
)

// ---------------------------------------------------------------------------
// Constants — colored prompts (goby uses » for ready, ¤ for waiting; we
// stick with the more familiar Lua-style > / >> but keep the colors).
// ---------------------------------------------------------------------------

const (
	promptReady   = "\033[32m> \033[0m"  // green
	promptWaiting = "\033[31m>> \033[0m" // red

	// Special commands the REPL intercepts before compiling. Mirrors
	// goby's `help` / `reset`; we add `exit`/`quit` for an explicit way
	// out (Ctrl-D works too).
	cmdExit  = "exit"
	cmdQuit  = "quit"
	cmdReset = "reset"
	cmdHelp  = "help"
)

// luaKeywords — used by the readline tab-completer.
var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	cmdHelp, cmdReset, cmdExit, cmdQuit,
}

// errExit is returned by Feed when a special command asked the loop to
// terminate. Internal sentinel; not exported.
var errExit = errors.New("repl: exit requested")

// ---------------------------------------------------------------------------
// VM-level helpers retained from the previous REPL stub.
// ---------------------------------------------------------------------------

// InitForREPL marks this VM as being driven from a REPL. The flag is
// purely informational today; the parser branches on its own Mode value.
func (vm *VM) InitForREPL() {
	vm.mode = parser.REPLMode
}

// GetREPLResult pops the top stack value and renders it as a string.
// Retained for embedders that drive the VM directly without going through
// the REPL struct.
func (vm *VM) GetREPLResult() string {
	top := vm.pop()
	if top != nil {
		return ToString(top)
	}
	return ""
}

// ---------------------------------------------------------------------------
// State — replaces the old NeedsMore() bool with a goby-style enum so the
// prompt selector and any future input-mode (e.g. inside a long string) can
// extend it without changing call sites.
// ---------------------------------------------------------------------------

type replState int

const (
	stateReady   replState = iota // empty buffer, expecting a fresh chunk
	stateWaiting                  // accumulating across continuation prompts
)

// ---------------------------------------------------------------------------
// engine — goby's `iVM` analog. Owns everything that survives a buffer
// reset: the VM (and its globals) and the persistent bytecode generator.
// ---------------------------------------------------------------------------

type engine struct {
	vm  *VM
	gen *bytecode.Generator
}

func newEngine(v *VM) *engine {
	return &engine{vm: v, gen: bytecode.NewGenerator()}
}

func (e *engine) compile(src string) ([]*bytecode.InstructionSet, error) {
	return compiler.CompileToInstructionsWith(e.gen, src, parser.REPLMode)
}

func (e *engine) runMain(chunk *bytecode.InstructionSet) error {
	return e.vm.Run(chunk)
}

func (e *engine) runMainWithResults(chunk *bytecode.InstructionSet) ([]Value, error) {
	return e.vm.RunMainChunkWithResults(chunk)
}

// ---------------------------------------------------------------------------
// REPL — goby's `iGb` analog. Owns input buffer, state, output writer.
// ---------------------------------------------------------------------------

// REPL is the interactive shell. Construct one per session via NewREPL.
type REPL struct {
	engine *engine
	cmds   []string
	state  replState
	out    io.Writer
}

// NewREPL wraps `v` in a REPL. The VM is reused across every turn, so
// globals defined in one input survive into the next.
func NewREPL(v *VM) *REPL {
	v.InitForREPL()
	return &REPL{engine: newEngine(v), state: stateReady}
}

// State returns the current input mode. Mostly useful for tests.
func (r *REPL) State() replState { return r.state }

// NeedsMore reports whether the buffer is mid-statement. Kept on the API
// for callers (and main.go) that want to ask without importing replState.
func (r *REPL) NeedsMore() bool { return r.state == stateWaiting }

// Reset drops any in-progress multi-line input and returns to the ready
// state. Wired to Ctrl-C in the readline path and to the `reset` command.
func (r *REPL) Reset() {
	r.cmds = nil
	r.state = stateReady
}

// switchPrompt returns the ANSI-colored prompt for the current state.
// Mirrors goby's `switchPrompt(s int) string` helper.
func (r *REPL) switchPrompt() string {
	switch r.state {
	case stateWaiting:
		return promptWaiting
	default:
		return promptReady
	}
}

// ---------------------------------------------------------------------------
// Feed — the eval half of REPL. Splits special commands, expression form,
// statement form. Pushes the parser-error classification into a dedicated
// dispatcher so the read-loop is decision-free.
// ---------------------------------------------------------------------------

// Feed processes a single source line. On a real parse or runtime error,
// the buffer is cleared and the error is returned. EOF-mid-statement is
// not an error — the state advances to stateWaiting and Feed returns nil.
// Returning errExit means the caller should terminate the loop.
func (r *REPL) Feed(line string) error {
	trimmed := strings.TrimSpace(line)

	// Escape commands fire in any state — the whole point of `reset` and
	// `exit` is to bail out when you're stuck mid-block. The chance of
	// these identifiers showing up as a bare Lua statement is low enough
	// that intercepting them is the right trade.
	switch trimmed {
	case cmdExit, cmdQuit:
		return errExit
	case cmdReset:
		had := len(r.cmds) > 0
		r.Reset()
		if had {
			fmt.Fprintln(r.out, "(input cleared)")
		} else {
			fmt.Fprintln(r.out, "(nothing to clear)")
		}
		return nil
	}

	// `help` only fires when the buffer is empty — it's plausible (if
	// unusual) Lua code to call a global named `help`, so don't shadow it
	// once the user is in the middle of typing a chunk.
	if r.state == stateReady {
		switch trimmed {
		case "":
			return nil
		case cmdHelp:
			r.printHelp()
			return nil
		}
	}

	r.cmds = append(r.cmds, line)
	src := strings.Join(r.cmds, "\n")

	// Lua-5.1 REPL shorthand: a leading `=` means "print this expression".
	exprSrc := src
	if t := strings.TrimLeft(exprSrc, " \t"); strings.HasPrefix(t, "=") {
		exprSrc = strings.Replace(exprSrc, "=", "", 1)
	}

	// Expression form first: bare expressions print their value.
	if chunks, cerr := r.engine.compile("return " + exprSrc + ";"); cerr == nil {
		r.Reset()
		results, runErr := r.engine.runMainWithResults(chunks[0])
		if runErr != nil {
			return runErr
		}
		r.printResults(results)
		return nil
	}

	// Statement form. A parser EOF here means "still typing".
	chunks, cerr := r.engine.compile(src)
	if cerr != nil {
		return r.handleParserError(cerr)
	}
	r.Reset()
	return r.engine.runMain(chunks[0])
}

// handleParserError dispatches a parser error to the right next-state.
// Mirrors goby's `handleParserError(*parserErr.Error, *iGb)`.
func (r *REPL) handleParserError(err error) error {
	var perr *parserrors.Error
	if !asParserErr(err, &perr) {
		// Non-parser error (shouldn't happen on the compile path, but be
		// defensive): clear and surface.
		r.Reset()
		return err
	}
	switch {
	case perr.IsEOF():
		// Truncated input — keep the buffer, ask for more.
		r.state = stateWaiting
		return nil
	case perr.IsUnexpectedEmptyLine(len(r.cmds) - 1):
		// Lone `end` with nothing to close (goby's "extra `end`" case).
		// Reset and surface so the user sees the message.
		r.Reset()
		return perr
	default:
		r.Reset()
		return perr
	}
}

// ---------------------------------------------------------------------------
// Run — the read+print half. Picks a readline-driven path for TTYs and a
// scanner path for piped/redirected stdin (preserves scriptable smoke-test
// usage).
// ---------------------------------------------------------------------------

// Run starts the interactive loop. `version` appears in the welcome banner.
// Returns nil on clean exit (Ctrl-D, "exit", "quit", or stdin EOF).
func (r *REPL) Run(version string) error {
	if readline.IsTerminal(int(os.Stdin.Fd())) {
		return r.runReadline(version)
	}
	return r.runScanner(os.Stdin, os.Stdout)
}

// runReadline — full interactive path with history, arrow keys, tab
// completion, and Ctrl-C buffer reset.
func (r *REPL) runReadline(version string) error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            r.switchPrompt(),
		HistoryFile:       filepath.Join(os.TempDir(), "sakura_history"),
		AutoComplete:      newCompleter(),
		InterruptPrompt:   "^C",
		EOFPrompt:         cmdExit,
		HistorySearchFold: true,
	})
	if err != nil {
		return err
	}
	defer rl.Close()

	r.out = colorable.NewColorableStdout()
	r.printBanner(version)

	for {
		rl.SetPrompt(r.switchPrompt())
		line, lerr := rl.Readline()
		switch {
		case errors.Is(lerr, readline.ErrInterrupt):
			// Ctrl-C: drop in-progress input but keep the session.
			r.Reset()
			continue
		case errors.Is(lerr, io.EOF):
			return nil
		case lerr != nil:
			return lerr
		}
		if err := r.Feed(line); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			fmt.Fprintln(r.out, "error:", err)
		}
	}
}

// runScanner — fallback path when stdin isn't a TTY. No colors, no
// history, plain prompts so the output is greppable.
func (r *REPL) runScanner(in io.Reader, out io.Writer) error {
	r.out = out
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	r.writePlainPrompt()
	for sc.Scan() {
		if err := r.Feed(sc.Text()); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			fmt.Fprintln(out, "error:", err)
		}
		r.writePlainPrompt()
	}
	fmt.Fprintln(out)
	return sc.Err()
}

func (r *REPL) writePlainPrompt() {
	if r.state == stateWaiting {
		fmt.Fprint(r.out, ">> ")
	} else {
		fmt.Fprint(r.out, "> ")
	}
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

// printResults writes the chunk's return values, tab-separated, mirroring
// Lua's interactive convention. Empty result lists print nothing.
func (r *REPL) printResults(results []Value) {
	if len(results) == 0 {
		return
	}
	parts := make([]string, len(results))
	for i, v := range results {
		parts[i] = ToString(v)
	}
	fmt.Fprintln(r.out, strings.Join(parts, "\t"))
}

func (r *REPL) printBanner(version string) {
	if version == "" {
		version = "dev"
	}
	fmt.Fprintf(r.out, "sakura %s — Lua 5.4 dialect\n", version)
	fmt.Fprintln(r.out, `Type "help" for usage, "exit" or Ctrl-D to quit.`)
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.out, "Special commands:")
	fmt.Fprintln(r.out, "  =<expr>   evaluate and print expression")
	fmt.Fprintln(r.out, "  reset     clear in-progress multi-line input")
	fmt.Fprintln(r.out, "  help      show this message")
	fmt.Fprintln(r.out, "  exit      leave the REPL (Ctrl-D works too)")
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

func newCompleter() *readline.PrefixCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(luaKeywords))
	for _, k := range luaKeywords {
		items = append(items, readline.PcItem(k))
	}
	return readline.NewPrefixCompleter(items...)
}

// asParserErr unwraps `err` looking for a *parserrors.Error. The compile
// path returns the typed error directly today; the wrapper-walk keeps us
// honest if a future caller sticks an fmt.Errorf("%w", ...) in the path.
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
