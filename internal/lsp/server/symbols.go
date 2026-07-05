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
