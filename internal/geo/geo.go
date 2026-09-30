// Package geo provides geographic primitives.
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
