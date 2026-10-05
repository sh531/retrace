package exif

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// StripMetadata copies the JPEG from src to dst without personal information,
// so the photo can be published. It keeps only what a browser needs to display
// the image: the image data, the colour profile, and, if the photo isn't
// upright, Orientation. It drops all other metadata, comments, and anything
// appended to the image, such as a motion photo's video.
func StripMetadata(dst io.Writer, src io.Reader) error {
	r := bufio.NewReader(src)
	if err := readSOI(r); err != nil {
		return err
	}
	w := bufio.NewWriter(dst) // keeps the first write error for Flush to return
	w.Write([]byte{0xFF, markerSOI})

	scanned := false
	marker, err := readMarker(r)
	for err == nil {
		switch {
		case marker == markerEOI && !scanned:
			return errors.New("JPEG has no image data")
		case marker == markerEOI:
			w.Write([]byte{0xFF, markerEOI})
			return w.Flush()
		case !hasPayload(marker):
			w.Write([]byte{0xFF, marker})
		default:
			if err = copySegment(w, r, marker); err != nil {
				return err
			}
		}

		if marker == markerSOS {
			scanned = true
			marker, err = copyScan(w, r)
		} else {
			marker, err = readMarker(r)
		}
	}
	return err
}

// displayAPPs are the application segments a browser needs to display an
// image, by marker and the header their payload starts with.
var displayAPPs = map[byte]string{
	markerAPP0:  "JFIF\x00",        // colour format, for decoders that need it
	markerAPP2:  "ICC_PROFILE\x00", // colour profile
	markerAPP14: "Adobe",           // colour transform
}

// copySegment copies the segment after marker if a browser needs it to
// display the image, replacing EXIF with only its Orientation.
func copySegment(w *bufio.Writer, r *bufio.Reader, marker byte) error {
	n, err := readLength(r, marker)
	if err != nil {
		return err
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return truncated(err)
	}

	switch {
	case marker == markerAPP1:
		v := orientation(payload)
		if v < 2 {
			return nil // XMP, or EXIF with nothing to keep
		}
		payload = orientationEXIF(v)
	case marker == markerCOM:
		return nil
	case marker >= markerAPP0 && marker <= markerAPP15:
		header, ok := displayAPPs[marker]
		if !ok || !bytes.HasPrefix(payload, []byte(header)) {
			return nil // metadata a browser doesn't need to display the image
		}
	}
	w.Write([]byte{0xFF, marker})
	w.Write(binary.BigEndian.AppendUint16(nil, uint16(len(payload)+2))) //nolint:gosec // G115: payloads are read from a segment or built by orientationEXIF, so len+2 fits in 16 bits
	w.Write(payload)
	return nil
}

// copyScan copies compressed image data, which follows an SOS segment, and
// returns the marker that ends it. Inside the data, 0xFF is followed by 0x00
// (an escaped 0xFF byte) or a restart marker, which are copied.
func copyScan(w *bufio.Writer, r *bufio.Reader) (byte, error) {
	for {
		data, err := r.ReadSlice(0xFF)
		if errors.Is(err, bufio.ErrBufferFull) {
			w.Write(data)
			continue
		}
		if err != nil {
			return 0, truncated(err)
		}
		w.Write(data[:len(data)-1])
		_ = r.UnreadByte() // puts back the 0xFF for readMarker; can't fail right after ReadSlice
		marker, err := readMarker(r)
		if err != nil {
			return 0, err
		}
		if marker != 0x00 && hasPayload(marker) {
			return marker, nil
		}
		w.Write([]byte{0xFF, marker}) // an escaped 0xFF or a restart marker
	}
}

// orientation returns the Orientation in an APP1 payload, or 0 if the
// payload isn't EXIF, or Orientation is missing, invalid, or malformed. It
// reads IFD0 only, so errors in other tags don't hide it.
func orientation(payload []byte) uint16 {
	data, ok := bytes.CutPrefix(payload, exifHeader)
	if !ok {
		return 0
	}
	d := decoder{data: data}
	v, ok := d.unsigned(d.ifd(d.header()), tagOrientation)
	if d.err != nil || !ok || v > 8 {
		return 0
	}
	return uint16(v)
}

// orientationEXIF returns an EXIF APP1 payload holding only Orientation v:
// a big-endian TIFF header, then IFD0 with one SHORT entry and no next IFD.
func orientationEXIF(v uint16) []byte {
	bigEndian := binary.BigEndian
	b := append(bytes.Clone(exifHeader), "MM\x00\x2a\x00\x00\x00\x08"...) // IFD0 at offset 8
	b = bigEndian.AppendUint16(b, 1)                                      // entry count
	b = bigEndian.AppendUint16(b, uint16(tagOrientation))
	b = bigEndian.AppendUint16(b, uint16(typeShort))
	b = bigEndian.AppendUint32(b, 1) // value count
	b = bigEndian.AppendUint16(b, v)
	b = append(b, 0, 0)                 // pads the value to 4 bytes
	return bigEndian.AppendUint32(b, 0) // offset of the next IFD: none
}
