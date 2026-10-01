package exif

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sh531/retrace/internal/geo"
)

// ErrNotJPEG is returned when a file doesn't start with a JPEG marker.
var ErrNotJPEG = errors.New("not a JPEG file (HEIC and RAW aren't supported; export as JPEG)")

// Camera is the device a photo was taken with.
type Camera struct {
	Make, Model string
}

// Settings are the camera settings a photo was taken with. Zero means unknown.
type Settings struct {
	FocalLength          float64 // mm, actual (not 35mm-equivalent)
	FNumber              float64
	ISO                  int
	ExposureTime         time.Duration
	ExposureCompensation *float64 // EV; nil when unknown, since 0 is common
}

// Metadata is what a photo's EXIF says, before retrace corrects anything.
type Metadata struct {
	Camera      Camera
	Lens        string
	Settings    Settings
	Orientation int            // how to rotate or flip the pixels for display, 1–8 as in EXIF spec; 0 (absent or invalid) and 1 both mean upright
	Time        time.Time      // DateTimeOriginal converted to UTC using OffsetTimeOriginal, or assumed to be UTC without it; zero when missing
	TimeOffset  *time.Duration // OffsetTimeOriginal; nil when absent, since 0 is a real offset
	GPS         *geo.Point     // nil when absent, void (GPSStatus V), or exactly (0,0)
}

// DecodeFile decodes the EXIF metadata of the JPEG file at path. See [Decode].
func DecodeFile(path string) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("read EXIF: %w", err) // os errors include the path
	}
	defer f.Close() //nolint:errcheck // read-only; a close error can't lose data

	m, err := Decode(f)
	if err != nil {
		return Metadata{}, fmt.Errorf("read EXIF %s: %w", path, err)
	}
	return m, nil
}

// Decode decodes the EXIF metadata of a JPEG. It stops before the image data.
// A JPEG without EXIF returns zero Metadata and no error.
func Decode(r io.Reader) (Metadata, error) {
	data, err := findEXIF(bufio.NewReader(r))
	if err != nil || data == nil {
		return Metadata{}, err
	}
	return decodeTIFF(data)
}

// decodeTIFF decodes the TIFF-format data in an EXIF APP1 segment: the
// payload after "Exif\0\0".
func decodeTIFF(data []byte) (Metadata, error) {
	d := decoder{data: data}
	ifd0 := d.ifd(d.header())

	m := Metadata{
		Camera: Camera{Make: d.ascii(ifd0, tagMake), Model: d.ascii(ifd0, tagModel)},
	}
	if v, ok := d.unsigned(ifd0, tagOrientation); ok && v >= 1 && v <= 8 {
		m.Orientation = int(v)
	}
	// ExifOffset and GPSInfo each hold the offset of a sub-IFD.
	if off, ok := d.unsigned(ifd0, tagExifOffset); ok {
		d.exif(d.ifd(off), &m)
	}
	if off, ok := d.unsigned(ifd0, tagGPSInfo); ok {
		m.GPS = d.gps(d.ifd(off))
	}

	if d.err != nil {
		return Metadata{}, d.err
	}
	return m, nil
}

// exif reads the Exif IFD into m.
func (d *decoder) exif(dir ifd, m *Metadata) {
	m.Lens = d.ascii(dir, tagLensModel)

	dt, subSec, offset := d.ascii(dir, tagDateTimeOriginal), d.ascii(dir, tagSubSecTimeOriginal), d.ascii(dir, tagOffsetTimeOriginal)
	if d.err == nil {
		var err error
		if m.Time, m.TimeOffset, err = parseTime(dt, subSec, offset); err != nil {
			d.setErr(err)
		}
	}

	s := &m.Settings
	if r := d.rationals(dir, tagFocalLength, 1); r != nil {
		s.FocalLength = positive(r[0])
	}
	if r := d.rationals(dir, tagFNumber, 1); r != nil {
		s.FNumber = positive(r[0])
	}
	if iso, ok := d.unsigned(dir, tagISO); ok {
		s.ISO = int(iso)
	}
	if r := d.rationals(dir, tagExposureTime, 1); r != nil && r[0].num > 0 && r[0].den > 0 {
		// Fits in int64: num < 2^32, so num * 1e9 < 2^63.
		s.ExposureTime = time.Duration(r[0].num * int64(time.Second) / r[0].den)
	}
	if r := d.rationals(dir, tagExposureCompensation, 1); r != nil && r[0].den != 0 {
		ev := float64(r[0].num) / float64(r[0].den)
		s.ExposureCompensation = &ev
	}
}

