// Package gpx parses GPX tracks.
//
// GPX is an XML format for GPS data, defined by the GPX 1.1 schema:
// https://www.topografix.com/GPX/1/1/. A file can hold waypoints (<wpt>),
// planned routes (<rte>), and recorded tracks (<trk>). retrace reads only
// tracks, since only a recorded track includes times:
//
//	<gpx creator="AllTrails.com">               Track.Creator
//	  <metadata><name>…</name></metadata>       Track.Name, if <trk> has none
//	  <trk>
//	    <name>…</name>                          Track.Name
//	    <trkseg>
//	      <trkpt lat="47.43" lon="-121.77">     TrackPoint.Point
//	        <ele>280.5</ele>                    TrackPoint.ElevationMeters (optional)
//	        <time>2026-09-20T15:00:00Z</time>   TrackPoint.Time (required)
//	      </trkpt>
//
// [Parse] and [ParseFile] merge all tracks and segments into one [Track],
// whose [TrackPoint] values are sorted by time.
package gpx

import (
	"cmp"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sh531/retrace/internal/geo"
)

// ErrNoTrackPoints is returned when a GPX file contains no track points.
var ErrNoTrackPoints = errors.New("no track points (routes and waypoints are not supported)")

// TrackPoint is a timestamped position recorded on a GPX track (<trkpt>).
type TrackPoint struct {
	Point           geo.Point
	Time            time.Time // UTC, from the GPX <time> element (required)
	ElevationMeters *float64  // nil when the point has no <ele>; 0 is sea level
}

// Track is a recorded GPX track, containing trackpoints in time order and optional descriptive details.
type Track struct {
	Name    string       // <trk><name>, falling back to <metadata><name>
	Creator string       // <gpx creator>, the app that exported the file
	Points  []TrackPoint // trackpoints in time order
}

// PointAt returns where the track was at time t, interpolating linearly
// between the points recorded before and after it. Elevation is nil unless
// both have one. It reports false if t is before the first point or after the
// last.
func (tr Track) PointAt(t time.Time) (TrackPoint, bool) {
	// i is the first point at or after time t.
	i, found := slices.BinarySearchFunc(tr.Points, t, func(p TrackPoint, target time.Time) int {
		return p.Time.Compare(target)
	})
	if found {
		return tr.Points[i], true
	}
	// t is before the track starts or after the track ends.
	if i == 0 || i == len(tr.Points) {
		return TrackPoint{}, false
	}

	// before.Time < t < after.Time, so the after minus before duration is never zero.
	before, after := tr.Points[i-1], tr.Points[i]
	// fraction is how far t is from before to after: 0 < fraction < 1.
	// The durations are converted to float64 so the division isn't rounded down.
	fraction := float64(t.Sub(before.Time)) / float64(after.Time.Sub(before.Time))
	tp := TrackPoint{
		Point: geo.Point{
			Lat: interpolate(before.Point.Lat, after.Point.Lat, fraction),
			Lon: interpolate(before.Point.Lon, after.Point.Lon, fraction),
		},
		Time: t,
	}
	if before.ElevationMeters != nil && after.ElevationMeters != nil {
		tp.ElevationMeters = new(interpolate(*before.ElevationMeters, *after.ElevationMeters, fraction))
	}
	return tp, true
}

// interpolate returns the value the given fraction of the way from a to b.
func interpolate(a, b, fraction float64) float64 {
	return a + (b-a)*fraction
}

// ParseFile parses the GPX file at path. See [Parse].
func ParseFile(ctx context.Context, path string) (Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return Track{}, fmt.Errorf("read GPX: %w", err) // os errors include the path
	}
	defer f.Close() //nolint:errcheck // read-only; a close error can't lose data

	track, err := Parse(ctx, f)
	if err != nil {
		return Track{}, fmt.Errorf("read GPX %s: %w", path, err)
	}
	return track, nil
}

// Parse reads a GPX document one element at a time.
// Points are merged and sorted by time; routes and waypoints are ignored.
// Every point must have lat, lon, and time; elevation is optional.
func Parse(ctx context.Context, r io.Reader) (Track, error) {
	p := parser{dec: xml.NewDecoder(r)}
	if err := p.run(ctx); err != nil {
		return Track{}, err
	}
	return p.result()
}

// parser holds the state of a single Parse call.
type parser struct {
	dec          *xml.Decoder
	track        Track
	trackName    string
	metadataName string
	openElements []string // enclosing elements not yet closed, outermost first
}

// run reads tokens until the end of the document.
func (p *parser) run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		tok, err := p.dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err // *xml.SyntaxError, which includes the line
		}

		switch tok := tok.(type) {
		case xml.StartElement:
			decoded, err := p.handleStart(tok)
			if err != nil {
				return err
			}
			// Decoded elements already consumed their end tag, so they're never pushed.
			if !decoded {
				p.openElements = append(p.openElements, tok.Name.Local)
			}
		case xml.EndElement:
			p.openElements = p.openElements[:len(p.openElements)-1]
		}
	}
}

