package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hilthontt/luascript/internal/testrunner"
)

// runTest implements `luascript test` — discover *_test.lsc files, run them,
// and report.
//
// Exit codes: 0 everything passed, 1 a test or file failed, 2 usage or
// discovery error. That split lets CI treat "no tests found" (2) differently
// from "tests ran and failed" (1).
func runTest(argv []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: luascript test [flags] [path...]

Runs every *`+testrunner.Suffix+` file under the given paths (default ".").
A path naming a file directly is run whether or not it matches that suffix.

Flags:
`)
		fs.PrintDefaults()
	}
	run := fs.String("run", "", "run only tests whose name matches this Lua pattern or substring")
	verbose := fs.Bool("v", false, "report every test, not just failures")
	failfast := fs.Bool("failfast", false, "stop at the first failure")
	list := fs.Bool("list", false, "list the tests that would run without running them")
	noColor := fs.Bool("no-color", false, "disable ANSI colour in the report")
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	opts := testrunner.Options{
		Paths:    fs.Args(),
		Filter:   *run,
		Verbose:  *verbose,
		FailFast: *failfast,
		List:     *list,
		Out:      os.Stdout,
		// nativeRegistrars is package main's single source of truth for
		// bundled modules; the runner takes it as a hook so internal/
		// packages never have to import the native ones.
		RegisterNatives: registerAllNatives,
	}
	opts.Color = !*noColor && testrunner.ColorAuto(opts.Out)

	summary, err := testrunner.Run(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test:", err)
		return 2
	}
	if !summary.OK() {
		return 1
	}
	return 0
}