// gps reads the GPS IFD. It returns nil when there's no usable fix.
func (d *decoder) gps(dir ifd) *geo.Point {
	if d.ascii(dir, tagGPSStatus) == "V" {
		return nil // "void": the receiver had no fix
	}
	lat, hasLat := d.coord(dir, tagGPSLatitudeRef, tagGPSLatitude, "N", "S")
	lon, hasLon := d.coord(dir, tagGPSLongitudeRef, tagGPSLongitude, "E", "W")
	switch {
	case d.err != nil || !hasLat && !hasLon:
		return nil
	case hasLat != hasLon:
		d.setErr(errors.New("GPS has only one of latitude and longitude"))
		return nil
	case lat == 0 && lon == 0:
		return nil // some devices write zeros without a fix
	case lat < geo.MinLat || lat > geo.MaxLat || lon < geo.MinLon || lon > geo.MaxLon:
		d.setErr(fmt.Errorf("GPS position (%v, %v) out of range", lat, lon))
		return nil
	}
	return &geo.Point{Lat: lat, Lon: lon}
}

// coord reads a GPS latitude or longitude, stored as degrees, minutes, and
// seconds plus a reference that gives its sign.
func (d *decoder) coord(dir ifd, refTag, valTag tag, pos, neg string) (float64, bool) {
	dms := d.rationals(dir, valTag, 3)
	if dms == nil {
		return 0, false
	}
	var v float64
	for i, scale := range []float64{1, 60, 3600} {
		if dms[i].den == 0 {
			d.tagErrorf(valTag, "zero denominator")
			return 0, false
		}
		v += float64(dms[i].num) / float64(dms[i].den) / scale
	}
	switch ref := d.ascii(dir, refTag); ref {
	case pos:
	case neg:
		v = -v
	default:
		d.tagErrorf(refTag, "got %q, want %q or %q", ref, pos, neg)
		return 0, false
	}
	return v, true
}

// positive returns r as a float64, or 0 (unknown) if r isn't positive.
// It checks before dividing because a zero denominator would give Inf or
// NaN, which encoding/json can't encode. Negative values only come from
// malformed files.
func positive(r rational) float64 {
	if r.num <= 0 || r.den <= 0 {
		return 0
	}
	return float64(r.num) / float64(r.den)
}

// exifTime is the layout of EXIF date-time strings.
const exifTime = "2006:01:02 15:04:05"

// parseTime combines DateTimeOriginal with its optional sub-seconds and
// converts it to UTC using offset, or assumes it is UTC if offset is empty.
// It returns the zero time if dt is blank.
func parseTime(dt, subSec, offset string) (time.Time, *time.Duration, error) {
	// The spec allows unknown dates to be blanked with spaces; zeros are common too.
	if strings.Trim(dt, " :0") == "" {
		return time.Time{}, nil, nil
	}

	// Parse accepts a fractional second after the seconds even when the layout
	// has none, and returns UTC when there's no zone (time/format.go).
	s, layout := dt, exifTime
	if subSec != "" {
		s += "." + subSec
	}
	if offset != "" {
		s += offset
		layout += "-07:00"
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("invalid time (DateTimeOriginal %q, SubSecTimeOriginal %q, OffsetTimeOriginal %q): %w", dt, subSec, offset, err)
	}

	var off *time.Duration
	if offset != "" {
		_, secs := t.Zone()
		o := time.Duration(secs) * time.Second
		off = &o
	}
	return t.UTC(), off, nil
}
