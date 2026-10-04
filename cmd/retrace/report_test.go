package main

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/enrich"
	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/locate"
	"github.com/sh531/retrace/internal/photo"
)

func TestReport(t *testing.T) {
	results := []enrich.Result{
		{Photo: photo.Photo{ // nothing to warn about
			Path:     "a.jpg",
			Time:     time.Date(2025, 8, 3, 3, 40, 0, 0, time.UTC),
			Location: &photo.Location{Source: photo.SourceInterpolated},
		}},
		{Photo: photo.Photo{Path: "b.jpg"}, Errs: []error{errors.New("exif: truncated"), errors.New("locate: oops")}},
		{Photo: photo.Photo{Path: "c.jpg"}}, // no time and no error
		{Photo: photo.Photo{Path: "d.jpg", Location: &photo.Location{Source: photo.SourceEXIF}}}, // GPS but no time: placed, so no warning
	}
	summary := locate.Summary{
		Cameras: []locate.CameraSummary{
			{TotalPhotos: 4, LocatedByEXIF: 1, LocatedByTrack: 1, Unlocated: 2}, // no camera in EXIF
			{
				Camera:              exif.Camera{Make: "SONY", Model: "ILCE-9"},
				TotalPhotos:         10,
				RecordedOffsets:     []time.Duration{-7 * time.Hour, 0, 5*time.Hour + 30*time.Minute},
				AssumedUTC:          2,
				AppliedOffset:       -90 * time.Second,
				LocatedByEXIF:       1,
				LocatedByTrack:      3,
				LocatedAtTrackEnd:   4,
				MaxTimeFromTrackEnd: 18*time.Minute + 28*time.Second + 400*time.Millisecond,
				Unlocated:           2,
			},
		},
		UnusedOffsets: []string{"other.jpg"},
	}

	var got bytes.Buffer
	report(newLogger(&got), results, summary)

	// b.jpg's failures already explain why it has no time, so it gets no second warning.
	want := `level=WARN msg="photo kept, but a step failed" photo=b.jpg err="exif: truncated"
level=WARN msg="photo kept, but a step failed" photo=b.jpg err="locate: oops"
level=WARN msg="no capture time or GPS in its EXIF, so it can't be located" photo=c.jpg
level=INFO msg=camera camera="none in EXIF" total_photos=4 located_by_exif=1 located_by_track=1 unlocated=2
level=INFO msg=camera camera="SONY ILCE-9" total_photos=10 recorded_offsets=-07:00,+00:00,+05:30 assumed_utc=2 applied_offset=-1m30s located_by_exif=1 located_by_track=3 located_at_track_end=4 max_time_from_track_end=18m28s unlocated=2
level=WARN msg="-offset photo is from a camera that took none of the photos" photo=other.jpg
`
	if diff := cmp.Diff(want, got.String()); diff != "" {
		t.Errorf("report mismatch (-want +got):\n%s", diff)
	}
}
