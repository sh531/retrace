package exif

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// JPEG markers (ITU-T T.81, table B.1). Each segment starts with 0xFF and one of these.
const (
	markerTEM   = 0x01 // no length or payload
	markerRST0  = 0xD0 // first restart marker: RST0–RST7 sit inside the image data, with no length or payload
	markerRST7  = 0xD7 // last restart marker
	markerSOI   = 0xD8 // start of image: the first two bytes of every JPEG
	markerEOI   = 0xD9 // end of image
	markerSOS   = 0xDA // start of scan: compressed image data follows
	markerAPP0  = 0xE0 // first application segment: APP0–APP15 hold metadata, each starting with a header naming its format; APP0 holds JFIF
	markerAPP1  = 0xE1 // EXIF or XMP
	markerAPP2  = 0xE2 // ICC colour profile, among others
	markerAPP14 = 0xEE // Adobe colour transform
	markerAPP15 = 0xEF // last application segment
	markerCOM   = 0xFE // comment
)

// exifHeader starts an APP1 segment holding EXIF; XMP uses APP1 too.
var exifHeader = []byte("Exif\x00\x00")

// findEXIF returns the TIFF data in the first EXIF APP1 segment, or nil if
// the image data starts without one. A JPEG is a series of segments: a 0xFF
// marker, then for most markers a 2-byte big-endian length (including
// itself) and a payload.
func findEXIF(r *bufio.Reader) ([]byte, error) {
	if err := readSOI(r); err != nil {
		return nil, err
	}

	for {
		marker, err := readMarker(r)
		if err != nil {
			return nil, err
		}
		switch {
		case marker == markerSOS || marker == markerEOI:
			return nil, nil // metadata comes before the image data
		case !hasPayload(marker):
			continue
		}

		n, err := readLength(r, marker)
		if err != nil {
			return nil, err
		}
		if marker != markerAPP1 {
			if _, err := r.Discard(n); err != nil {
				return nil, truncated(err)
			}
			continue
		}
		seg := make([]byte, n)
		if _, err := io.ReadFull(r, seg); err != nil {
			return nil, truncated(err)
		}
		if data, ok := bytes.CutPrefix(seg, exifHeader); ok {
			return data, nil
		}
	}
}

// readSOI reads the start of image marker that every JPEG begins with.
func readSOI(r *bufio.Reader) error {
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil || soi != [2]byte{0xFF, markerSOI} {
		return ErrNotJPEG
	}
	return nil
}

// readMarker reads a 0xFF byte, skips any 0xFF fill bytes, and returns the marker.
func readMarker(r *bufio.Reader) (byte, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, truncated(err)
	}
	if b != 0xFF {
		return 0, fmt.Errorf("want JPEG marker, got byte 0x%02X", b)
	}
	for b == 0xFF {
		if b, err = r.ReadByte(); err != nil {
			return 0, truncated(err)
		}
	}
	return b, nil
}

// hasPayload reports whether a segment with marker has a length and payload.
func hasPayload(marker byte) bool {
	return marker != markerTEM && (marker < markerRST0 || marker > markerRST7)
}

// readLength reads a segment's 2-byte big-endian length, which includes
// itself, and returns the length of the payload that follows.
func readLength(r *bufio.Reader, marker byte) (int, error) {
	var size [2]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return 0, truncated(err)
	}
	n := int(binary.BigEndian.Uint16(size[:])) - 2
	if n < 0 {
		return 0, fmt.Errorf("JPEG segment 0x%02X: invalid length %d", marker, n+2)
	}
	return n, nil
}

// truncated reports an early end of file as io.ErrUnexpectedEOF.
func truncated(err error) error {
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("truncated JPEG: %w", err)
}
