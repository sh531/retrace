// Command retrace places hike photos on the GPX track recorded during the
// hike and builds a static web page from them: an interactive map with each
// photo pinned where it was taken, and a flythrough that follows the trail.
//
// Photos with EXIF GPS are placed directly; the rest are placed by time,
// using where the track was when each photo was taken. The flythrough is
// still being built; see the README for usage and status.
//
// Usage:
//
//	retrace -photos dir -gpx file [-offset photo=duration]... [-output dir]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/sh531/retrace/internal/enrich"
	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/locate"
	"github.com/sh531/retrace/internal/photo"
	"github.com/sh531/retrace/internal/site"
)

// Exit codes, following the flag package: 2 for a bad command line.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Stderr) // os.Args[1:] drops the program name
	stop()                                  // os.Exit skips deferred calls

	code := exitCode(err)
	if code == exitError { // flag and parseArgs already printed usage errors
		slog.Error("retrace failed", "err", err)
	}
	os.Exit(code)
}

// usageError is a mistake in the command line.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// exitCode returns the exit code for an error returned by run.
func exitCode(err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if errors.As(err, new(usageError)) {
		return exitUsage
	}
	return exitError
}

// config is the parsed command line.
type config struct {
	photosDir string
	gpxPath   string
	offsets   locate.Offsets
	outputDir string
}

// run holds the real entrypoint so it can return errors and be tested
// without calling os.Exit. Usage, warnings, and progress go to stderr.
func run(ctx context.Context, args []string, stderr io.Writer) error {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}
	// Fail at once if -output already exists and isn't empty, before any work.
	// site.Write creates the directory at the end, so a run that fails earlier
	// leaves no empty directory behind.
	if err = site.CheckDir(cfg.outputDir); err != nil {
		return err
	}
	log := newLogger(stderr)

	track, err := gpx.ParseFile(ctx, cfg.gpxPath)
	if err != nil {
		return err
	}
	paths, err := photo.Find(cfg.photosDir)
	if err != nil {
		return err
	}

	photos := make([]photo.Photo, len(paths))
	for i, path := range paths {
		photos[i] = photo.Photo{Path: path}
	}
	enrichers := []enrich.Enricher{
		photo.EXIFEnricher{},
		locate.Enricher{Track: track, Offsets: cfg.offsets},
	}
	results, err := enrich.Run(ctx, photos, enrichers, runtime.GOMAXPROCS(0))
	if err != nil {
		return err
	}

	enriched := enrich.Photos(results)
	report(log, results, locate.Summarize(enriched, cfg.offsets))

	data := site.New(track, enriched)
	skipped, err := site.Write(cfg.outputDir, cfg.photosDir, data)
	if err != nil {
		return err
	}
	for _, failure := range skipped {
		log.Warn("photo left off the page", "err", failure)
	}
	log.Info("done", "photos", len(data.Photos)-len(skipped), "output", cfg.outputDir)
	return nil
}

// parseArgs parses the command line. Every mistake it finds is reported at
// once, followed by the usage.
func parseArgs(args []string, stderr io.Writer) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("retrace", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.photosDir, "photos", "", "`dir` of JPEG photos from the hike (required)")
	fs.StringVar(&cfg.gpxPath, "gpx", "", "GPX `file` recorded during the hike (required)")
	fs.Var(&cfg.offsets, "offset", "correct the clock of the camera that took photo by duration, given as `photo=duration`; repeatable, once per camera; e.g. DSC00042.jpg=2m30s")
	fs.StringVar(&cfg.outputDir, "output", "retrace-out", "`dir` to write the page to; must not exist yet or be empty")
	fs.Usage = func() {
		// Like flag itself, ignore write errors: there's nowhere left to report them.
		_, _ = fmt.Fprintln(fs.Output(), "Usage: retrace -photos dir -gpx file [flags]")
		fs.PrintDefaults()
	}

	// flag prints its own errors and the usage.
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, err
		}
		return config{}, usageError{err}
	}

	var problems []string
	if fs.NArg() > 0 {
		// flag stops at the first argument that isn't a flag, often an unquoted path with a space.
		problems = append(problems, fmt.Sprintf("unexpected argument %q (retrace takes only flags; quote paths that contain spaces)", fs.Arg(0)))
	}
	var missing []string
	if cfg.photosDir == "" {
		missing = append(missing, "-photos")
	}
	if cfg.gpxPath == "" {
		missing = append(missing, "-gpx")
	}
	if len(missing) > 0 {
		problems = append(problems, "missing required "+strings.Join(missing, " and "))
	}
	if sameDir(cfg.photosDir, cfg.outputDir) {
		problems = append(problems, "-output can't be the -photos directory")
	}
	if len(problems) > 0 {
		err := errors.New(strings.Join(problems, "\n"))
		_, _ = fmt.Fprintln(stderr, err)
		fs.Usage()
		return config{}, usageError{err}
	}
	return cfg, nil
}

// sameDir reports whether a and b are the same existing directory, however
// they are spelled.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aInfo, err := os.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}

// newLogger returns a logger that writes text lines to w without the time,
// which a CLI's reader doesn't need and which would make output differ per run.
func newLogger(w io.Writer) *slog.Logger {
	dropTime := func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{ReplaceAttr: dropTime}))
}
