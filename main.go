package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hilthontt/sakura-lang/native/db"
	osNative "github.com/hilthontt/sakura-lang/native/os"
	"github.com/hilthontt/sakura-lang/repl"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

func main() {
	interactive := flag.Bool("i", false, "start the interactive REPL even if a script is given")
	showVersion := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.GetVersionString())
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
