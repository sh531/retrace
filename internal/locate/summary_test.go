package locate

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/photo"
)

func TestSummarize(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC)
	pdt, pst := -7*time.Hour, -8*time.Hour
	interp := &photo.Location{Source: photo.SourceInterpolated}
	gps := &photo.Location{Source: photo.SourceEXIF}
	end := func(gap time.Duration) *photo.Location {
		return &photo.Location{Source: photo.SourceTrackEnd, TimeFromTrackEnd: gap}
	}

	photos := []photo.Photo{
		{Camera: sony, Time: t0, RecordedOffset: &pdt, Location: interp},
		{Camera: sony, Time: t0, RecordedOffset: &pdt, Location: interp},
		{Camera: sony, Time: t0, RecordedOffset: &pst, Location: end(2 * time.Hour)},
		{Camera: sony, Time: t0, RecordedOffset: &pdt, Location: end(-9 * time.Hour)}, // before the start
		{Camera: sony, Time: t0, RecordedOffset: &pdt, Location: end(time.Minute)},
		{Camera: apple, Time: t0, RecordedOffset: &pdt, Location: gps},
		{Camera: apple, Time: t0, Location: end(time.Second)}, // no timezone: assumed UTC
		{Path: "no-exif.jpg"}, // malformed or missing EXIF
	}
	offsets := offsetsFor(map[exif.Camera]time.Duration{
		sony:  time.Minute,
		apple: 0,
		// No photos from these; enough of them that an unsorted result
		// (map order is random) is almost never in order by chance.
		{Make: "SONY", Model: "ILCE-1"}:   time.Second,
		{Make: "Canon", Model: "EOS R5"}:  time.Second,
		{Make: "FUJIFILM", Model: "X-T5"}: time.Second,
		{Make: "NIKON", Model: "Z 8"}:     time.Second,
		{Make: "Apple", Model: "iPad"}:    time.Second,
	})

	want := Summary{
		Cameras: []CameraSummary{
			{TotalPhotos: 1, Unlocated: 1},
			{Camera: apple, TotalPhotos: 2, RecordedOffsets: []time.Duration{pdt}, AssumedUTC: 1, LocatedByEXIF: 1, LocatedAtTrackEnd: 1, MaxTimeFromTrackEnd: time.Second},
			{Camera: sony, TotalPhotos: 5, RecordedOffsets: []time.Duration{pst, pdt}, AppliedOffset: time.Minute, LocatedByTrack: 2, LocatedAtTrackEnd: 3, MaxTimeFromTrackEnd: 9 * time.Hour},
		},
		UnusedOffsets: []string{"Apple iPad.jpg", "Canon EOS R5.jpg", "FUJIFILM X-T5.jpg", "NIKON Z 8.jpg", "SONY ILCE-1.jpg"},
	}
	got := Summarize(photos, offsets)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Summarize() mismatch (-want +got):\n%s", diff)
	}
}
