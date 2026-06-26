package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hilthontt/luascript/pkgmanager"
)

// runPkg implements `luascript pkg <subcommand>`: the git/URL-based package
// manager. Packages are cloned into ./lua_modules, which `require` already
// searches via package.path.
//
//	luascript pkg add <host/path[@ref]> [name]   fetch + record a dependency
//	luascript pkg install                        fetch everything in the manifest
//	luascript pkg remove <name>                  uninstall + forget a dependency
//
// Exit codes: 0 = success, 1 = operation failed, 2 = usage error.
func runPkg(argv []string) int {
	if len(argv) == 0 {
		return pkgUsage()
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkg:", err)
		return 1
	}
	env := &pkgmanager.Env{
		Root:    wd,
		Fetcher: pkgmanager.GitFetcher{},
		Logf:    func(format string, args ...any) { fmt.Printf(format+"\n", args...) },
	}

	sub, rest := argv[0], argv[1:]
	switch sub {
	case "add":
		fs := flag.NewFlagSet("pkg add", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		args := fs.Args()
		if len(args) < 1 || len(args) > 2 {
			fmt.Fprintln(os.Stderr, "usage: luascript pkg add <host/path[@ref]> [name]")
			return 2
		}
		name := ""
		if len(args) == 2 {
			name = args[1]
		}
		if err := env.Add(args[0], name); err != nil {
			fmt.Fprintln(os.Stderr, "pkg add:", err)
			return 1
		}
		return 0

	case "install", "i":
		if err := env.Install(); err != nil {
			fmt.Fprintln(os.Stderr, "pkg install:", err)
			return 1
		}
		return 0

	case "remove", "rm":
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: luascript pkg remove <name>")
			return 2
		}
		if err := env.Remove(rest[0]); err != nil {
			fmt.Fprintln(os.Stderr, "pkg remove:", err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "pkg: unknown subcommand %q\n", sub)
		return pkgUsage()
	}
}

func pkgUsage() int {
	fmt.Fprintln(os.Stderr, "usage: luascript pkg <add|install|remove> ...")
	fmt.Fprintln(os.Stderr, "  add <host/path[@ref]> [name]   fetch and record a dependency")
	fmt.Fprintln(os.Stderr, "  install                        fetch everything in "+pkgmanager.ManifestName)
	fmt.Fprintln(os.Stderr, "  remove <name>                  uninstall and forget a dependency")
	return 2
}
