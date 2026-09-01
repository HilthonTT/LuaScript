package testrunner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hilthontt/luascript/internal/native/stdlib/testx"
)

type palette struct {
	reset, red, green, yellow, dim, bold string
}

func newPalette(enabled bool) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		reset:  "\033[0m",
		red:    "\033[31m",
		green:  "\033[32m",
		yellow: "\033[33m",
		dim:    "\033[2m",
		bold:   "\033[1m",
	}
}

func (p palette) wrap(color, s string) string {
	if color == "" {
		return s
	}
	return color + s + p.reset
}

func ColorAuto(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

const nameColumn = 52

type reporter struct {
	out  io.Writer
	opts Options
	c    palette
}

func newReporter(opts Options) *reporter {
	return &reporter{out: opts.Out, opts: opts, c: newPalette(opts.Color)}
}

func (r *reporter) printf(format string, args ...any) {
	fmt.Fprintf(r.out, format, args...)
}

func (r *reporter) file(fr FileResult) {
	if r.opts.Filter != "" && len(fr.Results) == 0 && fr.Err == nil {
		return
	}
	switch {
	case r.opts.List:
		r.listFile(fr)
	case r.opts.Verbose:
		r.verboseFile(fr)
	default:
		r.quietFile(fr)
	}
}

func (r *reporter) listFile(fr FileResult) {
	if fr.Err != nil {
		r.fileError(fr)
		return
	}
	if len(fr.Results) == 0 {
		return
	}
	r.printf("%s\n", r.c.wrap(r.c.bold, fr.Path))
	for _, res := range fr.Results {
		r.printf("  %s\n", res.Name)
	}
}

func (r *reporter) verboseFile(fr FileResult) {
	r.printf("%s\n", r.c.wrap(r.c.bold, fr.Path))
	width := nameWidth(fr.Results)
	for _, res := range fr.Results {
		switch res.Status {
		case testx.StatusPass:
			r.printf("  %s  %-*s  %s\n",
				r.c.wrap(r.c.green, "ok  "), width, res.Name,
				r.c.wrap(r.c.dim, testx.FormatDuration(res.Duration)))
		case testx.StatusSkip:
			r.printf("  %s  %s\n", r.c.wrap(r.c.yellow, "skip"), res.Name)
		case testx.StatusFail:
			r.printf("  %s  %-*s  %s\n",
				r.c.wrap(r.c.red, "FAIL"), width, res.Name,
				r.c.wrap(r.c.dim, testx.FormatDuration(res.Duration)))
			r.failureDetail(res, "        ")
		}
	}
	if fr.Err != nil {
		r.chunkError(fr, "  ")
	}
	r.printf("  %s\n\n", r.c.wrap(r.c.dim, tally(fr)+" in "+testx.FormatDuration(fr.Duration)))
}

func (r *reporter) quietFile(fr FileResult) {
	if fr.Err != nil && len(fr.Results) == 0 {
		r.fileError(fr)
		return
	}
	label, color := "ok  ", r.c.green
	if fr.Err != nil || anyFailed(fr.Results) {
		label, color = "FAIL", r.c.red
	}
	r.printf("%s  %-*s  %s\n", r.c.wrap(color, label), nameColumn, fr.Path,
		r.c.wrap(r.c.dim, tally(fr)+" in "+testx.FormatDuration(fr.Duration)))
	for _, res := range fr.Results {
		if res.Status != testx.StatusFail {
			continue
		}
		r.printf("      %s\n", r.c.wrap(r.c.red, res.Name))
		r.failureDetail(res, "        ")
	}
	if fr.Err != nil {
		r.chunkError(fr, "      ")
	}
}

func (r *reporter) fileError(fr FileResult) {
	r.printf("%s  %s\n", r.c.wrap(r.c.red, "ERR "), fr.Path)
	r.indent(fr.Err.Error(), "        ")
}

func (r *reporter) chunkError(fr FileResult, prefix string) {
	r.printf("%s%s\n", prefix, r.c.wrap(r.c.red, "chunk error"))
	r.indent(fr.Err.Error(), prefix+"  ")
}

func (r *reporter) failureDetail(res testx.Result, prefix string) {
	r.indent(res.Message, prefix)
	if res.Stack != "" && strings.Count(res.Stack, "\n") > 1 {
		r.indent(res.Stack, prefix)
	}
}

func (r *reporter) indent(text, prefix string) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		for strings.HasPrefix(line, "\t") {
			line = "  " + line[1:]
		}
		r.printf("%s%s\n", prefix, line)
	}
}

func (r *reporter) summary(s Summary) {
	if r.opts.List {
		r.printf("\n%d tests\n", s.Passed+s.Failed+s.Skipped)
		return
	}
	parts := []string{
		fmt.Sprintf("%d %s", s.Files, plural(s.Files, "file", "files")),
		fmt.Sprintf("%d passed", s.Passed),
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.Failed))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	if s.FileErrors > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", s.FileErrors, plural(s.FileErrors, "file error", "file errors")))
	}
	r.printf("\n%s in %s\n", strings.Join(parts, ", "), testx.FormatDuration(s.Duration))
	if s.OK() {
		r.printf("%s\n", r.c.wrap(r.c.green, "PASS"))
		return
	}
	r.printf("%s\n", r.c.wrap(r.c.red, "FAIL"))
}

func tally(fr FileResult) string {
	var pass, fail, skip int
	for _, res := range fr.Results {
		switch res.Status {
		case testx.StatusPass:
			pass++
		case testx.StatusFail:
			fail++
		case testx.StatusSkip:
			skip++
		}
	}
	parts := []string{fmt.Sprintf("%d passed", pass)}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", fail))
	}
	if skip > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skip))
	}
	return strings.Join(parts, ", ")
}

func anyFailed(results []testx.Result) bool {
	for _, res := range results {
		if res.Status == testx.StatusFail {
			return true
		}
	}
	return false
}

func nameWidth(results []testx.Result) int {
	w := 0
	for _, res := range results {
		if len(res.Name) > w {
			w = len(res.Name)
		}
	}
	if w > nameColumn {
		return nameColumn
	}
	return w
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
