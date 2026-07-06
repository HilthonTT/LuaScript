package server

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

// documentSymbols parses src and returns the top-level declarations as flat
// SymbolInformation entries: named functions, `local function`s, and `local`
// variable bindings. Nested scopes are intentionally not descended into for
// v1 — the outline of a file is its top-level chunk.
func documentSymbols(uri, src string) []protocol.SymbolInformationOrDocumentSymbol {
	p := parser.New(lexer.New(src))
	program, err := p.ParseProgram()
	if err != nil || program == nil {
		return nil
	}

	var out []protocol.SymbolInformationOrDocumentSymbol
	emit := func(name string, kind protocol.SymbolKind, line int) {
		if name == "" {
			return
		}
		si := &protocol.SymbolInformation{
			Name: name,
			Kind: kind,
			Location: protocol.Location{
				URI:   protocol.DocumentURI(uri),
				Range: wholeLine(src, line),
			},
		}
		out = append(out, protocol.SymbolInformationOrDocumentSymbol{SymbolInformation: si})
	}

	for _, stmt := range program.Block.Statements {
		switch s := stmt.(type) {
		case *ast.LocalFunctionStatement:
			emit(s.Name, protocol.SymbolKindFunction, s.Line())
		case *ast.FunctionDeclaration:
			emit(functionDeclName(s), protocol.SymbolKindFunction, s.Line())
		case *ast.LocalStatement:
			for _, n := range s.Names {
				emit(n.Name, protocol.SymbolKindVariable, s.Line())
			}
		}
	}
	return out
}

// functionDeclName reconstructs the dotted / method name of a function
// declaration (`function a.b:c()` -> "a.b:c") for the symbol outline.
func functionDeclName(fd *ast.FunctionDeclaration) string {
	if fd.Name == nil {
		return ""
	}
	name := fd.Name.Name
	for _, f := range fd.DottedFields {
		name += "." + f
	}
	if fd.MethodName != "" {
		name += ":" + fd.MethodName
	}
	return name
}

// wordAt returns the identifier under the given byte offset in src, along with
// its start/end byte offsets. Used by hover to look up the symbol being
// pointed at. Returns ("", 0, 0) when the cursor is not on an identifier.
func wordAt(src string, offset int) (word string, start, end int) {
	if offset < 0 || offset > len(src) {
		return "", 0, 0
	}
	isIdent := func(b byte) bool {
		return b == '_' ||
			(b >= 'a' && b <= 'z') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9')
	}
	start = offset
	for start > 0 && isIdent(src[start-1]) {
		start--
	}
	end = offset
	for end < len(src) && isIdent(src[end]) {
		end++
	}
	if start == end {
		return "", start, end
	}
	// A leading digit means this is a number, not an identifier.
	if src[start] >= '0' && src[start] <= '9' {
		return "", start, end
	}
	return src[start:end], start, end
}

func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// namespaceBefore returns the qualifier identifier of a `qualifier.` (or
// `qualifier:`) expression whose member starts at src[at]. It walks back over
// an optional `.`/`:` separator immediately preceding `at`, then reads the
// identifier before it. Returns "" when there is no qualified access — e.g. a
// bare word, a numeric prefix, or a chained `a.b.c` where the qualifier is
// itself dotted (we only model single-level namespaces). Used by both dotted
// completion and qualified hover.
func namespaceBefore(src string, at int) string {
	if at <= 0 || at > len(src) {
		return ""
	}
	sep := at - 1
	if src[sep] != '.' && src[sep] != ':' {
		return ""
	}
	end := sep
	begin := end
	for begin > 0 && isIdentByte(src[begin-1]) {
		begin--
	}
	if begin == end || (src[begin] >= '0' && src[begin] <= '9') {
		return ""
	}
	// Reject chained access (a.b.c): if the qualifier is itself preceded by a
	// separator, we don't know the type of the inner field.
	if begin > 0 && (src[begin-1] == '.' || src[begin-1] == ':') {
		return ""
	}
	return src[begin:end]
}
