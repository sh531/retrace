package locate

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sh531/retrace/internal/exif"
)

// Offsets holds a clock correction per camera. Each is set from a reference
// photo taken with that camera, and applies to every photo with the same EXIF
// Make and Model. Identifying the camera by a photo means it never has to be
// spelled out, and makes such as "NIKON CORPORATION" or models that repeat
// the make need no special handling. Two bodies of the same model can't be
// told apart. The zero value is empty and ready to use.
//
// *Offsets satisfies flag.Value, so -offset can be repeated, once per camera.
type Offsets struct {
	byCamera map[exif.Camera]offset
}

// offset is a clock correction and the reference photo it was set from.
type offset struct {
	duration       time.Duration
	referencePhoto string // path of the photo the offset was set from
}

// Set adds an offset written as photo=duration, e.g. "DSC00042.jpg=2m30s":
// every photo from the same camera as DSC00042.jpg gets 2m30s added. The
// duration uses [time.ParseDuration] syntax.
func (o *Offsets) Set(s string) error {
	// Split at the last "=": paths can contain "=", durations can't.
	i := strings.LastIndex(s, "=")
	if i < 0 {
		return errors.New("want photo=duration, e.g. DSC00042.jpg=2m30s")
	}
	path, dur := s[:i], s[i+1:]
	if path == "" {
		return errors.New("missing photo before =")
	}
	d, err := time.ParseDuration(strings.TrimSpace(dur))
	if err != nil {
		return err // quotes the duration, e.g. time: unknown unit "x" in duration "2x"
	}

	m, err := exif.DecodeFile(path)
	if err != nil {
		return err // names the path
	}
	c := m.Camera
	if c == (exif.Camera{}) {
		return fmt.Errorf("%s has neither a camera make nor a model in its EXIF, so it can't identify a camera", path)
	}
	if existing, ok := o.byCamera[c]; ok {
		if existing.referencePhoto == path {
			return fmt.Errorf("%s given twice", path)
		}
		return fmt.Errorf("%s is from the same camera (%s) as %s", path, c, existing.referencePhoto)
	}

	if o.byCamera == nil {
		o.byCamera = make(map[exif.Camera]offset)
	}
	o.byCamera[c] = offset{duration: d, referencePhoto: path}
	return nil
}

// String returns the offsets as referencePhoto=duration, sorted by
// referencePhoto and comma-separated, e.g. "DSC00042.jpg=2m30s,IMG_0001.jpg=0s".
// A nil o gives "", since the flag package may call String on a nil pointer.
func (o *Offsets) String() string {
	if o == nil {
		return ""
	}
	var pairs []string
	for _, entry := range o.byCamera {
		pairs = append(pairs, fmt.Sprintf("%s=%v", entry.referencePhoto, entry.duration))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, ",")
}

// For returns the offset for camera c, or 0 if none was set.
func (o *Offsets) For(c exif.Camera) time.Duration {
	return o.byCamera[c].duration
}
