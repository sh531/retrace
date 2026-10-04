package gpx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sh531/retrace/internal/geo"
)

// doc wraps body in a GPX 1.1 root element.
func doc(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">` + body + `</gpx>`
}

// track wraps points in a single track segment inside a GPX document.
func track(points string) string {
	return doc("<trk><trkseg>" + points + "</trkseg></trk>")
}

// mustParseTime parses an RFC 3339 time as UTC. It panics on error, so it is
// only for hard-coded test values, where an error means a typo.
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err) // test fixture typo
	}
	return t.UTC()
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []TrackPoint
		wantErr string // substring; empty means success
	}{
		{
			name: "single segment",
			input: track(`
				<trkpt lat="47.1" lon="-121.1"><ele>100</ele><time>2026-09-20T15:00:00Z</time></trkpt>
				<trkpt lat="47.2" lon="-121.2"><ele>110.5</ele><time>2026-09-20T15:00:10Z</time></trkpt>
			`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 47.1, Lon: -121.1}, Time: mustParseTime("2026-09-20T15:00:00Z"), ElevationMeters: new(100.0)},
				{Point: geo.Point{Lat: 47.2, Lon: -121.2}, Time: mustParseTime("2026-09-20T15:00:10Z"), ElevationMeters: new(110.5)},
			},
		},
		{
			name: "segments and tracks are merged in time order",
			input: doc(`
				<trk><trkseg>
					<trkpt lat="3" lon="3"><time>2026-09-20T15:00:30Z</time></trkpt>
				</trkseg><trkseg>
					<trkpt lat="1" lon="1"><time>2026-09-20T15:00:10Z</time></trkpt>
				</trkseg></trk>
				<trk><trkseg>
					<trkpt lat="2" lon="2"><time>2026-09-20T15:00:20Z</time></trkpt>
				</trkseg></trk>`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:10Z")},
				{Point: geo.Point{Lat: 2, Lon: 2}, Time: mustParseTime("2026-09-20T15:00:20Z")},
				{Point: geo.Point{Lat: 3, Lon: 3}, Time: mustParseTime("2026-09-20T15:00:30Z")},
			},
		},
		{
			name: "offset and fractional times are converted to UTC",
			input: track(`
				<trkpt lat="1" lon="1"><time>2026-09-20T08:00:00.5-07:00</time></trkpt>
			`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:00.5Z")},
			},
		},
		{
			name: "elevation: missing is nil; zero and negative are kept",
			input: track(`
				<trkpt lat="1" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>
				<trkpt lat="1" lon="1"><ele>0</ele><time>2026-09-20T15:00:10Z</time></trkpt>
				<trkpt lat="1" lon="1"><ele>-86</ele><time>2026-09-20T15:00:20Z</time></trkpt>
			`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:00Z")},
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:10Z"), ElevationMeters: new(0.0)},
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:20Z"), ElevationMeters: new(-86.0)},
			},
		},
		{
			name: "whitespace around values is ignored",
			input: track(`
				<trkpt lat=" 47.1 " lon=" -121.1 ">
					<ele>
						100
					</ele>
					<time>
						2026-09-20T15:00:00Z
					</time>
				</trkpt>
			`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 47.1, Lon: -121.1}, Time: mustParseTime("2026-09-20T15:00:00Z"), ElevationMeters: new(100.0)},
			},
		},
		{
			name: "coordinate limits are inclusive, including lon 180",
			input: track(`
				<trkpt lat="-90" lon="-180"><time>2026-09-20T15:00:00Z</time></trkpt>
				<trkpt lat="90" lon="180"><time>2026-09-20T15:00:10Z</time></trkpt>
			`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: -90, Lon: -180}, Time: mustParseTime("2026-09-20T15:00:00Z")},
				{Point: geo.Point{Lat: 90, Lon: 180}, Time: mustParseTime("2026-09-20T15:00:10Z")},
			},
		},
		{
			name: "waypoints and routes are ignored",
			input: doc(`
				<wpt lat="9" lon="9"><time>2026-09-20T14:00:00Z</time></wpt>
				<rte><rtept lat="8" lon="8"><time>2026-09-20T14:00:00Z</time></rtept></rte>
				<trk><trkseg>
					<trkpt lat="1" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>
				</trkseg></trk>`),
			want: []TrackPoint{
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:00Z")},
			},
		},
		{
			name: "GPX 1.0 namespace",
			input: `<gpx version="1.0" xmlns="http://www.topografix.com/GPX/1/0"><trk><trkseg>
				<trkpt lat="1" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>
			</trkseg></trk></gpx>`,
			want: []TrackPoint{
				{Point: geo.Point{Lat: 1, Lon: 1}, Time: mustParseTime("2026-09-20T15:00:00Z")},
			},
		},
		{
			name:    "malformed XML inside a track point",
			input:   `<gpx><trk><trkseg><trkpt lat="1" lon="1">`,
			wantErr: "track point 1 (line 1): XML syntax error",
		},
		{
			name:    "malformed XML between elements",
			input:   `<gpx><trk><trkseg>`,
			wantErr: "XML syntax error",
		},
		{
			name:    "malformed metadata",
			input:   doc(`<metadata><name>File name</metadata>`),
			wantErr: "<metadata>: XML syntax error",
		},
		{
			name:    "malformed track name",
			input:   doc(`<trk><name>Track name</trk>`),
			wantErr: "<trk><name>: XML syntax error",
		},
		{
			name:    "missing time",
			input:   track(`<trkpt lat="1" lon="1"/>`),
			wantErr: "track point 1 (line 2): missing <time> (retrace needs a recorded activity, not a planned route)",
		},
		{
			name:    "invalid time",
			input:   track(`<trkpt lat="1" lon="1"><time>yesterday</time></trkpt>`),
			wantErr: `invalid <time> "yesterday"`,
		},
		{
			name:    "missing lat",
			input:   track(`<trkpt lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: "missing lat",
		},
		{
			name:    "lat out of range",
			input:   track(`<trkpt lat="91" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: "lat 91 out of range",
		},
		{
			name:    "lon below range",
			input:   track(`<trkpt lat="1" lon="-180.5"><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: "lon -180.5 out of range [-180, 180]",
		},
		{
			name:    "lat NaN",
			input:   track(`<trkpt lat="NaN" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: "lat NaN out of range",
		},
		{
			name:    "lon not a number",
			input:   track(`<trkpt lat="1" lon="east"><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: `invalid lon "east"`,
		},
		{
			name:    "invalid elevation",
			input:   track(`<trkpt lat="1" lon="1"><ele>high</ele><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: `invalid <ele> "high"`,
		},
		{
			name:    "NaN elevation",
			input:   track(`<trkpt lat="1" lon="1"><ele>NaN</ele><time>2026-09-20T15:00:00Z</time></trkpt>`),
			wantErr: `invalid <ele> "NaN"`,
		},
		{
			name: "error reports which point failed",
			input: track(`
				<trkpt lat="1" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>
				<trkpt lat="1" lon="1"/>
			`),
			wantErr: "track point 2 (line 4): missing <time>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(t.Context(), strings.NewReader(tt.input))

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got.Points); diff != "" {
				t.Errorf("Parse() points mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestParseEqualTimesKeepFileOrder checks that the sort is stable. It needs
// many points: Go sorts short slices with insertion sort, which is stable
// anyway, so a few points would pass even with an unstable sort.
func TestParseEqualTimesKeepFileOrder(t *testing.T) {
	const n = 100
	early, late := mustParseTime("2026-09-20T15:00:00Z"), mustParseTime("2026-09-20T15:00:10Z")

	// Even positions are late and odd positions early; lon records each
	// point's file position.
	times := [2]time.Time{late, early}
	var body strings.Builder
	for i := range n {
		fmt.Fprintf(&body, `<trkpt lat="0" lon="%d"><time>%s</time></trkpt>`, i, times[i%2].Format(time.RFC3339))
	}
	got, err := Parse(t.Context(), strings.NewReader(track(body.String())))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	// Early points first, then late points, each group in file order.
	var want []TrackPoint
	for _, group := range []struct {
		first int
		time  time.Time
	}{{1, early}, {0, late}} {
		for i := group.first; i < n; i += 2 {
			want = append(want, TrackPoint{Point: geo.Point{Lon: float64(i)}, Time: group.time})
		}
	}
	if diff := cmp.Diff(want, got.Points); diff != "" {
		t.Errorf("Parse() points mismatch (-want +got):\n%s", diff)
	}
}

func TestParseDetails(t *testing.T) {
	// One valid point, so the documents under test parse successfully.
	const pt = `<trkpt lat="1" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>`

	tests := []struct {
		name  string
		input string
		want  Track // Points are ignored
	}{
		{
			name: "AllTrails export",
			input: `<gpx version="1.1" creator="AllTrails.com" xmlns="http://www.topografix.com/GPX/1/1">
				<metadata>
					<name><![CDATA[Evening hike at Yellow Aster Butte Trail]]></name>
					<link href="http://www.alltrails.com"><text>AllTrails, LLC</text></link>
				</metadata>
				<trk><name><![CDATA[Evening hike at Yellow Aster Butte Trail]]></name><trkseg>` + pt + `</trkseg></trk>
			</gpx>`,
			want: Track{
				Name:    "Evening hike at Yellow Aster Butte Trail",
				Creator: "AllTrails.com",
			},
		},
		{
			name: "track name preferred over metadata name",
			input: doc(`<metadata><name>File name</name></metadata>
				<trk><name>Track name</name><trkseg>` + pt + `</trkseg></trk>`),
			want: Track{Name: "Track name", Creator: "test"},
		},
		{
			name: "falls back to metadata name",
			input: doc(`<metadata><name>File name</name></metadata>
				<trk><trkseg>` + pt + `</trkseg></trk>`),
			want: Track{Name: "File name", Creator: "test"},
		},
		{
			name: "first track's name wins",
			input: doc(`<trk><name>First</name><trkseg>` + pt + `</trkseg></trk>
				<trk><name>Second</name><trkseg>` + pt + `</trkseg></trk>`),
			want: Track{Name: "First", Creator: "test"},
		},
		{
			name: "route, waypoint, and point names are ignored",
			input: doc(`<wpt lat="1" lon="1"><name>Waypoint</name></wpt>
				<rte><name>Route</name></rte>
				<trk><trkseg><trkpt lat="1" lon="1"><name>Point</name><time>2026-09-20T15:00:00Z</time></trkpt></trkseg></trk>`),
			want: Track{Creator: "test"},
		},
		{
			name:  "no details",
			input: `<gpx><trk><trkseg>` + pt + `</trkseg></trk></gpx>`,
			want:  Track{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(t.Context(), strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreFields(Track{}, "Points")); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		input   string
		wantErr error
	}{
		{name: "empty input", ctx: t.Context(), input: "", wantErr: ErrNoTrackPoints},
		{name: "route only", ctx: t.Context(), input: doc(`<rte><rtept lat="1" lon="1"/></rte>`), wantErr: ErrNoTrackPoints},
		{name: "canceled context", ctx: canceled, input: doc(""), wantErr: context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.ctx, strings.NewReader(tt.input))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Parse() error = %v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	t.Run("sample track", func(t *testing.T) {
		got, err := ParseFile(t.Context(), "testdata/sample.gpx")
		if err != nil {
			t.Fatalf("ParseFile() unexpected error: %v", err)
		}
		want := Track{
			Name:    "Morning hike at Rattlesnake Ledge",
			Creator: "AllTrails.com",
			Points: []TrackPoint{
				{Point: geo.Point{Lat: 47.4337, Lon: -121.7682}, Time: mustParseTime("2026-09-20T15:00:00Z"), ElevationMeters: new(280.5)},
				{Point: geo.Point{Lat: 47.4341, Lon: -121.7690}, Time: mustParseTime("2026-09-20T15:00:10Z"), ElevationMeters: new(301.2)},
				{Point: geo.Point{Lat: 47.4346, Lon: -121.7695}, Time: mustParseTime("2026-09-20T15:00:20Z"), ElevationMeters: new(322.8)},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ParseFile() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := "testdata/does-not-exist.gpx"
		_, err := ParseFile(t.Context(), path)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ParseFile() error = %v, want errors.Is fs.ErrNotExist", err)
		}
		if err != nil && strings.Count(err.Error(), path) != 1 {
			t.Errorf("ParseFile() error = %q, want the path exactly once", err)
		}
	})

	t.Run("parse error names the file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.gpx")
		if err := os.WriteFile(path, []byte(track(`<trkpt lat="91" lon="1"><time>2026-09-20T15:00:00Z</time></trkpt>`)), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ParseFile(t.Context(), path)
		want := "read GPX " + path + ": track point 1 (line 2): lat 91 out of range [-90, 90]"
		if err == nil || err.Error() != want {
			t.Errorf("ParseFile() error = %v, want %q", err, want)
		}
	})
}

func TestPointAt(t *testing.T) {
	at := func(s string) time.Time { return mustParseTime("2026-09-20T15:00:" + s + "Z") }
	tr := Track{Points: []TrackPoint{
		{Point: geo.Point{Lat: 47, Lon: -121}, Time: at("00"), ElevationMeters: new(100.0)},
		{Point: geo.Point{Lat: 48, Lon: -122}, Time: at("10"), ElevationMeters: new(200.0)},
		{Point: geo.Point{Lat: 48, Lon: -122}, Time: at("20")}, // no elevation
		{Point: geo.Point{Lat: 49, Lon: -123}, Time: at("20"), ElevationMeters: new(300.0)},
		{Point: geo.Point{Lat: 50, Lon: -124}, Time: at("30")}, // no elevation
		{Point: geo.Point{Lat: 51, Lon: -125}, Time: at("40"), ElevationMeters: new(500.0)},
	}}

	tests := []struct {
		name   string
		t      time.Time
		want   TrackPoint
		wantOK bool
	}{
		{name: "before the track", t: at("00").Add(-time.Nanosecond)},
		{name: "first point", t: at("00"), want: tr.Points[0], wantOK: true},
		{
			name:   "a quarter of the way between points",
			t:      at("02.5"),
			want:   TrackPoint{Point: geo.Point{Lat: 47.25, Lon: -121.25}, Time: at("02.5"), ElevationMeters: new(125.0)},
			wantOK: true,
		},
		{name: "exact hit", t: at("10"), want: tr.Points[1], wantOK: true},
		{
			name:   "later neighbour without elevation",
			t:      at("15"),
			want:   TrackPoint{Point: geo.Point{Lat: 48, Lon: -122}, Time: at("15")},
			wantOK: true,
		},
		{name: "equal times return the first", t: at("20"), want: tr.Points[2], wantOK: true},
		{
			name:   "after equal times",
			t:      at("25"),
			want:   TrackPoint{Point: geo.Point{Lat: 49.5, Lon: -123.5}, Time: at("25")},
			wantOK: true,
		},
		{
			name:   "earlier neighbour without elevation",
			t:      at("35"),
			want:   TrackPoint{Point: geo.Point{Lat: 50.5, Lon: -124.5}, Time: at("35")},
			wantOK: true,
		},
		{name: "last point", t: at("40"), want: tr.Points[5], wantOK: true},
		{name: "after the track", t: at("40").Add(time.Nanosecond)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tr.PointAt(tt.t)
			if ok != tt.wantOK {
				t.Fatalf("PointAt(%v) ok = %t, want %t", tt.t, ok, tt.wantOK)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateApprox(0, 1e-9)); diff != "" {
				t.Errorf("PointAt(%v) mismatch (-want +got):\n%s", tt.t, diff)
			}
		})
	}
}

func TestPointAtShortTracks(t *testing.T) {
	t0 := mustParseTime("2026-09-20T15:00:00Z")
	one := Track{Points: []TrackPoint{{Point: geo.Point{Lat: 47, Lon: -121}, Time: t0}}}

	if got, ok := one.PointAt(t0); !ok || got != one.Points[0] {
		t.Errorf("single point: PointAt(its time) = %+v, %t; want the point, true", got, ok)
	}
	for _, tt := range []time.Time{t0.Add(-time.Second), t0.Add(time.Second)} {
		if _, ok := one.PointAt(tt); ok {
			t.Errorf("single point: PointAt(%v) ok = true, want false", tt)
		}
	}
	if _, ok := (Track{}).PointAt(t0); ok {
		t.Error("empty track: PointAt() ok = true, want false")
	}
}
