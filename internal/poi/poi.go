// Package poi defines points of interest near a photo.
package poi

// POI is a named OpenStreetMap feature near a photo.
type POI struct {
	Name           string
	Category       Category
	DistanceMeters float64 // metres from the photo's Location
}

// Category is the kind of feature a POI is.
type Category string

// POI categories.
const (
	CategoryTrail     Category = "trail"
	CategoryTrailhead Category = "trailhead"
	CategoryPeak      Category = "peak"
	CategoryCampsite  Category = "campsite"
	CategoryLake      Category = "lake"
	CategoryWaterfall Category = "waterfall"
	CategoryRiver     Category = "river"
	CategoryStream    Category = "stream"
)
