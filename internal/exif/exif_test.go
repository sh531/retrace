package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sh531/retrace/internal/geo"
)

// approx compares floats that went through degrees/minutes/seconds or a rational.
var approx = cmpopts.EquateApprox(0, 1e-9)

// mustParseTime parses an RFC 3339 time as UTC. It panics on error, so it is
// only for hard-coded test values, where an error means a typo.
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err) // test fixture typo
	}
	return t.UTC()
}

// The helpers below build JPEG and TIFF bytes for cases exiftool won't write.
// They use big-endian; the fixtures cover both byte orders.

var be = binary.BigEndian

// entry is an IFD entry. data of up to 4 bytes is stored inline, longer data
// after the IFD. A non-nil sub is written as a nested IFD that entry points to.
type entry struct {
	tag   tag
	typ   fieldType
	count uint32
	data  []byte
	sub   []entry
}

func asciiEntry(t tag, s string) entry {
	return entry{tag: t, typ: typeASCII, count: uint32(len(s) + 1), data: append([]byte(s), 0)}
}

func shortEntry(t tag, v uint16) entry {
	return entry{tag: t, typ: typeShort, count: 1, data: be.AppendUint16(nil, v)}
}

// rationalEntry takes numerator, denominator pairs.
func rationalEntry(t tag, typ fieldType, pairs ...uint32) entry {
	var data []byte
	for _, v := range pairs {
		data = be.AppendUint32(data, v)
	}
	return entry{tag: t, typ: typ, count: uint32(len(pairs) / 2), data: data}
}

func subIFD(t tag, entries ...entry) entry {
	return entry{tag: t, typ: typeLong, count: 1, sub: entries}
}

// tiffData returns big-endian TIFF data whose first IFD holds ifd0.
func tiffData(ifd0 ...entry) []byte {
	return appendIFD([]byte("MM\x00\x2a\x00\x00\x00\x08"), ifd0)
}

func appendIFD(b []byte, entries []entry) []byte {
	start := len(b)
	b = append(b, make([]byte, 2+12*len(entries)+4)...)
	be.PutUint16(b[start:], uint16(len(entries)))
	for i, e := range entries {
		p := start + 2 + 12*i
		be.PutUint16(b[p:], uint16(e.tag))
		be.PutUint16(b[p+2:], uint16(e.typ))
		be.PutUint32(b[p+4:], e.count)
		switch {
		case e.sub != nil:
			be.PutUint32(b[p+8:], uint32(len(b)))
			b = appendIFD(b, e.sub)
		case len(e.data) <= 4:
			copy(b[p+8:p+12], e.data)
		default:
			be.PutUint32(b[p+8:], uint32(len(b)))
			b = append(b, e.data...)
		}
	}
	return b
}

// segment returns a JPEG marker segment with a length and payload.
func segment(marker byte, payload []byte) []byte {
	b := []byte{0xFF, marker}
	b = be.AppendUint16(b, uint16(len(payload)+2))
	return append(b, payload...)
}

// exifSegment wraps TIFF data in an EXIF APP1 segment.
func exifSegment(tiff []byte) []byte {
	return segment(markerAPP1, append([]byte("Exif\x00\x00"), tiff...))
}

// jpegFile joins parts between the start and end of image markers.
func jpegFile(parts ...[]byte) []byte {
	b := []byte{0xFF, markerSOI}
	for _, p := range parts {
		b = append(b, p...)
	}
	return append(b, 0xFF, markerEOI)
}

// withEXIF returns a JPEG whose EXIF IFD0 holds ifd0.
func withEXIF(ifd0 ...entry) []byte {
	return jpegFile(exifSegment(tiffData(ifd0...)))
}

