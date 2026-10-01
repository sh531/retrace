package photo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoPhotos is returned when a folder has no photos retrace can read.
var ErrNoPhotos = errors.New("no .jpg or .jpeg files (HEIC and RAW aren't supported; export as JPEG)")

// Find returns the paths of the JPEG files directly inside dir, sorted by
// name. Subfolders and hidden files (such as macOS "._" files) are skipped.
func Find(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir) // sorted by filename
	if err != nil {
		return nil, fmt.Errorf("find photos: %w", err) // os errors include the path
	}

	var paths []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !isJPEG(name) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("find photos in %s: %w", dir, ErrNoPhotos)
	}
	return paths, nil
}

// isJPEG reports whether name has a JPEG extension, in any case.
func isJPEG(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return true
	}
	return false
}
