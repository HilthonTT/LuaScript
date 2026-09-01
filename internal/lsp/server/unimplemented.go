package server

import (
	"context"

	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

type unimplementedServer struct{}

func (unimplementedServer) Initialize(context.Context, *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	return &protocol.InitializeResult{}, nil
}
func (unimplementedServer) Initialized(context.Context, *protocol.InitializedParams) error {
	return nil
}
func (unimplementedServer) Shutdown(context.Context) error { return nil }
func (unimplementedServer) Exit(context.Context) error     { return nil }
func (unimplementedServer) WorkDoneProgressCancel(context.Context, *protocol.WorkDoneProgressCancelParams) error {
	return nil
}
func (unimplementedServer) LogTrace(context.Context, *protocol.LogTraceParams) error { return nil }
func (unimplementedServer) SetTrace(context.Context, *protocol.SetTraceParams) error { return nil }
func (unimplementedServer) CodeAction(context.Context, *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	return nil, nil
}
func (unimplementedServer) CodeLens(context.Context, *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	return nil, nil
}
func (unimplementedServer) CodeLensResolve(context.Context, *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, nil
}
func (unimplementedServer) ColorPresentation(context.Context, *protocol.ColorPresentationParams) ([]protocol.ColorPresentation, error) {
	return nil, nil
}
func (unimplementedServer) Completion(context.Context, *protocol.CompletionParams) (*protocol.CompletionList, error) {
	return nil, nil
}
func (unimplementedServer) CompletionResolve(context.Context, *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return nil, nil
}
func (unimplementedServer) Declaration(context.Context, *protocol.DeclarationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (unimplementedServer) Definition(context.Context, *protocol.DefinitionParams) ([]protocol.Location, error) {
	return nil, nil
}
func (unimplementedServer) DidChange(context.Context, *protocol.DidChangeTextDocumentParams) error {
	return nil
}
func (unimplementedServer) DidChangeConfiguration(context.Context, *protocol.DidChangeConfigurationParams) error {
	return nil
}
func (unimplementedServer) DidChangeWatchedFiles(context.Context, *protocol.DidChangeWatchedFilesParams) error {
	return nil
}
func (unimplementedServer) DidChangeWorkspaceFolders(context.Context, *protocol.DidChangeWorkspaceFoldersParams) error {
	return nil
}
func (unimplementedServer) DidClose(context.Context, *protocol.DidCloseTextDocumentParams) error {
	return nil
}
func (unimplementedServer) DidOpen(context.Context, *protocol.DidOpenTextDocumentParams) error {
	return nil
}
func (unimplementedServer) DidSave(context.Context, *protocol.DidSaveTextDocumentParams) error {
	return nil
}
func (unimplementedServer) DocumentColor(context.Context, *protocol.DocumentColorParams) ([]protocol.ColorInformation, error) {
	return nil, nil
}
func (unimplementedServer) DocumentHighlight(context.Context, *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	return nil, nil
}
func (unimplementedServer) DocumentLink(context.Context, *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	return nil, nil
}
func (unimplementedServer) DocumentLinkResolve(context.Context, *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	return nil, nil
}
func (unimplementedServer) DocumentSymbol(context.Context, *protocol.DocumentSymbolParams) ([]protocol.SymbolInformationOrDocumentSymbol, error) {
	return nil, nil
}
func (unimplementedServer) ExecuteCommand(context.Context, *protocol.ExecuteCommandParams) (any, error) {
	return nil, nil
}
func (unimplementedServer) FoldingRanges(context.Context, *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	return nil, nil
}
func (unimplementedServer) Formatting(context.Context, *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (unimplementedServer) Hover(context.Context, *protocol.HoverParams) (*protocol.Hover, error) {
	return nil, nil
}
func (unimplementedServer) Implementation(context.Context, *protocol.ImplementationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (unimplementedServer) OnTypeFormatting(context.Context, *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (unimplementedServer) PrepareRename(context.Context, *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return nil, nil
}
func (unimplementedServer) RangeFormatting(context.Context, *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (unimplementedServer) References(context.Context, *protocol.ReferenceParams) ([]protocol.Location, error) {
	return nil, nil
}
func (unimplementedServer) Rename(context.Context, *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (unimplementedServer) SignatureHelp(context.Context, *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	return nil, nil
}
func (unimplementedServer) Symbols(context.Context, *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	return nil, nil
}
func (unimplementedServer) TypeDefinition(context.Context, *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	return nil, nil
}
func (unimplementedServer) WillSave(context.Context, *protocol.WillSaveTextDocumentParams) error {
	return nil
}
func (unimplementedServer) WillSaveWaitUntil(context.Context, *protocol.WillSaveTextDocumentParams) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (unimplementedServer) ShowDocument(context.Context, *protocol.ShowDocumentParams) (*protocol.ShowDocumentResult, error) {
	return nil, nil
}
func (unimplementedServer) WillCreateFiles(context.Context, *protocol.CreateFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (unimplementedServer) DidCreateFiles(context.Context, *protocol.CreateFilesParams) error {
	return nil
}
func (unimplementedServer) WillRenameFiles(context.Context, *protocol.RenameFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (unimplementedServer) DidRenameFiles(context.Context, *protocol.RenameFilesParams) error {
	return nil
}
func (unimplementedServer) WillDeleteFiles(context.Context, *protocol.DeleteFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (unimplementedServer) DidDeleteFiles(context.Context, *protocol.DeleteFilesParams) error {
	return nil
}
func (unimplementedServer) CodeLensRefresh(context.Context) error { return nil }
func (unimplementedServer) PrepareCallHierarchy(context.Context, *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return nil, nil
}
func (unimplementedServer) IncomingCalls(context.Context, *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, nil
}
func (unimplementedServer) OutgoingCalls(context.Context, *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, nil
}
func (unimplementedServer) SemanticTokensFull(context.Context, *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (unimplementedServer) SemanticTokensFullDelta(context.Context, *protocol.SemanticTokensDeltaParams) (any, error) {
	return nil, nil
}
func (unimplementedServer) SemanticTokensRange(context.Context, *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (unimplementedServer) SemanticTokensRefresh(context.Context) error { return nil }
func (unimplementedServer) LinkedEditingRange(context.Context, *protocol.LinkedEditingRangeParams) (*protocol.LinkedEditingRanges, error) {
	return nil, nil
}
func (unimplementedServer) Moniker(context.Context, *protocol.MonikerParams) ([]protocol.Moniker, error) {
	return nil, nil
}
func (unimplementedServer) Request(context.Context, string, any) (any, error) {
	return nil, nil
}