func TestDecodeFixtures(t *testing.T) {
	tests := []struct {
		file string
		want Metadata
	}{
		{
			file: "apple.jpg",
			want: Metadata{
				Camera: Camera{Make: "Apple", Model: "iPhone 13 Pro"},
				Lens:   "iPhone 13 Pro back triple camera 5.7mm f/1.5",
				Settings: Settings{
					FocalLength:          5.7,
					FNumber:              1.5,
					ISO:                  50,
					ExposureTime:         time.Second / 452,
					ExposureCompensation: new(0.0),
				},
				Time:        mustParseTime("2025-08-02T20:32:09.236-07:00"),
				TimeOffset:  new(-7 * time.Hour),
				GPS:         &geo.Point{Lat: 48.8961, Lon: -121.6617},
				Orientation: 6,
			},
		},
		{
			file: "sony.jpg",
			want: Metadata{
				Camera: Camera{Make: "SONY", Model: "ILCE-9"},
				Lens:   "FE 16-35mm F2.8 GM",
				Settings: Settings{
					FocalLength:          17,
					FNumber:              3.2,
					ISO:                  200,
					ExposureTime:         8 * time.Millisecond,
					ExposureCompensation: new(-0.7),
				},
				Time:       mustParseTime("2025-08-02T20:37:16-07:00"),
				TimeOffset: new(-7 * time.Hour),
			},
		},
		{
			file: "gps_south_east.jpg",
			want: Metadata{
				Camera: Camera{Make: "Apple"},
				Time:   mustParseTime("2025-01-15T06:00:00Z"), // no OffsetTimeOriginal: assumed to be UTC
				GPS:    &geo.Point{Lat: -33.8568, Lon: 151.2153},
			},
		},
		{file: "gps_void.jpg", want: Metadata{Camera: Camera{Make: "Apple"}}},
		{file: "gps_zero.jpg", want: Metadata{Camera: Camera{Make: "Apple"}}},
		{file: "no_exif.jpg", want: Metadata{}},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := DecodeFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("DecodeFile() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, approx); diff != "" {
				t.Errorf("DecodeFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	makeApple := asciiEntry(tagMake, "Apple")
	tests := []struct {
		name  string
		input []byte
		want  Metadata
	}{
		{
			name:  "image data before any EXIF",
			input: jpegFile(segment(0xE0, []byte("JFIF\x00")), []byte{0xFF, markerSOS}, exifSegment(tiffData(makeApple))),
			want:  Metadata{},
		},
		{
			name: "XMP APP1 before EXIF is skipped",
			input: jpegFile(
				segment(markerAPP1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>")),
				exifSegment(tiffData(makeApple)),
			),
			want: Metadata{Camera: Camera{Make: "Apple"}},
		},
		{
			name:  "fill bytes and markers without a length",
			input: jpegFile([]byte{0xFF, 0xFF, 0xFF, markerRST0, 0xFF, markerTEM}, exifSegment(tiffData(makeApple))),
			want:  Metadata{Camera: Camera{Make: "Apple"}},
		},
		{
			name:  "unknown entries and types are ignored",
			input: withEXIF(entry{tag: 0x1234, typ: 99, count: 1_000_000}, makeApple),
			want:  Metadata{Camera: Camera{Make: "Apple"}},
		},
		{
			name:  "strings end at NUL and are trimmed",
			input: withEXIF(entry{tag: tagMake, typ: typeASCII, count: 8, data: []byte(" Canon \x00x")}),
			want:  Metadata{Camera: Camera{Make: "Canon"}},
		},
		{
			name: "sub-IFD pointer of type IFD, LONG ISO",
			input: withEXIF(entry{tag: tagExifOffset, typ: typeIFD, count: 1, sub: []entry{
				{tag: tagISO, typ: typeLong, count: 1, data: be.AppendUint32(nil, 102400)},
			}}),
			want: Metadata{Settings: Settings{ISO: 102400}},
		},
		{
			name: "several SHORTs stored at an offset",
			input: withEXIF(subIFD(tagExifOffset,
				entry{tag: tagISO, typ: typeShort, count: 3, data: []byte{0x01, 0x90, 0, 0, 0, 0}}, // 400, 0, 0
			)),
			want: Metadata{Settings: Settings{ISO: 400}},
		},
		{
			name: "zero and negative values are unknown",
			input: withEXIF(subIFD(tagExifOffset,
				rationalEntry(tagFNumber, typeRational, 0, 0), // manual lens
				rationalEntry(tagFocalLength, typeSRational, 0xFFFFFFFF, 1),
				rationalEntry(tagExposureTime, typeRational, 1, 0),
				rationalEntry(tagExposureCompensation, typeSRational, 1, 0),
			)),
			want: Metadata{},
		},
		{
			name: "negative exposure compensation",
			input: withEXIF(subIFD(tagExifOffset,
				rationalEntry(tagExposureCompensation, typeSRational, uint32(0xFFFFFFFF-6), 3), // -7/3
			)),
			want: Metadata{Settings: Settings{ExposureCompensation: new(-7.0 / 3)}},
		},
		{
			name: "long exposure",
			input: withEXIF(subIFD(tagExifOffset,
				rationalEntry(tagExposureTime, typeRational, 30, 1),
			)),
			want: Metadata{Settings: Settings{ExposureTime: 30 * time.Second}},
		},
		{
			name:  "orientation",
			input: withEXIF(shortEntry(tagOrientation, 8)),
			want:  Metadata{Orientation: 8},
		},
		{
			name:  "invalid orientation is ignored",
			input: withEXIF(shortEntry(tagOrientation, 9)),
			want:  Metadata{},
		},
		{
			name: "180 longitude is accepted",
			input: withEXIF(subIFD(tagGPSInfo,
				asciiEntry(tagGPSLatitudeRef, "N"),
				rationalEntry(tagGPSLatitude, typeRational, 1, 1, 0, 1, 0, 1),
				asciiEntry(tagGPSLongitudeRef, "W"),
				rationalEntry(tagGPSLongitude, typeRational, 180, 1, 0, 1, 0, 1),
			)),
			want: Metadata{GPS: &geo.Point{Lat: 1, Lon: -180}},
		},
		{
			name: "GPS status A (active) is a fix",
			input: withEXIF(subIFD(tagGPSInfo,
				asciiEntry(tagGPSStatus, "A"),
				asciiEntry(tagGPSLatitudeRef, "S"),
				rationalEntry(tagGPSLatitude, typeRational, 10, 1, 30, 1, 0, 1),
				asciiEntry(tagGPSLongitudeRef, "E"),
				rationalEntry(tagGPSLongitude, typeRational, 20, 1, 0, 1, 36, 1),
			)),
			want: Metadata{GPS: &geo.Point{Lat: -10.5, Lon: 20.01}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(bytes.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Decode() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, approx); diff != "" {
				t.Errorf("Decode() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	gps := func(entries ...entry) []byte { return withEXIF(subIFD(tagGPSInfo, entries...)) }
	north := asciiEntry(tagGPSLatitudeRef, "N")
	east := asciiEntry(tagGPSLongitudeRef, "E")
	one := rationalEntry(tagGPSLatitude, typeRational, 1, 1, 0, 1, 0, 1)
	oneLon := rationalEntry(tagGPSLongitude, typeRational, 1, 1, 0, 1, 0, 1)
	fixture, err := os.ReadFile("testdata/apple.jpg")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   []byte
		wantIs  error  // checked with errors.Is when set
		wantErr string // substring
	}{
		// JPEG structure
		{name: "empty", input: nil, wantIs: ErrNotJPEG},
		{name: "PNG", input: []byte("\x89PNG\r\n\x1a\n"), wantIs: ErrNotJPEG},
		{name: "ends after SOI", input: []byte{0xFF, markerSOI}, wantIs: io.ErrUnexpectedEOF},
		{name: "fixture truncated inside EXIF", input: fixture[:100], wantIs: io.ErrUnexpectedEOF},
		{name: "truncated skipped segment", input: []byte{0xFF, markerSOI, 0xFF, 0xE0, 0x00, 0x10, 0x00}, wantIs: io.ErrUnexpectedEOF},
		{name: "truncated segment length", input: []byte{0xFF, markerSOI, 0xFF, 0xE0, 0x00}, wantIs: io.ErrUnexpectedEOF},
		{name: "ends in fill bytes", input: []byte{0xFF, markerSOI, 0xFF, 0xFF}, wantIs: io.ErrUnexpectedEOF},
		{name: "missing marker", input: []byte{0xFF, markerSOI, 0x00}, wantErr: "want JPEG marker, got byte 0x00"},
		{name: "segment length below 2", input: []byte{0xFF, markerSOI, 0xFF, 0xE0, 0x00, 0x01}, wantErr: "JPEG segment 0xE0: invalid length 1"},

		// TIFF structure
		{name: "short TIFF header", input: jpegFile(exifSegment([]byte("MM\x00"))), wantErr: "TIFF header too short"},
		{name: "invalid byte order", input: jpegFile(exifSegment([]byte("XX\x00\x2a\x00\x00\x00\x08"))), wantErr: `invalid byte order "XX"`},
		{name: "invalid magic number", input: jpegFile(exifSegment([]byte("II\x2b\x00\x08\x00\x00\x00"))), wantErr: "invalid TIFF magic number 43"},
		{name: "IFD0 offset out of range", input: jpegFile(exifSegment([]byte("MM\x00\x2a\xff\xff\xff\xff"))), wantErr: "IFD offset 4294967295 out of range"},
		{name: "IFD entries past the end", input: jpegFile(exifSegment([]byte("MM\x00\x2a\x00\x00\x00\x08\x00\x05"))), wantErr: "5 entries run past the end"},
		{
			name:    "value offset out of range",
			input:   withEXIF(entry{tag: tagMake, typ: typeASCII, count: 100, data: []byte{0xFF, 0xFF, 0xFF, 0xF0}}),
			wantErr: "Make: 100 bytes at offset 4294967280 run past the end",
		},
		{
			name:    "huge count doesn't overflow",
			input:   withEXIF(subIFD(tagExifOffset, entry{tag: tagFNumber, typ: typeRational, count: math.MaxUint32, data: []byte{0, 0, 0, 8}})),
			wantErr: "FNumber: 34359738360 bytes at offset 8 run past the end",
		},
		{name: "Make isn't ASCII", input: withEXIF(shortEntry(tagMake, 1)), wantErr: "Make: type SHORT, want ASCII"},
		{name: "unknown type is named by number", input: withEXIF(entry{tag: tagMake, typ: 99, count: 1}), wantErr: "Make: type 99, want ASCII"},
		{name: "ISO isn't a number", input: withEXIF(subIFD(tagExifOffset, asciiEntry(tagISO, "100"))), wantErr: "ISO: type ASCII, want SHORT or LONG or IFD"},
		{name: "Orientation isn't a number", input: withEXIF(asciiEntry(tagOrientation, "6")), wantErr: "Orientation: type ASCII, want SHORT or LONG or IFD"},
		{
			name:    "SHORT values out of range",
			input:   withEXIF(subIFD(tagExifOffset, entry{tag: tagISO, typ: typeShort, count: 3, data: []byte{0, 0, 0xFF, 0xF0}})),
			wantErr: "ISO: 6 bytes at offset 65520 run past the end",
		},
		{name: "ISO has no values", input: withEXIF(subIFD(tagExifOffset, entry{tag: tagISO, typ: typeShort})), wantErr: "ISO: no values"},
		{name: "FNumber isn't rational", input: withEXIF(subIFD(tagExifOffset, shortEntry(tagFNumber, 4))), wantErr: "FNumber: type SHORT, want RATIONAL or SRATIONAL"},
		{
			name:    "invalid DateTimeOriginal",
			input:   withEXIF(subIFD(tagExifOffset, asciiEntry(tagDateTimeOriginal, "2025-08-02 20:32:09"))),
			wantErr: `invalid time (DateTimeOriginal "2025-08-02 20:32:09"`,
		},
		{name: "Exif IFD out of range", input: withEXIF(entry{tag: tagExifOffset, typ: typeLong, count: 1, data: []byte{0, 0, 1, 0}}), wantErr: "IFD offset 256 out of range"},

		// GPS
		{name: "latitude without longitude", input: gps(north, one), wantErr: "GPS has only one of latitude and longitude"},
		{name: "longitude without latitude", input: gps(east, oneLon), wantErr: "GPS has only one of latitude and longitude"},
		{name: "too few GPS values", input: gps(north, rationalEntry(tagGPSLatitude, typeRational, 1, 1), east, oneLon), wantErr: "GPSLatitude: 1 values, want 3"},
		{name: "zero denominator", input: gps(north, rationalEntry(tagGPSLatitude, typeRational, 1, 1, 0, 0, 0, 1), east, oneLon), wantErr: "GPSLatitude: zero denominator"},
		{name: "invalid reference", input: gps(asciiEntry(tagGPSLatitudeRef, "X"), one, east, oneLon), wantErr: `GPSLatitudeRef: got "X", want "N" or "S"`},
		{name: "missing reference", input: gps(one, east, oneLon), wantErr: `GPSLatitudeRef: got "", want "N" or "S"`},
		{
			name:    "latitude out of range",
			input:   gps(north, rationalEntry(tagGPSLatitude, typeRational, 90, 1, 0, 1, 1, 1), east, oneLon),
			wantErr: "GPS position (90.00027777777778, 1) out of range",
		},
		{
			name:    "longitude out of range",
			input:   gps(north, one, east, rationalEntry(tagGPSLongitude, typeRational, 181, 1, 0, 1, 0, 1)),
			wantErr: "GPS position (1, 181) out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(bytes.NewReader(tt.input))
			if err == nil {
				t.Fatalf("Decode() = %+v, want error", got)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Decode() error = %v, want errors.Is %v", err, tt.wantIs)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Decode() error = %q, want it to contain %q", err, tt.wantErr)
			}
			if diff := cmp.Diff(Metadata{}, got); diff != "" {
				t.Errorf("Decode() returned partial metadata with an error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name               string
		dt, subSec, offset string
		want               time.Time
		wantOffset         *time.Duration
		wantErr            bool
	}{
		{name: "empty", want: time.Time{}},
		{name: "blanked with spaces", dt: "    :  :     :  :  ", want: time.Time{}},
		{name: "zeros", dt: "0000:00:00 00:00:00", offset: "+00:00", want: time.Time{}},
		{name: "assumed UTC without an offset", dt: "2025:08:02 20:32:09", want: mustParseTime("2025-08-02T20:32:09Z")},
		{name: "negative offset", dt: "2025:08:02 20:32:09", offset: "-07:00", want: mustParseTime("2025-08-03T03:32:09Z"), wantOffset: new(-7 * time.Hour)},
		{name: "half-hour offset", dt: "2025:08:02 20:32:09", offset: "+05:30", want: mustParseTime("2025-08-02T15:02:09Z"), wantOffset: new(5*time.Hour + 30*time.Minute)},
		{name: "zero offset is kept", dt: "2025:08:02 20:32:09", offset: "+00:00", want: mustParseTime("2025-08-02T20:32:09Z"), wantOffset: new(time.Duration(0))},
		{name: "sub-seconds", dt: "2025:08:02 20:32:09", subSec: "236", want: mustParseTime("2025-08-02T20:32:09.236Z")},
		{name: "one sub-second digit", dt: "2025:08:02 20:32:09", subSec: "5", offset: "-07:00", want: mustParseTime("2025-08-03T03:32:09.5Z"), wantOffset: new(-7 * time.Hour)},
		{name: "invalid date", dt: "2025:13:02 20:32:09", wantErr: true},
		{name: "invalid sub-seconds", dt: "2025:08:02 20:32:09", subSec: "abc", wantErr: true},
		{name: "invalid offset", dt: "2025:08:02 20:32:09", offset: "-7", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOffset, err := parseTime(tt.dt, tt.subSec, tt.offset)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTime() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !got.Equal(tt.want) || got.Location() != time.UTC {
				t.Errorf("parseTime() = %v, want %v in UTC", got, tt.want)
			}
			if diff := cmp.Diff(tt.wantOffset, gotOffset); diff != "" {
				t.Errorf("parseTime() offset mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodeFile(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		path := "testdata/does-not-exist.jpg"
		_, err := DecodeFile(path)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("DecodeFile() error = %v, want errors.Is fs.ErrNotExist", err)
		}
		if err != nil && strings.Count(err.Error(), path) != 1 {
			t.Errorf("DecodeFile() error = %q, want the path exactly once", err)
		}
	})

	t.Run("parse error names the file", func(t *testing.T) {
		path := "testdata/gps_lat_only.jpg"
		_, err := DecodeFile(path)
		want := "read EXIF " + path + ": GPS has only one of latitude and longitude"
		if err == nil || err.Error() != want {
			t.Errorf("DecodeFile() error = %v, want %q", err, want)
		}
	})
}

// FuzzDecode checks that hostile input never panics and that whatever Decode
// accepts is usable: GPS within limits, no negative or non-finite settings.
func FuzzDecode(f *testing.F) {
	fixtures, err := filepath.Glob("testdata/*.jpg")
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		if p := m.GPS; p != nil {
			if p.Lat < geo.MinLat || p.Lat > geo.MaxLat || p.Lon < geo.MinLon || p.Lon > geo.MaxLon || math.IsNaN(p.Lat) || math.IsNaN(p.Lon) {
				t.Errorf("GPS %v out of range", *p)
			}
			if p.Lat == 0 && p.Lon == 0 {
				t.Error("GPS (0,0) should be nil")
			}
		}
		s := m.Settings
		for name, v := range map[string]float64{"FocalLength": s.FocalLength, "FNumber": s.FNumber} {
			if v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
				t.Errorf("%s = %v, want finite and >= 0", name, v)
			}
		}
		if s.ExposureTime < 0 {
			t.Errorf("ExposureTime = %v, want >= 0", s.ExposureTime)
		}
		if c := s.ExposureCompensation; c != nil && (math.IsInf(*c, 0) || math.IsNaN(*c)) {
			t.Errorf("ExposureCompensation = %v, want finite", *c)
		}
		if !m.Time.IsZero() && m.Time.Location() != time.UTC {
			t.Errorf("Time %v not in UTC", m.Time)
		}
	})
}

func TestTagString(t *testing.T) {
	for tg, want := range map[tag]string{tagMake: "Make", 0xABCD: "tag 0xABCD"} {
		if got := tg.String(); got != want {
			t.Errorf("tag(0x%04X).String() = %q, want %q", uint16(tg), got, want)
		}
	}
}

func TestSetErrKeepsFirst(t *testing.T) {
	var d decoder
	first := errors.New("first")
	d.setErr(first)
	d.setErr(errors.New("second"))
	d.tagErrorf(tagMake, "third")
	if !errors.Is(d.err, first) {
		t.Errorf("err = %v, want the first error", d.err)
	}
}
