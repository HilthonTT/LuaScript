package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hilthontt/sakura-lang/bonsai"
	"github.com/hilthontt/sakura-lang/formatter"
	"github.com/hilthontt/sakura-lang/native/db"
	osNative "github.com/hilthontt/sakura-lang/native/os"
	"github.com/hilthontt/sakura-lang/repl"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

func main() {
	// Subcommand routing: `sakura fmt ...` is handled before flag parsing so
	// that its flags don't collide with the top-level ones (and so users can
	// run `sakura fmt -w file.sk` without the binary trying to interpret
	// `-w` as an unknown global flag).
	if len(os.Args) >= 2 && os.Args[1] == "fmt" {
		os.Exit(runFmt(os.Args[2:]))
	}

	interactive := flag.Bool("i", false, "start the interactive REPL even if a script is given")
	showVersion := flag.Bool("v", false, "print version and exit")
	growBonsai := flag.Bool("bonsai", false, "grow an ASCII bonsai tree and exit")
	bonsaiSeed := flag.Int64("seed", 0, "rng seed for -bonsai (0 = random)")
	bonsaiPrint := flag.Bool("bonsai-print", false, "with -bonsai: print the tree to stdout instead of staying in the alt-screen")
	bonsaiLive := flag.Bool("bonsai-live", false, "with -bonsai: animate growth step-by-step")
	bonsaiMsg := flag.String("bonsai-msg", "", "with -bonsai: attach a message next to the tree")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.GetVersionString())
		return
	}

	if *growBonsai {
		err := bonsai.Run(bonsai.Options{
			Seed:    *bonsaiSeed,
			Print:   *bonsaiPrint,
			Live:    *bonsaiLive,
			Message: *bonsaiMsg,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "bonsai:", err)
			os.Exit(1)
		}
		return
	}

	v := vm.New()
	r := repl.NewREPL(v, os.Stdin, os.Stdout)
	// Register native modules through the post-init hook so script
	// runs (RunFile creates a fresh non-REPL-mode VM) and `:reset`
	// (rebuilds the REPL VM) both get them.
	r.AddPostInit(db.RegisterDBPreload)
	r.AddPostInit(osNative.RegisterOSPreload)

	args := flag.Args()
	if *interactive || len(args) == 0 {
		r.Start()
		return
	}
	r.RunFile(args[0])
}

// runFmt implements `sakura fmt`. Exit codes: 0 success, 1 I/O or parse
// error, 2 usage error. Mirrors `gofmt`'s flag surface (-w write in place,
// -d diff is out of scope for v1).
//
// Modes:
//
//	sakura fmt file.sakura         -> reformat and print to stdout
//	sakura fmt -w file.sakura      -> reformat in place
//	sakura fmt -                -> read stdin, write stdout
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
