package site

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sh531/retrace/internal/exif"
)

// Files and directories in the output directory.
const (
	DataFile  = "retrace.json" // the page data, for other tools; the page has its own copy inlined
	pageFile  = "index.html"
	assetsDir = "assets"
	photosDir = "photos"
)

// web holds the page: the index.html template, and assets copied as they are.
//
//go:embed web
var web embed.FS

// page is the index.html template. It is parsed when the program starts, so
// a mistake in it fails every test.
var page = template.Must(template.ParseFS(web, "web/"+pageFile))

// CheckDir reports whether dir can take retrace's output: it must not exist
// yet, or be empty, so nothing is overwritten and no files are left over from
// an earlier run. It changes nothing.
func CheckDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // Write creates it
	}
	if err != nil {
		return fmt.Errorf("check output directory: %w", err) // os errors include the path
	}
	if len(entries) > 0 {
		return fmt.Errorf("output directory %s isn't empty; delete it or choose another", dir)
	}
	return nil
}

// Write creates dir and writes the page into it: index.html with d inlined,
// its assets, [DataFile], and d's photos from srcDir with their personal
// information stripped by [exif.StripMetadata]. A photo that can't be read or
// stripped is left off the page, and its error is returned in skipped. If
// Write fails, it removes dir. Call [CheckDir] first.
func Write(dir, srcDir string, d Data) (skipped []error, err error) {
	err = os.Mkdir(dir, 0o750)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("create output directory: %w", err) // os errors include the path
	}
	defer func() {
		if err != nil {
			// CheckDir made sure dir was missing or empty, so only retrace's files are removed.
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()

	if d.Photos, skipped, err = copyPhotos(filepath.Join(dir, photosDir), srcDir, d.Photos); err != nil {
		return nil, err
	}
	assets, err := fs.Sub(web, "web/"+assetsDir)
	if err != nil {
		return nil, err
	}
	if err = os.CopyFS(filepath.Join(dir, assetsDir), assets); err != nil {
		return nil, fmt.Errorf("write page assets: %w", err)
	}
	if err = writeFile(filepath.Join(dir, DataFile), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(d)
	}); err != nil {
		return nil, err
	}
	if err = writeFile(filepath.Join(dir, pageFile), func(w io.Writer) error {
		return page.Execute(w, d)
	}); err != nil {
		return nil, err
	}
	return skipped, nil
}

// copyPhotos copies photos from srcDir into dir without their personal
// information, and returns the ones it copied. A photo that can't be read or
// stripped is left out, and its error is returned in skipped; failing to
// write a copy is an error.
func copyPhotos(dir, srcDir string, photos []Photo) ([]Photo, []error, error) {
	if err := os.Mkdir(dir, 0o750); err != nil {
		return nil, nil, fmt.Errorf("create photos directory: %w", err)
	}
	copied := make([]Photo, 0, len(photos)) // a new slice, so the caller's Data is unchanged
	var skipped []error
	var stripped bytes.Buffer // the whole photo, so a bad one fails before its copy is created
	for _, p := range photos {
		stripped.Reset()
		if err := stripFile(&stripped, filepath.Join(srcDir, p.File)); err != nil {
			skipped = append(skipped, err)
			continue
		}
		if err := writeFile(filepath.Join(dir, p.File), func(w io.Writer) error {
			_, err := stripped.WriteTo(w)
			return err
		}); err != nil {
			return nil, nil, err
		}
		copied = append(copied, p)
	}
	return copied, skipped, nil
}

// stripFile writes the JPEG at path to w without its personal information.
func stripFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err // os errors include the path
	}
	defer f.Close() //nolint:errcheck // read-only; a close error can't lose data

	if err := exif.StripMetadata(w, f); err != nil {
		return fmt.Errorf("strip metadata from %s: %w", path, err)
	}
	return nil
}

// writeFile creates the file at path and writes its contents with write.
func writeFile(path string, write func(io.Writer) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err // os errors include the path
	}
	defer func() { err = errors.Join(err, f.Close()) }() // a failed close can lose written data

	w := bufio.NewWriter(f)
	if err = write(w); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return w.Flush()
}
