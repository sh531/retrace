package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/photo"
)

var start = time.Date(2024, 10, 12, 14, 0, 0, 0, time.UTC)

func TestNewConvertsTrackAndPhoto(t *testing.T) {
	track := gpx.Track{
		Name:    "Enchantments",
		Creator: "StravaGPX",
		Points: []gpx.TrackPoint{
			{Point: geo.Point{Lat: 47.5, Lon: -120.8}, Time: start, ElevationMeters: new(1042.5)},
			{Point: geo.Point{Lat: 47.6, Lon: -120.9}, Time: start.Add(time.Second)},
		},
	}
	photos := []photo.Photo{{
		Path:           filepath.Join("photos", "DSC00042.jpg"),
		Time:           start.Add(time.Hour),
		RecordedOffset: new(-7 * time.Hour),
		Location: &photo.Location{
			Point:            geo.Point{Lat: 47.6, Lon: -120.9},
			Source:           photo.SourceTrackEnd,
			TimeFromTrackEnd: 90 * time.Minute,
		},
		Camera: exif.Camera{Make: "SONY", Model: "ILCE-9"},
		Lens:   "FE 70-200mm F2.8 GM OSS",
		Settings: exif.Settings{
			FocalLength:          70,
			FNumber:              7.1,
			ISO:                  2500,
			ExposureTime:         12500 * time.Microsecond,
			ExposureCompensation: new(-0.7),
		},
	}}

	want := Data{
		Track: Track{
			Name:    "Enchantments",
			Creator: "StravaGPX",
			Points: []TrackPoint{
				{Lat: 47.5, Lon: -120.8, Time: start, ElevationMeters: new(1042.5)},
				{Lat: 47.6, Lon: -120.9, Time: start.Add(time.Second)},
			},
		},
		Photos: []Photo{{
			File:     "DSC00042.jpg",
			Time:     start.Add(time.Hour),
			Location: &Location{Lat: 47.6, Lon: -120.9, Source: "track_end", SecondsFromTrackEnd: 5400},
			Camera:   Camera{Make: "SONY", Model: "ILCE-9"},
			Lens:     "FE 70-200mm F2.8 GM OSS",
			Settings: Settings{
				FocalLengthMM:          70,
				FNumber:                7.1,
				ISO:                    2500,
				ExposureTimeSeconds:    0.0125,
				ExposureCompensationEV: new(-0.7),
			},
		}},
	}
	if diff := cmp.Diff(want, New(track, photos)); diff != "" {
		t.Errorf("New mismatch (-want +got):\n%s", diff)
	}
}

func TestNewSortsPhotosByTime(t *testing.T) {
	// 14 photos, more than the 12 that slices sorts by insertion sort (which
	// is always stable), so an unstable sort would show. Photos 4 and 9 have
	// no time; the rest share three times, latest first in file order.
	var photos []photo.Photo
	for i := range 14 {
		p := photo.Photo{Path: fmt.Sprintf("%02d.jpg", i)}
		if i%5 != 4 {
			p.Time = start.Add(time.Duration(2-i%3) * time.Hour)
		}
		photos = append(photos, p)
	}

	var got []string
	for _, p := range New(gpx.Track{}, photos).Photos {
		got = append(got, p.File)
	}
	want := []string{
		"02.jpg", "05.jpg", "08.jpg", "11.jpg", // start
		"01.jpg", "07.jpg", "10.jpg", "13.jpg", // start + 1h
		"00.jpg", "03.jpg", "06.jpg", "12.jpg", // start + 2h
		"04.jpg", "09.jpg", // no time, last
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("photo order mismatch (-want +got):\n%s", diff)
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name string
		data Data
		want string
	}{
		{
			name: "empty lists are [] not null",
			data: New(gpx.Track{}, nil),
			want: `{"track":{"points":[]},"photos":[]}`,
		},
		{
			name: "unknowns are omitted, zeros that are real values are kept",
			data: Data{
				Track: Track{Points: []TrackPoint{
					{Lat: 47.5, Lon: -120.8, Time: start},                                     // no elevation
					{Lat: 0, Lon: 0, Time: start.Add(time.Second), ElevationMeters: new(0.0)}, // sea level
				}},
				Photos: []Photo{
					{File: "unknown.jpg"},
					{
						File:     "zero.jpg",
						Time:     start,
						Location: &Location{Source: "exif"},
						Settings: Settings{ExposureCompensationEV: new(0.0)},
					},
				},
			},
			want: `{"track":{"points":[` +
				`{"lat":47.5,"lon":-120.8,"time":"2024-10-12T14:00:00Z"},` +
				`{"lat":0,"lon":0,"time":"2024-10-12T14:00:01Z","elevation_meters":0}]},` +
				`"photos":[` +
				`{"file":"unknown.jpg"},` +
				`{"file":"zero.jpg","time":"2024-10-12T14:00:00Z","location":{"lat":0,"lon":0,"source":"exif"},"settings":{"exposure_compensation_ev":0}}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, string(got)); diff != "" {
				t.Errorf("JSON mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCheckDir(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string) // dir doesn't exist yet
		wantErr string                         // substring; "" means OK
	}{
		{name: "missing"},
		{name: "empty", setup: mkdir},
		{
			name: "not empty",
			setup: func(t *testing.T, dir string) {
				mkdir(t, dir)
				writeFile(t, filepath.Join(dir, DataFile))
			},
			wantErr: "isn't empty",
		},
		{name: "a file", setup: writeFile, wantErr: "not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "out")
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			err := CheckDir(dir)
			if tt.wantErr == "" && err != nil {
				t.Errorf("CheckDir: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("CheckDir error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	want := Data{
		Track:  Track{Points: []TrackPoint{{Lat: 47.5, Lon: -120.8, Time: start}}},
		Photos: []Photo{{File: "a.jpg", Time: start}},
	}
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, dir string) // dir doesn't exist yet
	}{
		{name: "missing directory is created"},
		{name: "empty directory", setup: mkdir},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "out")
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			if err := Write(dir, want); err != nil {
				t.Fatalf("Write: %v", err)
			}

			b, err := os.ReadFile(filepath.Join(dir, DataFile))
			if err != nil {
				t.Fatal(err)
			}
			var got Data
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("decode %s: %v", DataFile, err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("written data mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	//nolint:gosec // G703: path is inside t.TempDir()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
