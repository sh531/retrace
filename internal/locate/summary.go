package locate

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/photo"
)

// Summary is how photos were timed and located, per camera.
type Summary struct {
	Cameras       []CameraSummary // sorted by make, then model
	UnusedOffsets []string        // reference photos of -offset whose camera took none of the photos, sorted
}

// CameraSummary is how the photos from one camera were timed and located.
type CameraSummary struct {
	Camera              exif.Camera     // zero for photos without EXIF
	TotalPhotos         int             // count of all photos from this camera
	RecordedOffsets     []time.Duration // distinct timezones the camera recorded, ascending
	AssumedUTC          int             // count of photos with a time but no recorded timezone
	AppliedOffset       time.Duration   // the -offset added for this camera
	LocatedByEXIF       int             // count of photos located by EXIF GPS
	LocatedByTrack      int             // count of photos interpolated on the track
	LocatedAtTrackEnd   int             // count of photos taken before or after the track, so placed at its nearest end
	MaxTimeFromTrackEnd time.Duration   // the longest TimeFromTrackEnd among LocatedAtTrackEnd photos, ignoring its sign
	Unlocated           int             // no location, since the time is unknown
}

// Summarize groups photos that [Enricher] has processed by camera.
func Summarize(photos []photo.Photo, offsets Offsets) Summary {
	summaries := make(map[exif.Camera]*CameraSummary)
	for _, p := range photos {
		cs, ok := summaries[p.Camera]
		if !ok {
			cs = &CameraSummary{Camera: p.Camera, AppliedOffset: offsets.For(p.Camera)}
			summaries[p.Camera] = cs
		}

		cs.TotalPhotos++
		switch {
		case p.Time.IsZero():
			// no time, so nothing was converted to UTC
		case p.RecordedOffset == nil:
			cs.AssumedUTC++
		default:
			if !slices.Contains(cs.RecordedOffsets, *p.RecordedOffset) {
				cs.RecordedOffsets = append(cs.RecordedOffsets, *p.RecordedOffset)
			}
		}

		switch {
		case p.Location == nil:
			cs.Unlocated++
		case p.Location.Source == photo.SourceEXIF:
			cs.LocatedByEXIF++
		case p.Location.Source == photo.SourceInterpolated:
			cs.LocatedByTrack++
		case p.Location.Source == photo.SourceTrackEnd:
			cs.LocatedAtTrackEnd++
			cs.MaxTimeFromTrackEnd = max(cs.MaxTimeFromTrackEnd, p.Location.TimeFromTrackEnd.Abs())
		}
	}

	var s Summary
	for _, cs := range summaries {
		slices.Sort(cs.RecordedOffsets)
		s.Cameras = append(s.Cameras, *cs)
	}
	slices.SortFunc(s.Cameras, func(a, b CameraSummary) int {
		return cmp.Or(
			strings.Compare(a.Camera.Make, b.Camera.Make),
			strings.Compare(a.Camera.Model, b.Camera.Model),
		)
	})
	for c, entry := range offsets.byCamera {
		if _, ok := summaries[c]; !ok {
			s.UnusedOffsets = append(s.UnusedOffsets, entry.referencePhoto)
		}
	}
	slices.Sort(s.UnusedOffsets)
	return s
}
