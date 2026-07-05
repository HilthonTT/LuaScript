// Package server implements the luascript language server: the concrete
// protocol.Server that drives the compiler front-end (lex -> parse ->
// typecheck -> analyze) to serve diagnostics, hover, completion, formatting
// and a document outline to any LSP-speaking editor over stdio.
//
// The name "proxy" is historical — an earlier design proxied to an external
// tool. Today the server answers every request itself; there is nothing to
// proxy to.
package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/hilthontt/luascript/internal/formatter"
	"github.com/hilthontt/luascript/internal/lsp/jsonrpc2"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
	"github.com/hilthontt/luascript/internal/version"
)

// Server is the luascript LSP server. It keeps the set of open documents and
// re-derives everything else (diagnostics, symbols, hover) on demand from the
// current text — there is no persistent AST cache, because a full recompile of
// a single file is cheap.
type Server struct {
	unimplementedServer

	logger *slog.Logger
	client protocol.Client
	docs   *documentStore
}

var _ protocol.Server = (*Server)(nil)

// NewServer builds a server with an empty document store.
func NewServer(logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{logger: logger, docs: newDocumentStore()}
}

// Initialize advertises the capabilities the server actually implements.
func (s *Server) Initialize(ctx context.Context, _ *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	// Stash the client so notification handlers can publish diagnostics even
	// though only requests receive a reply channel.
	if c := protocol.ClientFromContext(ctx); c != nil {
		s.client = c
	}
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
				Save:      &protocol.SaveOptions{IncludeText: true},
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", ":"},
			},
			HoverProvider:              true,
			DocumentFormattingProvider: true,
			DocumentSymbolProvider:     true,
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "luascript-lsp",
			Version: version.GetVersionString(),
		},
	}, nil
}

// Initialized is a no-op today; the client is already captured in Initialize.
func (s *Server) Initialized(context.Context, *protocol.InitializedParams) error {
	s.logger.Debug("initialized")
	return nil
}

// --- Text document synchronisation -----------------------------------------

func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	s.docs.open(uri, params.TextDocument.Text, params.TextDocument.Version)
	s.publishDiagnostics(ctx, uri, params.TextDocument.Version)
	return nil
}

func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}
	// Full-sync mode: the last change event carries the entire new document.
	text := params.ContentChanges[len(params.ContentChanges)-1].Text
	uri := string(params.TextDocument.URI)
	s.docs.update(uri, text, params.TextDocument.Version)
	s.publishDiagnostics(ctx, uri, params.TextDocument.Version)
	return nil
}

func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	if params.Text != "" {
		// The client included the saved text; trust it as the source of truth.
		_, version, _ := s.docs.get(uri)
		s.docs.update(uri, params.Text, version)
	}
	_, version, _ := s.docs.get(uri)
	s.publishDiagnostics(ctx, uri, version)
	return nil
}

func (s *Server) DidClose(_ context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.docs.close(string(params.TextDocument.URI))
	return nil
}

// publishDiagnostics recompiles the document at uri and pushes the results to
// the client. Safe to call with an unknown uri (it simply does nothing) or a
// nil client (before Initialize).
func (s *Server) publishDiagnostics(ctx context.Context, uri string, version int32) {
	if s.client == nil {
		return
	}
	text, _, ok := s.docs.get(uri)
	if !ok {
		return
	}
	diags := computeDiagnostics(text)
	err := s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentURI(uri),
		Version:     uint32(version),
		Diagnostics: diags,
	})
	if err != nil {
		s.logger.Error("publish diagnostics", slog.String("uri", uri), slog.Any("error", err))
	}
}

// --- Language features ------------------------------------------------------

// Hover returns documentation for the builtin identifier under the cursor.
func (s *Server) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	text, _, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	offset := positionToOffset(text, params.Position)
	word, start, end := wordAt(text, offset)
	if word == "" {
		return nil, nil
	}
	doc, ok := hoverDocs[word]
	if !ok {
		return nil, nil
	}
	rng := protocol.Range{
		Start: offsetToPosition(text, start),
		End:   offsetToPosition(text, end),
	}
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: doc},
		Range:    &rng,
	}, nil
}

// Completion returns the static keyword / global / module completion set. It
// ignores context for v1 — the client filters by prefix.
func (s *Server) Completion(_ context.Context, _ *protocol.CompletionParams) (*protocol.CompletionList, error) {
	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        completionItems(),
	}, nil
}

// Formatting reformats the whole document via the shared formatter and returns
// a single full-range replacement edit.
func (s *Server) Formatting(_ context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	text, _, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	opts := formatter.Options{
		Width:  80,
		Indent: 2,
	}
	if params.Options.TabSize > 0 {
		opts.Indent = int(params.Options.TabSize)
	}
	out, err := formatter.Format(text, opts)
	if err != nil {
		// A parse error means we can't format; leave the buffer untouched
		// rather than surfacing an LSP error the user can't act on.
		s.logger.Debug("format failed", slog.Any("error", err))
		return nil, nil
	}
	if out == text {
		return nil, nil
	}
	return []protocol.TextEdit{{
		Range:   fullRange(text),
		NewText: out,
	}}, nil
}

// DocumentSymbol returns the top-level declarations for the outline view.
func (s *Server) DocumentSymbol(_ context.Context, params *protocol.DocumentSymbolParams) ([]protocol.SymbolInformationOrDocumentSymbol, error) {
	uri := string(params.TextDocument.URI)
	text, _, ok := s.docs.get(uri)
	if !ok {
		return nil, nil
	}
	return documentSymbols(uri, text), nil
}

// Shutdown / Exit round out the lifecycle. Exit terminates the process; the
// serve loop's connection is closed by the client immediately afterwards.
func (s *Server) Shutdown(context.Context) error {
	s.logger.Debug("shutdown")
	return nil
}

// fullRange returns a range covering the entire document, used to replace the
// whole buffer on format.
func fullRange(src string) protocol.Range {
	line := uint32(0)
	col := uint32(0)
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: line, Character: col},
	}
}

// stdrwc adapts os.Stdin / os.Stdout into a single io.ReadWriteCloser so the
// jsonrpc2 stream can read requests and write responses over the standard LSP
// stdio transport.
type stdrwc struct {
	in  io.ReadCloser
	out io.WriteCloser
}

func (s stdrwc) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s stdrwc) Write(p []byte) (int, error) { return s.out.Write(p) }
func (s stdrwc) Close() error {
	err := s.in.Close()
	if oerr := s.out.Close(); err == nil {
		err = oerr
	}
	return err
}

// Run serves the language server over the supplied stdio streams until the
// connection closes (client disconnect or exit notification). It blocks.
func Run(ctx context.Context, logger *slog.Logger, in io.ReadCloser, out io.WriteCloser) error {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	srv := NewServer(logger)
	stream := jsonrpc2.NewStream(stdrwc{in: in, out: out})
	ctx, conn, _ := protocol.NewServer(ctx, srv, stream, logger)
	<-conn.Done()
	return conn.Err()
}
