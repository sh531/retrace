// Package geo defines a position on Earth and the limits used to validate it.
//
// Coordinates are WGS84 decimal degrees, as used by GPS receivers, GPX files,
// and EXIF: latitude from -90 (south) to 90 (north), longitude from -180
// (west) to 180 (east). Packages that read coordinates from files check them
// against [MinLat], [MaxLat], [MinLon], and [MaxLon] before creating a [Point].
package geo

// Coordinate limits in decimal degrees.
const (
	MaxLat = 90
	MinLat = -MaxLat
	MaxLon = 180 // inclusive: GPX says < 180, but 180 is the same meridian as -180
	MinLon = -MaxLon
)

// Point is a WGS84 position in decimal degrees.
type Point struct {
	Lat, Lon float64
}
