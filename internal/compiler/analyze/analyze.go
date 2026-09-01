package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	default:
		return "?"
	}
}

type Finding struct {
	Pass     string
	Rule     string
	Severity Severity
	Message  string
	Line     int
}

type Metrics struct {
	Lines           int
	Functions       int
	MaxComplexity   int
	TotalComplexity int
}

type Report struct {
	Findings []Finding
	Metrics  Metrics
}

func (r *Report) add(f Finding) {
	r.Findings = append(r.Findings, f)
}

type Options struct {
	MaxComplexity int
}

type Pass interface {
	Name() string
	Run(prog *ast.Program, opts Options, rep *Report)
}

var passes = []Pass{
	complexityPass{},
	lintPass{},
	securityPass{},
}

func Analyze(src string, opts Options) (*Report, error) {
	if opts.MaxComplexity <= 0 {
		opts.MaxComplexity = 10
	}

	p := parser.New(lexer.New(src))
	prog, perr := p.ParseProgram()
	if perr != nil {
		return nil, perr
	}

	rep := &Report{}
	rep.Metrics.Lines = strings.Count(src, "\n") + 1
	for _, pass := range passes {
		pass.Run(prog, opts, rep)
	}

	sort.SliceStable(rep.Findings, func(i, j int) bool {
		a, b := rep.Findings[i], rep.Findings[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	return rep, nil
}

func (r *Report) String() string {
	var b strings.Builder
	b.WriteString("analysis report\n\n")

	b.WriteString("metrics:\n")
	fmt.Fprintf(&b, "  lines             %d\n", r.Metrics.Lines)
	fmt.Fprintf(&b, "  functions         %d\n", r.Metrics.Functions)
	fmt.Fprintf(&b, "  total complexity  %d\n", r.Metrics.TotalComplexity)
	fmt.Fprintf(&b, "  max complexity    %d\n", r.Metrics.MaxComplexity)
	b.WriteString("\n")

	if len(r.Findings) == 0 {
		b.WriteString("findings: none\n\nsummary: 0 finding(s)\n")
		return b.String()
	}

	var errs, warns, infos int
	b.WriteString("findings:\n")
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityError:
			errs++
		case SeverityWarning:
			warns++
		default:
			infos++
		}
		fmt.Fprintf(&b, "  %-7s  line %-4d  [%s/%s] %s\n",
			f.Severity, f.Line, f.Pass, f.Rule, f.Message)
	}
	fmt.Fprintf(&b, "\nsummary: %d finding(s) — %d error, %d warning, %d info\n",
		len(r.Findings), errs, warns, infos)
	return b.String()
}
