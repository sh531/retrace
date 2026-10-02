package photo

import (
	"context"

	"github.com/sh531/retrace/internal/exif"
)

// EXIFEnricher fills in a photo from the EXIF metadata of the file at its Path.
// It satisfies enrich.Enricher.
type EXIFEnricher struct{}

// Name returns "exif".
func (EXIFEnricher) Name() string { return "exif" }

// Enrich returns the photo at p.Path built with [FromEXIF]. A file without EXIF
// is not an error; a malformed one or a non-JPEG is.
func (EXIFEnricher) Enrich(_ context.Context, p Photo) (Photo, error) {
	m, err := exif.DecodeFile(p.Path)
	if err != nil {
		return p, err
	}
	return FromEXIF(p.Path, m), nil
}
