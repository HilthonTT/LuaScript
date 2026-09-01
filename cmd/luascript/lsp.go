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
			logger.Error("open log file", slog.Any("error", err))
		} else {
			defer f.Close()
			logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
	}

	fmt.Fprintf(os.Stderr, "luascript language server %s: listening on stdio, waiting for client...\n",
		version.GetVersionString())

	if err := server.Run(context.Background(), logger, os.Stdin, os.Stdout); err != nil && err != io.EOF {
		logger.Error("lsp server exited", slog.Any("error", err))
		return 1
	}
	return 0
}
