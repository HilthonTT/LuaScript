package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hilthontt/luascript/internal/lsp/server"
	"github.com/hilthontt/luascript/internal/version"
)

// runLSP implements `luascript lsp`: it starts the language server and speaks
// LSP over stdio (the transport every editor uses to launch a server). It
// blocks until the client disconnects or sends `exit`.
//
// Debug logs go to a file, never stdout — the stdio channel is reserved for
// the JSON-RPC protocol, and interleaving text would corrupt it. A one-line
// startup banner is written to stderr (safe: editors show it in their LSP
// output panel), so there is a visible "it's running" signal.
//
//	luascript lsp                 # serve, logging disabled
//	luascript lsp -log lsp.log    # serve, debug logs to lsp.log
func runLSP(argv []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	logPath := fs.String("log", "", "write debug logs to this file (default: no logging)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			// Fall back to discarding logs rather than failing to start — a
			// missing log file should never take the editor's language
			// features down with it.
			logger.Error("open log file", slog.Any("error", err))
		} else {
			defer f.Close()
			logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
	}

	// Announce startup on stderr — never stdout, which is the JSON-RPC
	// channel. Editors surface stderr in their LSP output panel, and a human
	// running this in a terminal sees the banner and knows it's alive (and
	// waiting on stdin) rather than hung.
	fmt.Fprintf(os.Stderr, "luascript language server %s: listening on stdio, waiting for client...\n",
		version.GetVersionString())

	if err := server.Run(context.Background(), logger, os.Stdin, os.Stdout); err != nil && err != io.EOF {
		logger.Error("lsp server exited", slog.Any("error", err))
		return 1
	}
	return 0
}
