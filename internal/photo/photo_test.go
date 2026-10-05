package photo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
)

// touch creates empty files and directories (names ending in "/") under dir.
func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFromEXIF(t *testing.T) {
	taken := time.Date(2025, 8, 3, 3, 32, 9, 0, time.UTC)
	settings := exif.Settings{FocalLength: 5.7, FNumber: 1.5, ISO: 50, ExposureTime: time.Second / 452, ExposureCompensation: new(0.0)}

	tests := []struct {
		name string
		m    exif.Metadata
		want Photo
	}{
		{
			name: "with GPS",
			m: exif.Metadata{
				Camera:         exif.Camera{Make: "Apple", Model: "iPhone 13 Pro"},
				Lens:           "iPhone 13 Pro back triple camera 5.7mm f/1.5",
				Settings:       settings,
				Time:           taken,
				TimeOffset:     new(-7 * time.Hour),
				GPS:            &geo.Point{Lat: 48.8961, Lon: -121.6617},
				AltitudeMeters: new(1650.5),
				Orientation:    6,
			},
			want: Photo{
				Path:           "IMG_1.jpg",
				Time:           taken,
				RecordedOffset: new(-7 * time.Hour),
				Location:       &Location{Point: geo.Point{Lat: 48.8961, Lon: -121.6617}, Source: SourceEXIF, ElevationMeters: new(1650.5)},
				Camera:         exif.Camera{Make: "Apple", Model: "iPhone 13 Pro"},
				Lens:           "iPhone 13 Pro back triple camera 5.7mm f/1.5",
				Settings:       settings,
				Orientation:    6,
			},
		},
		{
			name: "without GPS",
			m:    exif.Metadata{Camera: exif.Camera{Make: "SONY", Model: "ILCE-9"}, Time: taken},
			want: Photo{Path: "IMG_1.jpg", Time: taken, Camera: exif.Camera{Make: "SONY", Model: "ILCE-9"}},
		},
		{
			name: "no EXIF",
			want: Photo{Path: "IMG_1.jpg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromEXIF("IMG_1.jpg", tt.m)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FromEXIF() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
