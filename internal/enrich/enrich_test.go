package enrich

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sh531/retrace/internal/photo"
)

// photo.EXIFEnricher must satisfy Enricher; this fails to compile if it doesn't.
var _ Enricher = photo.EXIFEnricher{}

// stub is an Enricher that calls fn.
type stub struct {
	name string
	fn   func(ctx context.Context, p photo.Photo) (photo.Photo, error)
}

func (s stub) Name() string { return s.name }

func (s stub) Enrich(ctx context.Context, p photo.Photo) (photo.Photo, error) {
	return s.fn(ctx, p)
}

// appendLens returns an enricher that appends s to the photo's Lens, to record
// which enrichers ran and in what order.
func appendLens(s string) Enricher {
	return stub{name: s, fn: func(_ context.Context, p photo.Photo) (photo.Photo, error) {
		p.Lens += s
		return p, nil
	}}
}

// failOn returns an enricher that changes the photo, then fails with err if
// the photo's Path is path.
func failOn(path string, err error) Enricher {
	return stub{name: "fail", fn: func(_ context.Context, p photo.Photo) (photo.Photo, error) {
		p.Lens += "fail"
		if p.Path == path {
			return p, err
		}
		return p, nil
	}}
}

// photosWithPaths returns one photo per path, with only Path set.
func photosWithPaths(paths ...string) []photo.Photo {
	ps := make([]photo.Photo, len(paths))
	for i, path := range paths {
		ps[i] = photo.Photo{Path: path}
	}
	return ps
}

func TestRun(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name      string
		photos    []photo.Photo
		enrichers []Enricher
		want      []Result
	}{
		{
			name:      "no photos",
			enrichers: []Enricher{appendLens("a")},
			want:      []Result{},
		},
		{
			name:   "no enrichers",
			photos: photosWithPaths("1.jpg", "2.jpg"),
			want:   []Result{{Photo: photo.Photo{Path: "1.jpg"}}, {Photo: photo.Photo{Path: "2.jpg"}}},
		},
		{
			name:      "enrichers run in order",
			photos:    photosWithPaths("1.jpg", "2.jpg"),
			enrichers: []Enricher{appendLens("a"), appendLens("b"), appendLens("c")},
			want: []Result{
				{Photo: photo.Photo{Path: "1.jpg", Lens: "abc"}},
				{Photo: photo.Photo{Path: "2.jpg", Lens: "abc"}},
			},
		},
		{
			name:      "failed enricher is discarded and the rest still run",
			photos:    photosWithPaths("1.jpg", "2.jpg", "3.jpg"),
			enrichers: []Enricher{appendLens("a"), failOn("2.jpg", errBoom), appendLens("b")},
			want: []Result{
				{Photo: photo.Photo{Path: "1.jpg", Lens: "afailb"}},
				{Photo: photo.Photo{Path: "2.jpg", Lens: "ab"}, Errs: []error{errBoom}},
				{Photo: photo.Photo{Path: "3.jpg", Lens: "afailb"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Run(t.Context(), tt.photos, tt.enrichers, 2)
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("Run() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRunErrorNamesEnricher(t *testing.T) {
	got, err := Run(t.Context(), photosWithPaths("1.jpg"), []Enricher{failOn("1.jpg", errors.New("boom"))}, 1)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(got) != 1 || len(got[0].Errs) != 1 {
		t.Fatalf("Run() = %v, want 1 result with 1 error", got)
	}
	if msg, want := got[0].Errs[0].Error(), "fail: boom"; msg != want {
		t.Errorf("Run() error = %q, want %q", msg, want)
	}
}

func TestRunLimitsWorkers(t *testing.T) {
	const workers = 3

	// synctest runs the test in a bubble with a fake clock: time.Sleep returns
	// as soon as every goroutine in the bubble is blocked, so all workers that
	// the limit allows are running at once, without real waiting.
	synctest.Test(t, func(t *testing.T) {
		var (
			mu           sync.Mutex
			active, peak int
		)
		track := stub{name: "track", fn: func(_ context.Context, p photo.Photo) (photo.Photo, error) {
			mu.Lock()
			active++
			peak = max(peak, active)
			mu.Unlock()

			time.Sleep(time.Second)

			mu.Lock()
			active--
			mu.Unlock()
			return p, nil
		}}

		ps := make([]photo.Photo, 10)
		for i := range ps {
			ps[i].Path = fmt.Sprintf("%d.jpg", i)
		}
		got, err := Run(t.Context(), ps, []Enricher{track}, workers)
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
		if len(got) != len(ps) {
			t.Errorf("Run() returned %d results, want %d", len(got), len(ps))
		}
		if peak != workers {
			t.Errorf("peak concurrent enrichers = %d, want %d", peak, workers)
		}
	})
}

func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var calls []string // one worker, so no lock needed
	cancelOnFirst := stub{name: "cancel", fn: func(_ context.Context, p photo.Photo) (photo.Photo, error) {
		calls = append(calls, p.Path)
		cancel()
		return p, nil
	}}
	record := stub{name: "record", fn: func(_ context.Context, p photo.Photo) (photo.Photo, error) {
		calls = append(calls, "record "+p.Path)
		return p, nil
	}}

	got, err := Run(ctx, photosWithPaths("1.jpg", "2.jpg", "3.jpg"), []Enricher{cancelOnFirst, record}, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want errors.Is context.Canceled", err)
	}
	if got != nil {
		t.Errorf("Run() results = %v, want nil", got)
	}
	if want := []string{"1.jpg"}; !cmp.Equal(want, calls) {
		t.Errorf("enrichers called for %v, want only %v", calls, want)
	}
}

func TestRunInvalidWorkers(t *testing.T) {
	for _, workers := range []int{0, -1} {
		_, err := Run(t.Context(), photosWithPaths("1.jpg"), nil, workers)
		if err == nil || !strings.Contains(err.Error(), "at least 1") {
			t.Errorf("Run(workers = %d) error = %v, want one saying at least 1", workers, err)
		}
	}
}
