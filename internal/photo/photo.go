// Package photo defines [Photo], a photo and what retrace learns about it, and
// finds the photos to process.
//
// [Find] lists the JPEG files in a folder, and [FromEXIF] builds a Photo from
// what [exif.DecodeFile] read from one:
//
//	paths, err := photo.Find(dir)
//	...
//	m, err := exif.DecodeFile(path)
//	...
//	p := photo.FromEXIF(path, m)
//
// A photo's [Location] records where it was taken and how that was determined
// ([SourceEXIF] or [SourceInterpolated]). It is nil when the location is unknown,
// so a missing location can't be mistaken for (0,0).
package photo

import (
	"time"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
	"github.com/sh531/retrace/internal/poi"
)

// LocationSource records how a photo's Location was determined.
type LocationSource string

// Location sources.
const (
	SourceEXIF         LocationSource = "exif"
	SourceInterpolated LocationSource = "interpolated"
)

// Location is where a photo was taken and how that was determined.
type Location struct {
	Point  geo.Point
	Source LocationSource
}

// Photo is an image and what retrace learns about it.
type Photo struct {
	Path        string
	Time        time.Time // when the photo was taken, in UTC; zero when unknown
	Location    *Location // nil when the photo has no location
	Camera      exif.Camera
	Lens        string
	Settings    exif.Settings
	Orientation int // EXIF orientation 1–8; 0 (unknown) and 1 both mean upright
	NearbyPOIs  []poi.POI
	Tags        []string
}

// FromEXIF returns the photo at path with what its EXIF says. Time is the
// camera's time before its --offset is added; Location is set from EXIF GPS.
func FromEXIF(path string, m exif.Metadata) Photo {
	p := Photo{
		Path:        path,
		Time:        m.Time,
		Camera:      m.Camera,
		Lens:        m.Lens,
		Settings:    m.Settings,
		Orientation: m.Orientation,
	}
	if m.GPS != nil {
		p.Location = &Location{Point: *m.GPS, Source: SourceEXIF}
	}
	return p
}
