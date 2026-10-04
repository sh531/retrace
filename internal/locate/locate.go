// Package locate places photos on a GPX track by time.
//
// Camera clocks are wrong in a way that stays the same for a device: one that
// is never adjusted keeps a fixed timezone and drifts. [Offsets] holds a
// correction per camera, set with --offset photo=duration from a photo the
// camera took. [Enricher]
// adds it to each photo's time, then sets the photo's location: EXIF GPS if
// the photo has it, otherwise where the track was at that time, or the
// track's nearest end if the photo was taken before or after it.
//
// [Summarize] reports, per camera, how the photos were timed and located, so
// a wrong offset shows up as photos placed at the track's ends.
package locate

import (
	"context"
	"time"

	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/photo"
)

// Enricher corrects each photo's time with Offsets and locates it on Track.
// It satisfies enrich.Enricher.
type Enricher struct {
	Track   gpx.Track
	Offsets Offsets
}

// Name returns "locate".
func (Enricher) Name() string { return "locate" }

// Enrich adds the offset for p's camera to p.Time. If p has no Location
// from EXIF GPS, it sets one interpolated from the track, or at the track's
// first or last point if p was taken before or after it. A photo with no time
// is left without a location; that isn't an error.
func (e Enricher) Enrich(_ context.Context, p photo.Photo) (photo.Photo, error) {
	if p.Time.IsZero() {
		return p, nil // unknown time: an offset would make it look known
	}
	p.Time = p.Time.Add(e.Offsets.For(p.Camera))

	if p.Location != nil {
		return p, nil // EXIF GPS is more accurate than the track
	}
	if tp, ok := e.Track.PointAt(p.Time); ok {
		p.Location = &photo.Location{Point: tp.Point, Source: photo.SourceInterpolated}
		return p, nil
	}
	if end, ok := e.trackEndFor(p.Time); ok {
		p.Location = &photo.Location{
			Point:            end.Point,
			Source:           photo.SourceTrackEnd,
			TimeFromTrackEnd: p.Time.Sub(end.Time), // negative before the start
		}
	}
	return p, nil
}

// trackEndFor returns the track's first point if t is before it, or its last
// point otherwise. It reports false if the track has no points.
func (e Enricher) trackEndFor(t time.Time) (gpx.TrackPoint, bool) {
	points := e.Track.Points
	if len(points) == 0 {
		return gpx.TrackPoint{}, false
	}
	if t.Before(points[0].Time) {
		return points[0], true
	}
	return points[len(points)-1], true
}
