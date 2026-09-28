// Command retrace locates hike photos along a GPX track and renders a
// self-contained HTML map with a photo flythrough.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("retrace failed", "err", err)
		os.Exit(1)
	}
}

// run holds the real entrypoint so it can return errors and be tested
// without calling os.Exit. Flag parsing and the pipeline are wired in later.
func run(ctx context.Context, args []string) error {
	_ = ctx
	_ = args
	return nil
}
