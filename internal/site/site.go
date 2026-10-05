// Package site writes retrace's output directory.
//
// [New] converts the track and the enriched photos into [Data], the JSON the
// web page reads. Its types are separate from [photo.Photo] and [gpx.Track]:
// the JSON is a contract with the page's JavaScript, so its field names carry
// units (elevation_meters, seconds_from_track_end) and it leaves out what the
// page doesn't need.
//
// [CheckDir] checks the output directory without changing anything, so a bad
// one fails before any work; [Write] creates it and writes the page into it.
package site

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/photo"
)

// Data is everything the page shows.
type Data struct {
	Track  Track   `json:"track"`
	Photos []Photo `json:"photos"` // by time, photos without a time last; never null
}

// Track is the recorded GPX track.
type Track struct {
	Name    string       `json:"name,omitzero"`
	Creator string       `json:"creator,omitzero"` // the app that exported the GPX file
	Points  []TrackPoint `json:"points"`
}

// TrackPoint is a recorded position on the track.
type TrackPoint struct {
	Lat             float64   `json:"lat"`
	Lon             float64   `json:"lon"`
	Time            time.Time `json:"time"`
	ElevationMeters *float64  `json:"elevation_meters,omitzero"` // omitted when unknown; 0 is sea level
}

// Photo is a photo and where it was taken.
type Photo struct {
	File                  string    `json:"file"`                             // name in the photos directory
	Time                  time.Time `json:"time,omitzero"`                    // corrected, in UTC; omitted when unknown
	RecordedOffsetMinutes *int      `json:"recorded_offset_minutes,omitzero"` // the camera's timezone, in minutes east of UTC, to show local time; omitted when it recorded none
	Location              *Location `json:"location,omitzero"`                // omitted when the photo couldn't be located
	Camera                Camera    `json:"camera,omitzero"`
	Lens                  string    `json:"lens,omitzero"`
	Settings              Settings  `json:"settings,omitzero"`
}

// Location is where a photo was taken and how that was determined.
type Location struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Source string  `json:"source"` // "exif", "interpolated", or "track_end"
	// SecondsFromTrackEnd is set for "track_end": negative before the start,
	// positive after the end.
	SecondsFromTrackEnd float64  `json:"seconds_from_track_end,omitzero"`
	ElevationMeters     *float64 `json:"elevation_meters,omitzero"` // omitted when unknown; 0 is sea level
}

// Camera is the device a photo was taken with, as its EXIF names it.
type Camera struct {
	Make  string `json:"make,omitzero"`
	Model string `json:"model,omitzero"`
}

// Settings are the camera settings a photo was taken with; each is omitted when unknown.
type Settings struct {
	FocalLengthMM          float64  `json:"focal_length_mm,omitzero"`
	FNumber                float64  `json:"f_number,omitzero"`
	ISO                    int      `json:"iso,omitzero"`
	ExposureTimeSeconds    float64  `json:"exposure_time_seconds,omitzero"`
	ExposureCompensationEV *float64 `json:"exposure_compensation_ev,omitzero"`
}

// New returns the page data for track and photos. Photos are sorted by time,
// since file names don't always follow capture order; photos without a time
// go last, in their original order.
func New(track gpx.Track, photos []photo.Photo) Data {
	d := Data{
		Track: Track{
			Name:    track.Name,
			Creator: track.Creator,
			Points:  make([]TrackPoint, 0, len(track.Points)),
		},
		Photos: make([]Photo, 0, len(photos)),
	}
	for _, tp := range track.Points {
		d.Track.Points = append(d.Track.Points, TrackPoint{
			Lat:             tp.Point.Lat,
			Lon:             tp.Point.Lon,
			Time:            tp.Time,
			ElevationMeters: tp.ElevationMeters,
		})
	}
	for _, p := range photos {
		d.Photos = append(d.Photos, newPhoto(p))
	}
	slices.SortStableFunc(d.Photos, func(a, b Photo) int {
		switch {
		case a.Time.IsZero() && b.Time.IsZero():
			return 0
		case a.Time.IsZero():
			return 1
		case b.Time.IsZero():
			return -1
		}
		return a.Time.Compare(b.Time)
	})
	return d
}

// newPhoto converts p for the page.
func newPhoto(p photo.Photo) Photo {
	sp := Photo{
		File:   filepath.Base(p.Path),
		Time:   p.Time,
		Camera: Camera{Make: p.Camera.Make, Model: p.Camera.Model},
		Lens:   p.Lens,
		Settings: Settings{
			FocalLengthMM:          p.Settings.FocalLength,
			FNumber:                p.Settings.FNumber,
			ISO:                    p.Settings.ISO,
			ExposureTimeSeconds:    p.Settings.ExposureTime.Seconds(),
			ExposureCompensationEV: p.Settings.ExposureCompensation,
		},
	}
	if off := p.RecordedOffset; off != nil {
		sp.RecordedOffsetMinutes = new(int(off.Minutes()))
	}
	if loc := p.Location; loc != nil {
		sp.Location = &Location{
			Lat:                 loc.Point.Lat,
			Lon:                 loc.Point.Lon,
			Source:              string(loc.Source),
			SecondsFromTrackEnd: loc.TimeFromTrackEnd.Seconds(),
			ElevationMeters:     loc.ElevationMeters,
		}
	}
	return sp
}
