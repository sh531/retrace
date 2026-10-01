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
	markerTEM  = 0x01 // no length or payload
	markerRST0 = 0xD0 // restart markers RST0–RST7; no length or payload
	markerRST7 = 0xD7
	markerSOI  = 0xD8 // start of image: the first two bytes of every JPEG
	markerEOI  = 0xD9 // end of image
	markerSOS  = 0xDA // start of scan: compressed image data follows
	markerAPP1 = 0xE1 // application segment 1: EXIF or XMP
)

// exifHeader starts an APP1 segment holding EXIF; XMP uses APP1 too.
var exifHeader = []byte("Exif\x00\x00")

// findEXIF returns the TIFF data in the first EXIF APP1 segment, or nil if
// the image data starts without one. A JPEG is a series of segments: a 0xFF
// marker, then for most markers a 2-byte big-endian length (including
// itself) and a payload.
func findEXIF(r *bufio.Reader) ([]byte, error) {
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil || soi != [2]byte{0xFF, markerSOI} {
		return nil, ErrNotJPEG
	}

	for {
		marker, err := readMarker(r)
		if err != nil {
			return nil, err
		}
		switch {
		case marker == markerSOS || marker == markerEOI:
			return nil, nil // metadata comes before the image data
		case marker == markerTEM || marker >= markerRST0 && marker <= markerRST7:
			continue // no length or payload
		}

		var size [2]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return nil, truncated(err)
		}
		n := int(binary.BigEndian.Uint16(size[:])) - 2 // JPEG is always big-endian; the length includes itself
		if n < 0 {
			return nil, fmt.Errorf("JPEG segment 0x%02X: invalid length %d", marker, n+2)
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

// truncated reports an early end of file as io.ErrUnexpectedEOF.
func truncated(err error) error {
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("truncated JPEG: %w", err)
}
