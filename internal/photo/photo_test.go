package photo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
)

// touch creates empty files and folders (names ending in "/") under dir.
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

func TestFind(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir,
		"c.jpeg", "a.jpg", "b.JPG", "d.JPEG", // any case, returned sorted
		"._a.jpg", ".hidden.jpg", // hidden, e.g. macOS AppleDouble files
		"e.heic", "f.ARW", "notes.txt", "a.jpg.xmp", // not JPEG
		"folder.jpg/", "sub/", "sub/g.jpg", // subfolders aren't searched
	)

	got, err := Find(dir)
	if err != nil {
		t.Fatalf("Find() unexpected error: %v", err)
	}
	var want []string
	for _, name := range []string{"a.jpg", "b.JPG", "c.jpeg", "d.JPEG"} {
		want = append(want, filepath.Join(dir, name))
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Find() mismatch (-want +got):\n%s", diff)
	}
}

func TestFindErrors(t *testing.T) {
	t.Run("no JPEGs", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "a.heic", "._b.jpg", "sub/", "sub/c.jpg")
		_, err := Find(dir)
		if !errors.Is(err, ErrNoPhotos) {
			t.Errorf("Find() error = %v, want errors.Is ErrNoPhotos", err)
		}
		if err != nil && !strings.Contains(err.Error(), dir) {
			t.Errorf("Find() error = %q, want it to name %s", err, dir)
		}
	})

	t.Run("missing folder", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does-not-exist")
		_, err := Find(dir)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Find() error = %v, want errors.Is fs.ErrNotExist", err)
		}
		if err != nil && strings.Count(err.Error(), dir) != 1 {
			t.Errorf("Find() error = %q, want the path exactly once", err)
		}
	})
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
				Camera:      exif.Camera{Make: "Apple", Model: "iPhone 13 Pro"},
				Lens:        "iPhone 13 Pro back triple camera 5.7mm f/1.5",
				Settings:    settings,
				Time:        taken,
				TimeOffset:  new(-7 * time.Hour),
				GPS:         &geo.Point{Lat: 48.8961, Lon: -121.6617},
				Orientation: 6,
			},
			want: Photo{
				Path:        "IMG_1.jpg",
				Time:        taken,
				Location:    &Location{Point: geo.Point{Lat: 48.8961, Lon: -121.6617}, Source: SourceEXIF},
				Camera:      exif.Camera{Make: "Apple", Model: "iPhone 13 Pro"},
				Lens:        "iPhone 13 Pro back triple camera 5.7mm f/1.5",
				Settings:    settings,
				Orientation: 6,
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
