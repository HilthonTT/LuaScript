package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hilthontt/luascript/internal/bonsai"
	"github.com/hilthontt/luascript/internal/formatter"
	"github.com/hilthontt/luascript/internal/gctune"
	"github.com/hilthontt/luascript/internal/repl"
	"github.com/hilthontt/luascript/internal/version"
	"github.com/hilthontt/luascript/internal/vm"
)

func main() {
	payload, ok, err := readEmbeddedPayload()
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript:", err)
		os.Exit(1)
	}
	if ok {
		os.Exit(runBundled(payload))
	}
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) >= 1 && argv[0] == "fmt" {
		return runFmt(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "build" {
		return runBuild(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "analyze" {
		return runAnalyze(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "test" {
		return runTest(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "profile" {
		return runProfile(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "pkg" {
		return runPkg(argv[1:])
	}
	if len(argv) >= 1 && argv[0] == "lsp" {
		return runLSP(argv[1:])
	}
	if len(argv) >= 1 && (argv[0] == "doc" || argv[0] == "man") {
		return runDoc(argv[1:])
	}

	fs := flag.NewFlagSet("luascript", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: luascript [flags] [script.lsc]

With no script, starts the REPL.

Subcommands:
  doc [topic]     stdlib reference (alias: man); "doc -k text" searches
  fmt [-w] file   format a source file
  build -o out    bundle a script and the interpreter into one executable
  test [path...]  run *_test.lsc files; "-run pat" filters, "-v" is verbose
  analyze file    static analysis
  profile file    collect a CPU profile for PGO
  pkg             package manifest / lockfile commands
  lsp             run the language server on stdio

Flags:
`)
		fs.PrintDefaults()
	}
	interactive := fs.Bool("i", false, "start the interactive REPL even if a script is given")
	showVersion := fs.Bool("v", false, "print version and exit")
	growBonsai := fs.Bool("bonsai", false, "grow an ASCII bonsai tree and exit")
	bonsaiSeed := fs.Int64("seed", 0, "rng seed for -bonsai (0 = random)")
	bonsaiPrint := fs.Bool("bonsai-print", false, "with -bonsai: print the tree to stdout instead of staying in the alt-screen")
	bonsaiLive := fs.Bool("bonsai-live", false, "with -bonsai: animate growth step-by-step")
	bonsaiMsg := fs.String("bonsai-msg", "", "with -bonsai: attach a message next to the tree")
	watch := fs.Bool("watch", false, "re-run file on every save")
	timed := fs.Bool("time", false, "print execution time after the program finishes")
	dis := fs.Bool("dis", false, "Disassemble a .lsc file")
	gcPercent := fs.Int("gc-percent", 0, "set GOGC for the run (0 = leave default; negative disables GC)")
	memLimit := fs.Int64("mem-limit", 0, "soft heap memory limit in bytes (0 = unlimited)")

	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

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
		fmt.Fprintln(os.Stderr, "luascript: --time and --watch are mutually exclusive")
		return 2
	}

	args := fs.Args()
	if (*timed || *watch) && len(args) == 0 {
		fmt.Fprintln(os.Stderr, "luascript: --time and --watch require a script file")
		return 2
	}

	gctune.Apply(gctune.Options{Percent: *gcPercent, MemoryLimit: *memLimit})

	v := vm.New()
	r := repl.NewREPL(v, os.Stdin, os.Stdout)
	for _, reg := range nativeRegistrars {
		r.AddPostInit(reg)
	}

	if *interactive && len(args) > 0 {
		fmt.Fprintln(os.Stderr, "luascript: -i takes no script argument")
		return 2
	}
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
			fmt.Fprintln(os.Stderr, "luascript:", err)
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
		fmt.Fprintln(os.Stderr, "usage: luascript fmt [-w] [-width N] [-indent N] file.sk... | -")
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
