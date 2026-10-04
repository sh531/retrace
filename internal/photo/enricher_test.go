package photo

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/sh531/retrace/internal/exif"
)

func TestEXIFEnricher(t *testing.T) {
	t.Run("reads the file at Path", func(t *testing.T) {
		path := filepath.Join("..", "exif", "testdata", "apple.jpg")
		got, err := EXIFEnricher{}.Enrich(t.Context(), Photo{Path: path})
		if err != nil {
			t.Fatalf("Enrich() unexpected error: %v", err)
		}
		if got.Path != path || got.Camera.Make != "Apple" || got.Location == nil || got.Time.IsZero() {
			t.Errorf("Enrich() = %+v, want Path %s, Make Apple, a Location, and a Time", got, path)
		}
	})

	t.Run("not a JPEG", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "empty.jpg")
		_, err := EXIFEnricher{}.Enrich(t.Context(), Photo{Path: filepath.Join(dir, "empty.jpg")})
		if !errors.Is(err, exif.ErrNotJPEG) {
			t.Errorf("Enrich() error = %v, want errors.Is exif.ErrNotJPEG", err)
		}
	})
}
