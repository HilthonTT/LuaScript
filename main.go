package main

import (
	"flag"
	"fmt"
	"os"

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

	args := flag.Args()
	if *interactive || len(args) == 0 {
		r.Start()
		return
	}
	r.RunFile(args[0])
}
