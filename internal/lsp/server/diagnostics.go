package server

import (
	"github.com/hilthontt/luascript/internal/compiler/analyze"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/compiler/typecheck"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

// computeDiagnostics runs the luascript front-end over src and returns the
// findings as LSP diagnostics. The pipeline mirrors compiler.go:
//
//	lex+parse -> (parse error stops here) -> typecheck -> analyze
//
// A parse error is fatal to the later stages (there is no usable AST), so we
// report just that one and return. When the parse succeeds we layer on type
// errors (respecting the file's --!strict / --!nocheck mode directive) and the
// static-analysis lint/complexity/security findings as warnings and hints.
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

	// Type checking honours the same mode directive the compiler does. The
	// directive is stamped on the lexer as it runs, so it is only valid after
	// ParseProgram has consumed the token stream.
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

	// Static analysis is advisory: surface it as warnings / hints so it never
	// masks a real error. Analyze re-parses internally; if that somehow fails
	// after we just parsed cleanly, drop the findings rather than double-report.
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

// severityFor maps an analyze pass severity onto an LSP diagnostic severity.
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
