// Command retrace places hike photos on the GPX track recorded during the
// hike and builds a static web page from them: an interactive map with each
// photo pinned where it was taken, and a flythrough that follows the trail.
//
// Photos with EXIF GPS are placed directly; the rest are placed by time,
// using where the track was when each photo was taken. The CLI is still
// being built; see the README for usage and status.
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
