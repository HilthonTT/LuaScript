package server

import (
	"github.com/hilthontt/luascript/internal/compiler/analyze"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/compiler/typecheck"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

func computeDiagnostics(src string) []protocol.Diagnostic {
	diags := []protocol.Diagnostic{}

	l := lexer.New(src)
	p := parser.New(l)
	program, perr := p.ParseProgram()
	if perr != nil {
		line, col := extractLineCol(perr.Error())
		var rng protocol.Range
		if col > 0 {
			rng = spanFrom(src, line, col)
		} else {
			rng = wholeLine(src, line)
		}
		diags = append(diags, protocol.Diagnostic{
			Range:    rng,
			Severity: protocol.DiagnosticSeverityError,
			Source:   "luascript",
			Code:     "syntax",
			Message:  perr.Error(),
		})
		return diags
	}

	if l.ModeDirective != "nocheck" {
		opts := typecheck.Options{Strict: l.ModeDirective == "strict"}
		for _, te := range typecheck.Check(program, opts) {
			diags = append(diags, protocol.Diagnostic{
				Range:    wholeLine(src, te.Line),
				Severity: protocol.DiagnosticSeverityError,
				Source:   "luascript-types",
				Code:     te.Code,
				Message:  te.Format(),
			})
		}
	}

	if rep, aerr := analyze.Analyze(src, analyze.Options{}); aerr == nil {
		for _, f := range rep.Findings {
			diags = append(diags, protocol.Diagnostic{
				Range:    wholeLine(src, f.Line),
				Severity: severityFor(f.Severity),
				Source:   "luascript-" + f.Pass,
				Code:     f.Rule,
				Message:  f.Message,
			})
		}
	}

	return diags
}

func severityFor(s analyze.Severity) protocol.DiagnosticSeverity {
	switch s {
	case analyze.SeverityError:
		return protocol.DiagnosticSeverityError
	case analyze.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityInformation
	}
}
