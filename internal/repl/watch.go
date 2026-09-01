package repl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	watchPollInterval = 200 * time.Millisecond
	watchDebounce     = 150 * time.Millisecond
)

func (r *REPL) WatchFile(filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(
		os.Stderr,
		"\033[2m  watching %s — press Ctrl+C to stop\033[0m\n\n",
		filepath.Base(filePath),
	)

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	var (
		lastMod    time.Time
		pendingMod time.Time
		seenAt     time.Time
		firstRun   = true
	)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\n\033[2m  watch stopped\033[0m")
			return nil

		case <-ticker.C:
			info, err := os.Stat(absPath)
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					fmt.Fprintf(os.Stderr, "watch: %s\n", err)
				}
				continue
			}

			mod := info.ModTime()
			switch {
			case mod.Equal(lastMod):
				continue

			case !mod.Equal(pendingMod):
				pendingMod = mod
				seenAt = time.Now()

			case time.Since(seenAt) >= watchDebounce:
				lastMod = pendingMod
				pendingMod = time.Time{}

				if !firstRun {
					fmt.Printf("\n\033[2m─── %s ───\033[0m\n\n",
						mod.Format("15:04:05"))
				}
				firstRun = false
				r.safeRunFile(absPath)
			}
		}
	}
}

func (r *REPL) safeRunFile(path string) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Fprintf(os.Stderr, "watch: panic during run: %v\n", rec)
		}
	}()
	r.RunFile(path)
}
