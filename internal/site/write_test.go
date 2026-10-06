package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
				touch(t, filepath.Join(dir, pageFile))
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
		Track:     Track{Name: `</script><b>"Hike"</b>`, Points: []TrackPoint{{Lat: 47.5, Lon: -120.8, Time: start}}},
		Photos:    []Photo{{File: "bad.jpg"}, good},
		Copyright: "Photos © 2024 Ann & Bo. All rights reserved.",
	}
	original := Data{Track: input.Track, Photos: slices.Clone(input.Photos), Copyright: input.Copyright}
	want := Data{Track: input.Track, Photos: []Photo{good}, Copyright: input.Copyright} // bad.jpg left off

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
			skipped, err := Write(t.Context(), dir, srcDir, input)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}

			if len(skipped) != 1 || !strings.Contains(skipped[0].Error(), "bad.jpg") {
				t.Errorf("skipped = %v, want one error about bad.jpg", skipped)
			}
			if diff := cmp.Diff([]string{"assets/app.js", "assets/format.js", "assets/style.css", "assets/timeline.js", "assets/track.js", "index.html", "photos/good.jpg"}, files(t, dir)); diff != "" {
				t.Errorf("files mismatch (-want +got):\n%s", diff)
			}
			page := readFile(t, filepath.Join(dir, pageFile))
			if diff := cmp.Diff(want, decode(t, inlinedData(t, page))); diff != "" {
				t.Errorf("data inlined in %s mismatch (-want +got):\n%s", pageFile, diff)
			}
			if notice := "<p class=\"muted\">Photos © 2024 Ann &amp; Bo. All rights reserved.</p>"; !bytes.Contains(page, []byte(notice)) {
				t.Errorf("%s doesn't show the notice %s", pageFile, notice)
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
	if _, err := Write(t.Context(), dir, t.TempDir(), nan); err == nil {
		t.Fatal("Write succeeded with data JSON can't encode")
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after a failed Write, stat %s: %v; want it not to exist", dir, err)
	}
}

func TestCopyPhotos(t *testing.T) {
	srcDir := t.TempDir()
	fixture, err := os.ReadFile("../exif/testdata/apple.jpg")
	if err != nil {
		t.Fatal(err)
	}
	// Good and bad photos alternate, so the bad ones, which fail at once,
	// finish before good ones started earlier; the results must still be in
	// the photos' order.
	var photos, wantCopied []Photo
	var wantSkipped, wantFiles []string
	for i := range 20 {
		name := fmt.Sprintf("%02d.jpg", i)
		p := Photo{File: name}
		photos = append(photos, p)
		if i%2 == 0 {
			writeTestFile(t, filepath.Join(srcDir, name), fixture)
			wantCopied = append(wantCopied, p)
			wantFiles = append(wantFiles, name)
		} else {
			writeTestFile(t, filepath.Join(srcDir, name), []byte("not a JPEG"))
			wantSkipped = append(wantSkipped, name)
		}
	}

	for _, workers := range []int{1, 8} {
		t.Run(fmt.Sprintf("%d workers", workers), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "photos")
			copied, skipped, err := copyPhotos(t.Context(), dir, srcDir, photos, workers)
			if err != nil {
				t.Fatalf("copyPhotos: %v", err)
			}
			if diff := cmp.Diff(wantCopied, copied); diff != "" {
				t.Errorf("copied mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantSkipped, skippedNames(t, skipped, photos)); diff != "" {
				t.Errorf("skipped mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantFiles, files(t, dir)); diff != "" {
				t.Errorf("files mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCopyPhotosWriteError(t *testing.T) {
	srcDir := t.TempDir()
	fixture, err := os.ReadFile("../exif/testdata/apple.jpg")
	if err != nil {
		t.Fatal(err)
	}
	mkdir(t, filepath.Join(srcDir, "sub"))
	writeTestFile(t, filepath.Join(srcDir, "sub", "good.jpg"), fixture)
	// The copy goes to dir/sub/good.jpg, and dir has no sub directory.
	_, _, err = copyPhotos(t.Context(), filepath.Join(t.TempDir(), "photos"), srcDir, []Photo{{File: "sub/good.jpg"}}, 8)
	if err == nil {
		t.Fatal("copyPhotos succeeded although a copy couldn't be written")
	}
}

func TestWriteCancelled(t *testing.T) {
	srcDir := t.TempDir()
	fixture, err := os.ReadFile("../exif/testdata/apple.jpg")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(srcDir, "good.jpg"), fixture)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // as Ctrl-C would, before the photos are copied

	dir := filepath.Join(t.TempDir(), "out")
	d := Data{Track: Track{Points: []TrackPoint{{Lat: 47.5, Lon: -120.8, Time: start}}}, Photos: []Photo{{File: "good.jpg"}}}
	if _, err := Write(ctx, dir, srcDir, d); !errors.Is(err, context.Canceled) {
		t.Errorf("Write error = %v, want errors.Is context.Canceled", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after a cancelled Write, stat %s: %v; want it not to exist", dir, err)
	}
}

// skippedNames returns, for each error in skipped, the name of the photo it
// is about.
func skippedNames(t *testing.T, skipped []error, photos []Photo) []string {
	t.Helper()
	var names []string
	for _, err := range skipped {
		i := slices.IndexFunc(photos, func(p Photo) bool { return strings.Contains(err.Error(), p.File) })
		if i < 0 {
			t.Fatalf("skipped error names no photo: %v", err)
		}
		names = append(names, photos[i].File)
	}
	return names
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
