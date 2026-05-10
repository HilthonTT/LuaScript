package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hilthontt/sakura-lang/bonsai"
	"github.com/hilthontt/sakura-lang/native/db"
	osNative "github.com/hilthontt/sakura-lang/native/os"
	"github.com/hilthontt/sakura-lang/repl"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

func main() {
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
