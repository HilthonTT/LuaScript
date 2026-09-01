package main

import (
	"flag"
	"fmt"
	"io"

	"os"

	"github.com/hilthontt/luascript/internal/compiler/analyze"
)

func runAnalyze(argv []string) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	maxComplexity := fs.Int("max-complexity", 10,
		"complexity threshold above which a function is flagged")
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	args := fs.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage:.lsc analyze [-max-complexity N] <script.lsc>")
		return 2
	}

	src, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "analyze:", err)
		return 1
	}

	report, err := analyze.Analyze(string(src), analyze.Options{MaxComplexity: *maxComplexity})
	if err != nil {
		fmt.Fprintln(os.Stderr, "analyze: parse:", err)
		return 1
	}

	io.WriteString(os.Stdout, report.String())
	if len(report.Findings) > 0 {
		return 1
	}
	return 0
}
