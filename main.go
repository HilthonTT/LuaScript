// sakura — entry point. With `-i` or no script argument the binary drops
// into the interactive REPL; with a path argument it compiles and runs
// the given source file as the main chunk.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/parser"
	"github.com/hilthontt/sakura-lang/vm"
)

// version is stamped into the REPL banner. Override at build time with
// `-ldflags "-X main.version=..."` once a release process exists.
const version = "dev"

func main() {
	interactive := flag.Bool("i", false, "start the interactive REPL even if a script is given")
	showVersion := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("sakura", version)
		return
	}

	args := flag.Args()
	if *interactive || len(args) == 0 {
		runREPL()
		return
	}
	runFile(args[0])
}

func runREPL() {
	r := vm.NewREPL(vm.New())
	if err := r.Run(version); err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		os.Exit(1)
	}
}

func runFile(path string) {
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