// handleStart handles an opening tag and reports whether it decoded the whole element.
func (p *parser) handleStart(start xml.StartElement) (decoded bool, err error) {
	element, parent := start.Name.Local, p.parent()
	switch {
	case element == "gpx" && parent == "":
		p.parseGPX(start)
		return false, nil
	case element == "metadata" && parent == "gpx":
		return true, p.parseMetadata(start)
	case element == "name" && parent == "trk" && p.trackName == "":
		return true, p.parseTrackName(start)
	case element == "trkpt":
		return true, p.parseTrackPoint(start)
	}
	return false, nil
}

// parent returns the name of the innermost open element, or "" at the root.
func (p *parser) parent() string {
	if len(p.openElements) == 0 {
		return ""
	}
	return p.openElements[len(p.openElements)-1]
}

// parseGPX reads the root element's creator attribute.
func (p *parser) parseGPX(start xml.StartElement) {
	p.track.Creator = strings.TrimSpace(attr(start, "creator"))
}

// parseMetadata reads the file's name.
func (p *parser) parseMetadata(start xml.StartElement) error {
	var m rawMetadata
	if err := p.dec.DecodeElement(&m, &start); err != nil {
		return fmt.Errorf("<metadata>: %w", err)
	}
	p.metadataName = strings.TrimSpace(m.Name)
	return nil
}

// parseTrackName reads the first track's <name>.
func (p *parser) parseTrackName(start xml.StartElement) error {
	if err := p.dec.DecodeElement(&p.trackName, &start); err != nil {
		return fmt.Errorf("<trk><name>: %w", err)
	}
	p.trackName = strings.TrimSpace(p.trackName)
	return nil
}

// parseTrackPoint reads one <trkpt>. Errors name the point and its line.
func (p *parser) parseTrackPoint(start xml.StartElement) error {
	line, _ := p.dec.InputPos()
	n := len(p.track.Points) + 1

	var raw rawTrackPoint
	if err := p.dec.DecodeElement(&raw, &start); err != nil {
		return fmt.Errorf("track point %d (line %d): %w", n, line, err)
	}
	tp, err := raw.parse()
	if err != nil {
		return fmt.Errorf("track point %d (line %d): %w", n, line, err)
	}
	p.track.Points = append(p.track.Points, tp)
	return nil
}

// result finishes the track once the whole document has been read.
func (p *parser) result() (Track, error) {
	if len(p.track.Points) == 0 {
		return Track{}, ErrNoTrackPoints
	}

	// <trk><name> names the track; fallback to <metadata><name> which names the file.
	p.track.Name = cmp.Or(p.trackName, p.metadataName)

	// Tracks and segments are not guaranteed to be in time order.
	// Need to sort as interpolation will binary search by time.
	slices.SortStableFunc(p.track.Points, func(a, b TrackPoint) int {
		return a.Time.Compare(b.Time)
	})
	return p.track, nil
}

// rawMetadata holds the parts of <metadata> that Track keeps.
type rawMetadata struct {
	Name string `xml:"name"`
}

// attr returns the value of the named attribute, or "" if e doesn't have it.
func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// rawTrackPoint holds a <trkpt> as text, so missing values can be told apart
// from zero.
type rawTrackPoint struct {
	Lat  string `xml:"lat,attr"`
	Lon  string `xml:"lon,attr"`
	Ele  string `xml:"ele"`
	Time string `xml:"time"`
}

func (r rawTrackPoint) parse() (TrackPoint, error) {
	lat, err := parseCoord("lat", r.Lat, geo.MinLat, geo.MaxLat)
	if err != nil {
		return TrackPoint{}, err
	}
	lon, err := parseCoord("lon", r.Lon, geo.MinLon, geo.MaxLon)
	if err != nil {
		return TrackPoint{}, err
	}

	var ele *float64
	if s := strings.TrimSpace(r.Ele); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return TrackPoint{}, fmt.Errorf("invalid <ele> %q: %w", r.Ele, err)
		}
		// JSON can't encode NaN or infinity.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return TrackPoint{}, fmt.Errorf("invalid <ele> %q", r.Ele)
		}
		ele = &v
	}

	s := strings.TrimSpace(r.Time)
	if s == "" {
		return TrackPoint{}, errors.New("missing <time> (retrace needs a recorded activity, not a planned route)")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return TrackPoint{}, fmt.Errorf("invalid <time> %q: %w", r.Time, err)
	}

	return TrackPoint{
		Point:           geo.Point{Lat: lat, Lon: lon},
		Time:            t.UTC(),
		ElevationMeters: ele,
	}, nil
}

// parseCoord parses a lat or lon attribute and checks it is within [lo, hi].
func parseCoord(name, s string, lo, hi float64) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("missing %s", name)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, s, err)
	}
	if math.IsNaN(v) || v < lo || v > hi {
		return 0, fmt.Errorf("%s %v out of range [%v, %v]", name, v, lo, hi)
	}
	return v, nil
}
