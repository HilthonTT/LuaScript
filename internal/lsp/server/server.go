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

type Server struct {
	unimplementedServer

	logger *slog.Logger
	client protocol.Client
	docs   *documentStore
}

var _ protocol.Server = (*Server)(nil)

func NewServer(logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{logger: logger, docs: newDocumentStore()}
}

func (s *Server) Initialize(ctx context.Context, _ *protocol.InitializeParams) (*protocol.InitializeResult, error) {
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

func (s *Server) Initialized(context.Context, *protocol.InitializedParams) error {
	s.logger.Debug("initialized")
	return nil
}

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
	text := params.ContentChanges[len(params.ContentChanges)-1].Text
	uri := string(params.TextDocument.URI)
	s.docs.update(uri, text, params.TextDocument.Version)
	s.publishDiagnostics(ctx, uri, params.TextDocument.Version)
	return nil
}

func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	if params.Text != "" {
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
	if ns := namespaceBefore(text, start); ns != "" {
		if qdoc, qok := hoverDocs[ns+"."+word]; qok {
			doc, ok = qdoc, true
		}
	}
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

func (s *Server) Completion(_ context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	if text, _, ok := s.docs.get(string(params.TextDocument.URI)); ok {
		offset := positionToOffset(text, params.Position)
		wordStart := offset
		for wordStart > 0 && isIdentByte(text[wordStart-1]) {
			wordStart--
		}
		if ns := namespaceBefore(text, wordStart); ns != "" {
			if items := memberCompletionItems(ns); items != nil {
				return &protocol.CompletionList{IsIncomplete: false, Items: items}, nil
			}
		}
	}
	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        completionItems(),
	}, nil
}

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

func (s *Server) DocumentSymbol(_ context.Context, params *protocol.DocumentSymbolParams) ([]protocol.SymbolInformationOrDocumentSymbol, error) {
	uri := string(params.TextDocument.URI)
	text, _, ok := s.docs.get(uri)
	if !ok {
		return nil, nil
	}
	return documentSymbols(uri, text), nil
}

func (s *Server) Shutdown(context.Context) error {
	s.logger.Debug("shutdown")
	return nil
}

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
