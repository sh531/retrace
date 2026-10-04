package locate

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sh531/retrace/internal/enrich"
	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/photo"
)

// This fails to compile if Enricher stops satisfying enrich.Enricher.
var _ enrich.Enricher = Enricher{}

var (
	sony  = exif.Camera{Make: "SONY", Model: "ILCE-9"}
	apple = exif.Camera{Make: "Apple", Model: "iPhone 13 Pro"}
)

// offsetsFor returns Offsets for the given cameras without reading reference
// photos, for tests of what uses Offsets rather than of Set.
func offsetsFor(byCamera map[exif.Camera]time.Duration) Offsets {
	var o Offsets
	o.byCamera = make(map[exif.Camera]offset)
	for c, d := range byCamera {
		o.byCamera[c] = offset{duration: d, referencePhoto: cameraName(c) + ".jpg"}
	}
	return o
}

func TestEnrich(t *testing.T) {
	at := func(s string) time.Time { return time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC).Add(mustDuration(s)) }
	tr := gpx.Track{Points: []gpx.TrackPoint{
		{Point: geo.Point{Lat: 48, Lon: -121}, Time: at("0s")},
		{Point: geo.Point{Lat: 49, Lon: -122}, Time: at("10m")},
	}}
	exifLoc := &photo.Location{Point: geo.Point{Lat: 1, Lon: 2}, Source: photo.SourceEXIF}
	e := Enricher{Track: tr, Offsets: offsetsFor(map[exif.Camera]time.Duration{sony: 2 * time.Minute, apple: -2 * time.Minute})}

	tests := []struct {
		name string
		p    photo.Photo
		want photo.Photo
	}{
		{
			name: "positive offset, interpolated",
			p:    photo.Photo{Camera: sony, Time: at("3m")},
			want: photo.Photo{Camera: sony, Time: at("5m"), Location: &photo.Location{
				Point: geo.Point{Lat: 48.5, Lon: -121.5}, Source: photo.SourceInterpolated,
			}},
		},
		{
			name: "negative offset, interpolated",
			p:    photo.Photo{Camera: apple, Time: at("4m")},
			want: photo.Photo{Camera: apple, Time: at("2m"), Location: &photo.Location{
				Point: geo.Point{Lat: 48.2, Lon: -121.2}, Source: photo.SourceInterpolated,
			}},
		},
		{
			name: "no offset for camera",
			p:    photo.Photo{Camera: exif.Camera{Make: "Canon"}, Time: at("5m")},
			want: photo.Photo{Camera: exif.Camera{Make: "Canon"}, Time: at("5m"), Location: &photo.Location{
				Point: geo.Point{Lat: 48.5, Lon: -121.5}, Source: photo.SourceInterpolated,
			}},
		},
		{
			name: "EXIF GPS is kept, time still corrected",
			p:    photo.Photo{Camera: sony, Time: at("3m"), Location: exifLoc},
			want: photo.Photo{Camera: sony, Time: at("5m"), Location: exifLoc},
		},
		{
			name: "offset moves the time onto the track",
			p:    photo.Photo{Camera: sony, Time: at("-1m")},
			want: photo.Photo{Camera: sony, Time: at("1m"), Location: &photo.Location{
				Point: geo.Point{Lat: 48.1, Lon: -121.1}, Source: photo.SourceInterpolated,
			}},
		},
		{
			name: "offset moves the time off the track, placed at the last point",
			p:    photo.Photo{Camera: sony, Time: at("9m")},
			want: photo.Photo{Camera: sony, Time: at("11m"), Location: &photo.Location{
				Point: geo.Point{Lat: 49, Lon: -122}, Source: photo.SourceTrackEnd, TimeFromTrackEnd: time.Minute,
			}},
		},
		{
			name: "before the track, placed at the first point",
			p:    photo.Photo{Camera: apple, Time: at("1m")},
			want: photo.Photo{Camera: apple, Time: at("-1m"), Location: &photo.Location{
				Point: geo.Point{Lat: 48, Lon: -121}, Source: photo.SourceTrackEnd, TimeFromTrackEnd: -time.Minute,
			}},
		},
		{
			name: "hours after the track",
			p:    photo.Photo{Camera: exif.Camera{Make: "Canon"}, Time: at("2h14m")},
			want: photo.Photo{Camera: exif.Camera{Make: "Canon"}, Time: at("2h14m"), Location: &photo.Location{
				Point: geo.Point{Lat: 49, Lon: -122}, Source: photo.SourceTrackEnd, TimeFromTrackEnd: 2*time.Hour + 4*time.Minute,
			}},
		},
		{
			name: "unknown time stays zero",
			p:    photo.Photo{Camera: sony},
			want: photo.Photo{Camera: sony},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Enrich(t.Context(), tt.p)
			if err != nil {
				t.Fatalf("Enrich() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateApprox(0, 1e-9)); diff != "" {
				t.Errorf("Enrich() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEnrichAssignsNewLocation checks the enrich.Enricher rule: a Location is
// replaced, never modified in place, so the caller's photo is unchanged.
func TestEnrichAssignsNewLocation(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC)
	tr := gpx.Track{Points: []gpx.TrackPoint{{Point: geo.Point{Lat: 48, Lon: -121}, Time: t0}}}
	stale := &photo.Location{Point: geo.Point{Lat: 1, Lon: 2}, Source: photo.SourceInterpolated}

	got, _ := Enricher{Track: tr}.Enrich(t.Context(), photo.Photo{Time: t0, Location: stale})
	if got.Location != stale {
		t.Fatalf("Enrich() replaced an existing Location; want it kept")
	}
	if *stale != (photo.Location{Point: geo.Point{Lat: 1, Lon: 2}, Source: photo.SourceInterpolated}) {
		t.Errorf("Enrich() modified the caller's Location: %+v", *stale)
	}
}

func TestEnrichEmptyTrack(t *testing.T) {
	p := photo.Photo{Time: time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC)}
	got, err := Enricher{}.Enrich(t.Context(), p)
	if err != nil || got.Location != nil {
		t.Errorf("Enrich() = Location %+v, error %v; want nil, nil", got.Location, err)
	}
}

// mustDuration parses a duration. It panics on error, so it is only for
// hard-coded test values.
func mustDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic(err) // test fixture typo
	}
	return d
}
