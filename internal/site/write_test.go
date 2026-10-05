package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
)

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
				touch(t, filepath.Join(dir, DataFile))
			},
			wantErr: "isn't empty",
		},
		{name: "a file", setup: touch, wantErr: "not a directory"},
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
	srcDir := t.TempDir()
	fixture, err := os.ReadFile("../exif/testdata/apple.jpg") // GPS and Orientation 6
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(srcDir, "good.jpg"), fixture)
	writeTestFile(t, filepath.Join(srcDir, "bad.jpg"), []byte("not a JPEG"))

	good := Photo{File: "good.jpg", Time: start}
	input := Data{
		// A name that would end the inlined <script> early if it weren't escaped.
		Track:  Track{Name: `</script><b>"Hike"</b>`, Points: []TrackPoint{{Lat: 47.5, Lon: -120.8, Time: start}}},
		Photos: []Photo{{File: "bad.jpg"}, good},
	}
	original := Data{Track: input.Track, Photos: slices.Clone(input.Photos)}
	want := Data{Track: input.Track, Photos: []Photo{good}} // bad.jpg left off

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
			skipped, err := Write(dir, srcDir, input)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}

			if len(skipped) != 1 || !strings.Contains(skipped[0].Error(), "bad.jpg") {
				t.Errorf("skipped = %v, want one error about bad.jpg", skipped)
			}
			if diff := cmp.Diff([]string{"assets/app.js", "assets/style.css", "index.html", "photos/good.jpg", "retrace.json"}, files(t, dir)); diff != "" {
				t.Errorf("files mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(want, decode(t, readFile(t, filepath.Join(dir, DataFile)))); diff != "" {
				t.Errorf("%s mismatch (-want +got):\n%s", DataFile, diff)
			}
			if diff := cmp.Diff(want, decode(t, inlinedData(t, readFile(t, filepath.Join(dir, pageFile))))); diff != "" {
				t.Errorf("data inlined in %s mismatch (-want +got):\n%s", pageFile, diff)
			}

			m, err := exif.Decode(bytes.NewReader(readFile(t, filepath.Join(dir, photosDir, "good.jpg"))))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(exif.Metadata{Orientation: 6}, m); diff != "" {
				t.Errorf("published photo's metadata (-want +got):\n%s", diff)
			}
		})
	}
	if diff := cmp.Diff(original, input); diff != "" {
		t.Errorf("Write changed the caller's data (-before +after):\n%s", diff)
	}
}

func TestWriteRemovesDirOnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	nan := Data{Track: Track{Points: []TrackPoint{{Lat: math.NaN()}}}} // JSON can't encode NaN
	if _, err := Write(dir, t.TempDir(), nan); err == nil {
		t.Fatal("Write succeeded with data JSON can't encode")
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after a failed Write, stat %s: %v; want it not to exist", dir, err)
	}
}

// inlinedData returns the JSON inlined in the page's data script.
func inlinedData(t *testing.T, page []byte) []byte {
	t.Helper()
	_, after, ok := bytes.Cut(page, []byte(`<script id="data" type="application/json">`))
	data, _, ok2 := bytes.Cut(after, []byte("</script>"))
	if !ok || !ok2 {
		t.Fatalf("no data script in the page:\n%s", page)
	}
	return data
}

// files returns the paths of the files under dir, relative to it.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func decode(t *testing.T, b []byte) Data {
	t.Helper()
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return d
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

// touch creates an empty file at path.
func touch(t *testing.T, path string) {
	t.Helper()
	writeTestFile(t, path, nil)
}

func writeTestFile(t *testing.T, path string, b []byte) {
	t.Helper()
	//nolint:gosec // G703: path is inside t.TempDir()
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
