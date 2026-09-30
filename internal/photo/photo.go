// Package photo defines a photo and the metadata retrace attaches to it.
package photo

import (
	"time"

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
	Path         string
	Time         time.Time // The camera's EXIF DateTimeOriginal, read as UTC. --offset corrects both timezone and drift
	Location     *Location // nil when the photo has no location
	Camera, Lens string
	NearbyPOIs   []poi.POI
	Tags         []string
}
