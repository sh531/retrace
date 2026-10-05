package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/site"
)

// exifTestdata holds fixture JPEGs written by exiftool; see its gen.sh.
const exifTestdata = "../../internal/exif/testdata"

// trackGPX covers 03:30–03:40 UTC on 2025-08-03, when the sony.jpg and
// apple.jpg fixtures were taken (20:37:16 and 20:32:09 at -07:00 the day before).
const trackGPX = `<?xml version="1.0"?>
<gpx version="1.1" creator="test">
  <trk><name>Test hike</name><trkseg>
    <trkpt lat="47.0" lon="-121.0"><ele>1000</ele><time>2025-08-03T03:30:00Z</time></trkpt>
    <trkpt lat="47.1" lon="-121.1"><ele>1100</ele><time>2025-08-03T03:40:00Z</time></trkpt>
  </trkseg></trk>
</gpx>`

// hike is a photos directory and a GPX file in a temporary directory.
type hike struct {
	dir, photosDir, gpxPath, outputDir string
}

// newHike copies fixtures into a temporary photos directory, named so that file
// order differs from time order: a.jpg (Sony, no GPS), b.jpg (iPhone, with
// GPS, taken first), and c.jpg (no EXIF, so no time).
func newHike(t *testing.T) hike {
	t.Helper()
	dir := t.TempDir()
	h := hike{
		dir:       dir,
		photosDir: filepath.Join(dir, "photos"),
		gpxPath:   filepath.Join(dir, "track.gpx"),
		outputDir: filepath.Join(dir, "out"),
	}
	if err := os.Mkdir(h.photosDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, fixture := range map[string]string{"a.jpg": "sony.jpg", "b.jpg": "apple.jpg", "c.jpg": "no_exif.jpg"} {
		copyFile(t, filepath.Join(exifTestdata, fixture), filepath.Join(h.photosDir, name))
	}
	writeFile(t, h.gpxPath, trackGPX)
	return h
}

// args returns the flags for h, followed by extra.
func (h hike) args(extra ...string) []string {
	return append([]string{"-photos", h.photosDir, "-gpx", h.gpxPath, "-output", h.outputDir}, extra...)
}

func TestRunWritesPageData(t *testing.T) {
	h := newHike(t)
	// a.jpg's camera: 03:37:16 + 2m44s is exactly the track's last point.
	args := h.args("-offset", filepath.Join(h.photosDir, "a.jpg")+"=2m44s", "-photographer", "Test Hiker")
	writeFile(t, filepath.Join(h.photosDir, "d.jpg"), "not a JPEG") // left off the page

	var stderr bytes.Buffer
	if err := run(t.Context(), args, &stderr); err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr.String())
	}

	apple, err := exif.DecodeFile(filepath.Join(h.photosDir, "b.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	want := []site.Photo{
		{
			File:                  "b.jpg",
			Time:                  apple.Time,
			RecordedOffsetMinutes: new(-420),
			Location:              &site.Location{Lat: apple.GPS.Lat, Lon: apple.GPS.Lon, Source: "exif", ElevationMeters: new(1650.5)},
			Camera:                site.Camera{Make: "Apple", Model: "iPhone 13 Pro"},
		},
		{
			File:                  "a.jpg",
			Time:                  time.Date(2025, 8, 3, 3, 40, 0, 0, time.UTC),
			RecordedOffsetMinutes: new(-420),
			Location:              &site.Location{Lat: 47.1, Lon: -121.1, Source: "interpolated", ElevationMeters: new(1100.0)},
			Camera:                site.Camera{Make: "SONY", Model: "ILCE-9"},
		},
		{File: "c.jpg"},
	}
	b, err := os.ReadFile(filepath.Join(h.outputDir, site.DataFile))
	if err != nil {
		t.Fatal(err)
	}
	var got site.Data
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if want := "Photos © 2025 Test Hiker. All rights reserved."; got.Copyright != want {
		t.Errorf("copyright = %q, want %q", got.Copyright, want)
	}
	ignoreDetails := cmpopts.IgnoreFields(site.Photo{}, "Lens", "Settings") // covered by the exif and site tests
	if diff := cmp.Diff(want, got.Photos, ignoreDetails); diff != "" {
		t.Errorf("photos mismatch (-want +got):\n%s", diff)
	}

	// report's own test covers the rest of what's logged.
	for _, line := range []string{
		`msg=camera camera="SONY ILCE-9" total_photos=1 recorded_offsets=-07:00 applied_offset=2m44s located_by_track=1`,
		`msg="photo left off the page" err="strip metadata from ` + filepath.Join(h.photosDir, "d.jpg"),
		`msg=done photos=3`,
	} {
		if !strings.Contains(stderr.String(), line) {
			t.Errorf("stderr doesn't contain %s; got:\n%s", line, stderr.String())
		}
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       func(h hike) []string
		setup      func(t *testing.T, h hike)
		wantCode   int
		wantStderr []string // substrings of what run prints itself
		wantErr    string   // substring of the returned error
	}{
		{
			name:     "help",
			args:     func(hike) []string { return []string{"-h"} },
			wantCode: exitOK,
			wantStderr: []string{
				"Usage: retrace -photos dir -gpx file [flags]",
				"-offset photo=duration", // placeholder from the backquoted usage
			},
		},
		{
			name:     "stray argument, no flags",
			args:     func(hike) []string { return []string{"photos"} },
			wantCode: exitUsage,
			wantStderr: []string{
				`unexpected argument "photos" (retrace takes only flags; quote paths that contain spaces)`,
				"missing required -photos and -gpx",
				"Usage:",
			},
		},
		{
			name:       "bad offset",
			args:       func(h hike) []string { return h.args("-offset", "a.jpg") },
			wantCode:   exitUsage,
			wantStderr: []string{`invalid value "a.jpg" for flag -offset: want photo=duration`},
		},
		{
			name: "output is the photos directory",
			args: func(h hike) []string {
				return []string{"-photos", h.photosDir, "-gpx", h.gpxPath, "-output", filepath.Join(h.photosDir, ".")}
			},
			wantCode:   exitUsage,
			wantStderr: []string{"-output can't be the -photos directory"},
		},
		{
			name: "output isn't empty",
			args: func(h hike) []string { return h.args() },
			setup: func(t *testing.T, h hike) {
				if err := os.Mkdir(h.outputDir, 0o750); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(h.outputDir, "notes.txt"), "mine")
			},
			wantCode: exitError,
			wantErr:  "isn't empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHike(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			var stderr bytes.Buffer
			err := run(t.Context(), tt.args(h), &stderr)
			if got := exitCode(err); got != tt.wantCode {
				t.Errorf("exit code = %d, want %d (err: %v)", got, tt.wantCode, err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("error = %v, want one containing %q", err, tt.wantErr)
			}
			for _, s := range tt.wantStderr {
				if !strings.Contains(stderr.String(), s) {
					t.Errorf("stderr doesn't contain %q; got:\n%s", s, stderr.String())
				}
			}
		})
	}
}

func TestRunLeavesNoOutputOnError(t *testing.T) {
	h := newHike(t)
	writeFile(t, h.gpxPath, "<gpx></gpx>") // no track points

	var stderr bytes.Buffer
	if err := run(t.Context(), h.args(), &stderr); err == nil {
		t.Fatal("run succeeded with a GPX file that has no track points")
	}
	if _, err := os.Stat(h.outputDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after a failed run, stat %s: %v; want it not to exist", h.outputDir, err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G703: to is inside t.TempDir()
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	//nolint:gosec // G703: path is inside t.TempDir()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
