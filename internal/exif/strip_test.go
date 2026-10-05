package exif

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// scan is a scan header and compressed image data holding an escaped 0xFF
// (0xFF 0x00) and a restart marker, which must be copied as data, and a run
// without 0xFF longer than bufio's 4096-byte buffer.
var scan = slices.Concat(segment(markerSOS, []byte{1, 2, 3}),
	[]byte{0x12, 0xFF, 0x00, 0x34, 0xFF, markerRST0}, bytes.Repeat([]byte{0x56}, 5000))

func TestStripMetadata(t *testing.T) {
	var (
		jfif  = segment(markerAPP0, []byte("JFIF\x00"))
		icc   = segment(markerAPP2, []byte("ICC_PROFILE\x00"))
		adobe = segment(markerAPP14, []byte("Adobe"))
		dqt   = segment(0xDB, []byte{0, 1})
		com   = segment(markerCOM, []byte("comment"))
		// An EXIF segment holding only Orientation 6, written out by hand.
		orientation6 = segment(markerAPP1, []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08"+
			"\x00\x01"+"\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00"+"\x00\x00\x00\x00"))
	)
	exifWith := func(entries ...entry) []byte {
		return exifSegment(tiffData(append([]entry{asciiEntry(tagMake, "Apple")}, entries...)...))
	}

	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name: "keeps only what a browser needs, in order",
			input: jpegFile(jfif, exifWith(), icc, com, adobe, dqt,
				segment(markerAPP1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>")),
				segment(0xEA, []byte("AROT\x00")),          // Apple HDR gain curve
				segment(0xED, []byte("Photoshop 3.0\x00")), // IPTC
				segment(markerAPP15, []byte("unknown")),    // any other application segment
				scan),
			want: jpegFile(jfif, icc, adobe, dqt, scan),
		},
		{
			name: "drops APP0, APP2, and APP14 with another header",
			input: jpegFile(segment(markerAPP0, []byte("JFXX\x00")),
				segment(markerAPP2, []byte("MPF\x00")), // locates appended images
				segment(markerAPP14, []byte("NotAdobe")),
				scan),
			want: jpegFile(scan),
		},
		{
			name:  "keeps only Orientation when it isn't upright",
			input: jpegFile(jfif, exifWith(shortEntry(tagOrientation, 6)), icc, scan),
			want:  jpegFile(jfif, orientation6, icc, scan),
		},
		{
			name: "keeps Orientation when another IFD is malformed",
			input: jpegFile(exifWith(shortEntry(tagOrientation, 6),
				entry{tag: tagGPSInfo, typ: typeLong, count: 1, data: be.AppendUint32(nil, 9999)}), scan),
			want: jpegFile(orientation6, scan),
		},
		{
			name:  "drops data after the image",
			input: append(jpegFile(scan), "motion photo video"...),
			want:  jpegFile(scan),
		},
		{
			name:  "keeps segments between scans",
			input: jpegFile(scan, dqt, com, scan),
			want:  jpegFile(scan, dqt, scan),
		},
		{
			name:  "drops upright Orientation",
			input: jpegFile(exifWith(shortEntry(tagOrientation, 1)), scan),
			want:  jpegFile(scan),
		},
		{
			name:  "drops invalid Orientation",
			input: jpegFile(exifWith(shortEntry(tagOrientation, 9)), scan),
			want:  jpegFile(scan),
		},
		{
			name:  "drops malformed EXIF",
			input: jpegFile(exifSegment([]byte("junk")), scan),
			want:  jpegFile(scan),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := StripMetadata(&got, bytes.NewReader(tt.input)); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got.Bytes()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestStripMetadataFixtures checks files written by exiftool: only
// Orientation is left, and the image data is unchanged.
func TestStripMetadataFixtures(t *testing.T) {
	tests := []struct {
		file string
		want Metadata
	}{
		{file: "apple.jpg", want: Metadata{Orientation: 6}},
		{file: "sony.jpg", want: Metadata{}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			var stripped bytes.Buffer
			if err := StripMetadata(&stripped, bytes.NewReader(input)); err != nil {
				t.Fatal(err)
			}

			got, err := Decode(bytes.NewReader(stripped.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("metadata mismatch (-want +got):\n%s", diff)
			}
			sos := []byte{0xFF, markerSOS}
			if !bytes.HasSuffix(stripped.Bytes(), input[bytes.Index(input, sos):]) {
				t.Error("image data changed")
			}
		})
	}
}

func TestStripMetadataErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantIs  error  // checked with errors.Is when set
		wantErr string // substring
	}{
		{name: "PNG", input: []byte("\x89PNG\r\n\x1a\n"), wantIs: ErrNotJPEG},
		{name: "no image data", input: jpegFile(segment(0xE0, []byte("JFIF\x00"))), wantErr: "JPEG has no image data"},
		{name: "ends before the image data", input: []byte{0xFF, markerSOI}, wantIs: io.ErrUnexpectedEOF},
		{name: "ends inside the image data", input: append([]byte{0xFF, markerSOI}, scan...), wantIs: io.ErrUnexpectedEOF},
		{name: "truncated payload", input: []byte{0xFF, markerSOI, 0xFF, 0xE0, 0x00, 0x10, 0x00}, wantIs: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StripMetadata(io.Discard, bytes.NewReader(tt.input))
			if err == nil {
				t.Fatal("StripMetadata() = nil, want error")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("StripMetadata() error = %v, want errors.Is %v", err, tt.wantIs)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("StripMetadata() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// FuzzStripMetadata checks that any input either fails or leaves no metadata
// but Orientation, so nothing else is published.
func FuzzStripMetadata(f *testing.F) {
	addFixtures(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		var stripped bytes.Buffer
		if err := StripMetadata(&stripped, bytes.NewReader(data)); err != nil {
			return
		}
		m, err := Decode(bytes.NewReader(stripped.Bytes()))
		if err != nil {
			t.Fatalf("decoding the stripped JPEG: %v", err)
		}
		if diff := cmp.Diff(Metadata{Orientation: m.Orientation}, m); diff != "" {
			t.Errorf("metadata left (-want +got):\n%s", diff)
		}
	})
}
