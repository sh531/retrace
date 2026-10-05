package site

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sh531/retrace/internal/exif"
	"github.com/sh531/retrace/internal/geo"
	"github.com/sh531/retrace/internal/gpx"
	"github.com/sh531/retrace/internal/photo"
)

var start = time.Date(2024, 10, 12, 14, 0, 0, 0, time.UTC)

func TestNewConvertsTrackAndPhoto(t *testing.T) {
	track := gpx.Track{
		Name:    "Enchantments",
		Creator: "StravaGPX",
		Points: []gpx.TrackPoint{
			{Point: geo.Point{Lat: 47.5, Lon: -120.8}, Time: start, ElevationMeters: new(1042.5)},
			{Point: geo.Point{Lat: 47.6, Lon: -120.9}, Time: start.Add(time.Second)},
		},
	}
	photos := []photo.Photo{{
		Path:           filepath.Join("photos", "DSC00042.jpg"),
		Time:           start.Add(time.Hour),
		RecordedOffset: new(-7 * time.Hour),
		Location: &photo.Location{
			Point:            geo.Point{Lat: 47.6, Lon: -120.9},
			Source:           photo.SourceTrackEnd,
			TimeFromTrackEnd: 90 * time.Minute,
			ElevationMeters:  new(2280.4),
		},
		Camera: exif.Camera{Make: "SONY", Model: "ILCE-9"},
		Lens:   "FE 70-200mm F2.8 GM OSS",
		Settings: exif.Settings{
			FocalLength:          70,
			FNumber:              7.1,
			ISO:                  2500,
			ExposureTime:         12500 * time.Microsecond,
			ExposureCompensation: new(-0.7),
		},
	}}

	want := Data{
		Track: Track{
			Name:    "Enchantments",
			Creator: "StravaGPX",
			Points: []TrackPoint{
				{Lat: 47.5, Lon: -120.8, Time: start, ElevationMeters: new(1042.5)},
				{Lat: 47.6, Lon: -120.9, Time: start.Add(time.Second)},
			},
		},
		Photos: []Photo{{
			File:                  "DSC00042.jpg",
			Time:                  start.Add(time.Hour),
			RecordedOffsetMinutes: new(-420),
			Location:              &Location{Lat: 47.6, Lon: -120.9, Source: "track_end", SecondsFromTrackEnd: 5400, ElevationMeters: new(2280.4)},
			Camera:                Camera{Make: "SONY", Model: "ILCE-9"},
			Lens:                  "FE 70-200mm F2.8 GM OSS",
			Settings: Settings{
				FocalLengthMM:          70,
				FNumber:                7.1,
				ISO:                    2500,
				ExposureTimeSeconds:    0.0125,
				ExposureCompensationEV: new(-0.7),
			},
		}},
	}
	if diff := cmp.Diff(want, New(track, photos)); diff != "" {
		t.Errorf("New mismatch (-want +got):\n%s", diff)
	}
}

func TestNewSortsPhotosByTime(t *testing.T) {
	// 14 photos, more than the 12 that slices sorts by insertion sort (which
	// is always stable), so an unstable sort would show. Photos 4 and 9 have
	// no time; the rest share three times, latest first in file order.
	var photos []photo.Photo
	for i := range 14 {
		p := photo.Photo{Path: fmt.Sprintf("%02d.jpg", i)}
		if i%5 != 4 {
			p.Time = start.Add(time.Duration(2-i%3) * time.Hour)
		}
		photos = append(photos, p)
	}

	var got []string
	for _, p := range New(gpx.Track{}, photos).Photos {
		got = append(got, p.File)
	}
	want := []string{
		"02.jpg", "05.jpg", "08.jpg", "11.jpg", // start
		"01.jpg", "07.jpg", "10.jpg", "13.jpg", // start + 1h
		"00.jpg", "03.jpg", "06.jpg", "12.jpg", // start + 2h
		"04.jpg", "09.jpg", // no time, last
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("photo order mismatch (-want +got):\n%s", diff)
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name string
		data Data
		want string
	}{
		{
			name: "empty lists are [] not null",
			data: New(gpx.Track{}, nil),
			want: `{"track":{"points":[]},"photos":[]}`,
		},
		{
			name: "unknowns are omitted, zeros that are real values are kept",
			data: Data{
				Track: Track{Points: []TrackPoint{
					{Lat: 47.5, Lon: -120.8, Time: start},                                     // no elevation
					{Lat: 0, Lon: 0, Time: start.Add(time.Second), ElevationMeters: new(0.0)}, // sea level
				}},
				Photos: []Photo{
					{File: "unknown.jpg"},
					{
						File:                  "zero.jpg",
						Time:                  start,
						RecordedOffsetMinutes: new(0),                                               // UTC
						Location:              &Location{Source: "exif", ElevationMeters: new(0.0)}, // sea level
						Settings:              Settings{ExposureCompensationEV: new(0.0)},
					},
				},
			},
			want: `{"track":{"points":[` +
				`{"lat":47.5,"lon":-120.8,"time":"2024-10-12T14:00:00Z"},` +
				`{"lat":0,"lon":0,"time":"2024-10-12T14:00:01Z","elevation_meters":0}]},` +
				`"photos":[` +
				`{"file":"unknown.jpg"},` +
				`{"file":"zero.jpg","time":"2024-10-12T14:00:00Z","recorded_offset_minutes":0,"location":{"lat":0,"lon":0,"source":"exif","elevation_meters":0},"settings":{"exposure_compensation_ev":0}}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, string(got)); diff != "" {
				t.Errorf("JSON mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
