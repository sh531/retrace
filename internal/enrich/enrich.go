// Package enrich runs [Enricher] over photos concurrently.
//
// [Run] passes each photo through the enrichers in order, so a later one sees
// what earlier ones added (the location must be known before nearby POIs can
// be found). Photos are processed in parallel, up to a set number at a time.
//
// An enricher that fails doesn't stop the others: its result is discarded, the
// error is recorded in the photo's [Result], and the next enricher gets the
// photo as it was. Only cancelling the context stops a run.
package enrich

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/sh531/retrace/internal/photo"
)

// An Enricher adds what it knows to a photo.
//
// Enrich returns an updated copy of p. On error, the returned photo is ignored
// and the next enricher gets p as it was. The copy is shallow: p's pointer
// fields, such as Location, are shared with the caller's photo, so an
// enricher must assign new values to them, never modify them in place, or a
// change made before an error would still appear in the photo that is kept.
type Enricher interface {
	Name() string // short name used in errors, e.g. "exif"
	Enrich(ctx context.Context, p photo.Photo) (photo.Photo, error)
}

// Result is a photo after enrichment and the errors from enrichers that failed on it.
type Result struct {
	Photo photo.Photo
	Errs  []error // each prefixed with the enricher's name
}

// Run enriches photos with enrichers, processing up to workers photos at a
// time. Results are in the same order as photos. The error is non-nil only if
// ctx is cancelled; enricher errors are recorded in each [Result].
func Run(ctx context.Context, photos []photo.Photo, enrichers []Enricher, workers int) ([]Result, error) {
	if workers < 1 {
		return nil, fmt.Errorf("enrich photos with %d workers: need at least 1", workers)
	}

	// Each goroutine writes only its own element, so results needs no lock.
	results := make([]Result, len(photos))
	var g errgroup.Group // enricher errors don't cancel, so no WithContext
	g.SetLimit(workers)
	for i, p := range photos {
		g.Go(func() error {
			r, err := enrichOne(ctx, p, enrichers)
			results[i] = r
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// Photos returns the photo from each result, in order.
func Photos(results []Result) []photo.Photo {
	photos := make([]photo.Photo, len(results))
	for i, r := range results {
		photos[i] = r.Photo
	}
	return photos
}

// enrichOne passes p through each enricher in order. Its error is non-nil
// only if ctx is cancelled.
func enrichOne(ctx context.Context, p photo.Photo, enrichers []Enricher) (Result, error) {
	r := Result{Photo: p}
	for _, e := range enrichers {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		got, err := e.Enrich(ctx, r.Photo)
		if err != nil {
			r.Errs = append(r.Errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		r.Photo = got
	}
	return r, nil
}
