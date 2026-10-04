package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sh531/retrace/internal/enrich"
	"github.com/sh531/retrace/internal/locate"
)

// report logs a warning per photo that a step failed on or that couldn't be located,
// then a record per camera, then a warning per unused -offset.
func report(log *slog.Logger, results []enrich.Result, s locate.Summary) {
	for _, r := range results {
		for _, failure := range r.Errs {
			log.Warn("photo kept, but a step failed", "photo", r.Photo.Path, "err", failure)
		}
		// exif doesn't treat a missing date as an error, so say why it's unlocated.
		if r.Photo.Location == nil && len(r.Errs) == 0 {
			log.Warn("no capture time or GPS in its EXIF, so it can't be located", "photo", r.Photo.Path)
		}
	}
	for _, cs := range s.Cameras {
		log.Info("camera", cameraAttrs(cs)...)
	}
	for _, path := range s.UnusedOffsets {
		log.Warn("-offset photo is from a camera that took none of the photos", "photo", path)
	}
}

// cameraAttrs returns how one camera's photos were timed and located, as log
// attributes named after the [locate.CameraSummary] fields. Zeros are left
// out, so a line shows only what happened.
func cameraAttrs(cs locate.CameraSummary) []any {
	attrs := []any{
		"camera", cmp.Or(cs.Camera.String(), "none in EXIF"),
		"total_photos", cs.TotalPhotos,
	}
	attrs = appendNonZero(attrs, "recorded_offsets", formatZones(cs.RecordedOffsets))
	attrs = appendNonZero(attrs, "assumed_utc", cs.AssumedUTC)
	attrs = appendNonZero(attrs, "applied_offset", cs.AppliedOffset)
	attrs = appendNonZero(attrs, "located_by_exif", cs.LocatedByEXIF)
	attrs = appendNonZero(attrs, "located_by_track", cs.LocatedByTrack)
	attrs = appendNonZero(attrs, "located_at_track_end", cs.LocatedAtTrackEnd)
	attrs = appendNonZero(attrs, "max_time_from_track_end", cs.MaxTimeFromTrackEnd.Round(time.Second))
	attrs = appendNonZero(attrs, "unlocated", cs.Unlocated)
	return attrs
}

// appendNonZero appends key and value to attrs, unless value is zero.
func appendNonZero[T comparable](attrs []any, key string, value T) []any {
	var zero T
	if value == zero {
		return attrs
	}
	return append(attrs, key, value)
}

// formatZones returns timezone offsets as EXIF writes them, comma-separated,
// e.g. "-07:00,+05:30"; "" if there are none.
func formatZones(offsets []time.Duration) string {
	zones := make([]string, len(offsets))
	for i, d := range offsets {
		zones[i] = formatZone(d)
	}
	return strings.Join(zones, ",")
}

// formatZone returns a timezone offset as EXIF writes it, e.g. "-07:00".
func formatZone(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	return fmt.Sprintf("%s%02d:%02d", sign, int(d.Hours()), int(d.Minutes())%60)
}
