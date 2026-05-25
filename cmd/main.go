package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hilthontt/sakura-lang/bonsai"
	"github.com/hilthontt/sakura-lang/formatter"
	"github.com/hilthontt/sakura-lang/repl"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

func main() {
	// If this binary has an embedded script trailer, run it and exit —
	// the bundled .exe should behave as the user's program, not as the
	// sakura CLI. Falls through to the normal CLI when no payload is
	// present (i.e., this is the plain interpreter binary).
	payload, ok, err := readEmbeddedPayload()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		os.Exit(1)
	}
	if ok {
		os.Exit(runBundled(payload))
	}
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	// Subcommand routing happens before flag parsing so that `sakura fmt -w`
	// doesn't collide with the top-level flag set.
	if len(argv) >= 1 && argv[0] == "fmt" {
		return runFmt(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "build" {
		return runBuild(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "analyze" {
		return runAnalyze(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "profile" {
		return runProfile(argv[1:])
	}

	fs := flag.NewFlagSet("sakura", flag.ContinueOnError)
	interactive := fs.Bool("i", false, "start the interactive REPL even if a script is given")
	showVersion := fs.Bool("v", false, "print version and exit")
	growBonsai := fs.Bool("bonsai", false, "grow an ASCII bonsai tree and exit")
	bonsaiSeed := fs.Int64("seed", 0, "rng seed for -bonsai (0 = random)")
	bonsaiPrint := fs.Bool("bonsai-print", false, "with -bonsai: print the tree to stdout instead of staying in the alt-screen")
	bonsaiLive := fs.Bool("bonsai-live", false, "with -bonsai: animate growth step-by-step")
	bonsaiMsg := fs.String("bonsai-msg", "", "with -bonsai: attach a message next to the tree")
	watch := fs.Bool("watch", false, "re-run file on every save")
	timed := fs.Bool("time", false, "print execution time after the program finishes")
	dis := fs.Bool("dis", false, "Disassemble a .sakura file")

	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// Standalone modes that don't touch the VM.
	switch {
	case *showVersion:
		fmt.Println(version.GetVersionString())
		return 0
	case *growBonsai:
		if err := bonsai.Run(bonsai.Options{
			Seed:    *bonsaiSeed,
			Print:   *bonsaiPrint,
			Live:    *bonsaiLive,
			Message: *bonsaiMsg,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "bonsai:", err)
			return 1
		}
		return 0
	}

	if *timed && *watch {
		fmt.Fprintln(os.Stderr, "sakura: --time and --watch are mutually exclusive")
		return 2
	}

	args := fs.Args()
	if (*timed || *watch) && len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sakura: --time and --watch require a script file")
		return 2
	}

	v := vm.New()
	r := repl.NewREPL(v, os.Stdin, os.Stdout)
	// Register native modules via the post-init hook so script runs (a fresh
	// non-REPL-mode VM) and `:reset` (rebuilt REPL VM) both get them. The
	// registrar list lives in cmd/natives.go so the bundled-binary code path
	// in build.go's runBundled can walk the same slice.
	for _, reg := range nativeRegistrars {
		r.AddPostInit(reg)
	}

	// -i with a script argument is rejected rather than silently ignoring
	// the file. "Load script then drop to REPL" would need a new REPL
	// entry point; until then, refuse the combo so users don't think the
	// script ran.
	if *interactive && len(args) > 0 {
		fmt.Fprintln(os.Stderr, "sakura: -i takes no script argument")
		return 2
	}
	// No script, or -i requested: drop into the REPL.
	if *interactive || len(args) == 0 {
		r.Start()
		return 0
	}

	file := args[0]
	switch {
	case *dis:
		r.DisassembleFile(file)
	case *watch:
		if err := r.WatchFile(file); err != nil {
			fmt.Fprintln(os.Stderr, "sakura:", err)
			return 1
		}
	case *timed:
		start := time.Now()
		r.RunFile(file)
		fmt.Fprintf(os.Stdout, "Execution time: %v\n", time.Since(start))
	default:
		r.RunFile(file)
	}
	return 0
}

// runFmt implements `sakura fmt`. Exit codes: 0 success, 1 I/O or parse
// error, 2 usage error. Mirrors gofmt's flag surface (-w write in place;
// -d diff is out of scope for v1).
//
// Modes:
//
//	sakura fmt file.sakura     -> reformat and print to stdout
//	sakura fmt -w file.sakura  -> reformat in place
//	sakura fmt -               -> read stdin, write stdout
//
// On parse error the original source is left untouched and the error is
// reported on stderr.
func runFmt(argv []string) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	write := fs.Bool("w", false, "write the formatted output back to the source file (no-op with stdin)")
	width := fs.Int("width", 80, "target line width")
	indent := fs.Int("indent", 2, "spaces per indent level")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sakura fmt [-w] [-width N] [-indent N] file.sk... | -")
		return 2
	}
	opts := formatter.Options{Width: *width, Indent: *indent}
	exit := 0
	for _, path := range files {
		if path == "-" {
			src, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, "fmt: read stdin:", err)
				return 1
			}
			out, err := formatter.Format(string(src), opts)
			if err != nil {
				fmt.Fprintln(os.Stderr, "fmt: stdin:", err)
				exit = 1
				continue
			}
			io.WriteString(os.Stdout, out)
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fmt:", err)
			exit = 1
			continue
		}
		out, err := formatter.Format(string(src), opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fmt: %s: %v\n", path, err)
			exit = 1
			continue
		}
		if *write {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "fmt: write %s: %v\n", path, err)
				exit = 1
			}
			continue
		}
		io.WriteString(os.Stdout, out)
	}
	return exit
}
