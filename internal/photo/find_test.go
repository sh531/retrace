package photo

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFind(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir,
		"c.jpeg", "a.jpg", "b.JPG", "d.JPEG", // any case, returned sorted
		"._a.jpg", ".hidden.jpg", // hidden, e.g. macOS AppleDouble files
		"e.heic", "f.ARW", "notes.txt", "a.jpg.xmp", // not JPEG
		"directory.jpg/", "sub/", "sub/g.jpg", // subdirectories aren't searched
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

	t.Run("missing directory", func(t *testing.T) {
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
