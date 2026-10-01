// Package exif reads camera metadata from the EXIF block of JPEG files.
//
// [Decode] and [DecodeFile] return a [Metadata]: camera, lens, settings,
// orientation, the time the photo was taken (in UTC), and GPS position.
// They read only the tags retrace uses and stop before the image data.
//
// # Layout
//
// EXIF reuses the structure of the TIFF image format. Its values are
// stored in image file directories (IFDs): tables of entries, each a tag
// such as Make or ExposureTime with its value. IFD0 is the first directory;
// some of its entries point to further directories, such as the Exif IFD.
//
// EXIF data is nested three layers deep, and each file in this package
// handles one layer:
//
//	JPEG ── APP1 "Exif\0\0" ── TIFF header ── IFD0 ──┬── ExifOffset → Exif IFD
//	                                                 └── GPSInfo    → GPS IFD
//
// From the outside in:
//
//   - jpeg.go: a JPEG is a series of segments, each starting with a 0xFF
//     marker byte. EXIF lives in the APP1 segment (0xFFE1) whose payload
//     starts with "Exif\0\0". XMP uses APP1 too, with a different header.
//   - tiff.go: the rest of that payload uses TIFF's structure. An 8-byte
//     header gives the byte order ("II" little-endian or "MM" big-endian),
//     the magic number 42, and the offset of IFD0. Offsets count from the
//     start of this header.
//   - exif.go, tags.go: what the tags mean. IFD0 holds Make, Model, and
//     Orientation, plus pointers to the Exif IFD (times, exposure, lens)
//     and the GPS IFD (latitude, longitude, status).
//
// # Image file directories
//
// An IFD is a 2-byte entry count followed by 12-byte entries:
//
//	bytes 0–1   tag      which value, e.g. 0x010F Make
//	bytes 2–3   type     how to read it, e.g. 2 ASCII, 3 SHORT, 5 RATIONAL
//	bytes 4–7   count    how many values of that type
//	bytes 8–11  value    the values if they fit in 4 bytes, else their offset
//
// For example, this entry in a big-endian file is Make:
//
//	01 0f | 00 02 | 00 00 00 06 | 00 00 00 56
//
// Tag 0x010F (Make), type 2 (ASCII), 6 values: "Apple" plus a NUL
// terminator. Six bytes don't fit in the entry, so the last field is the
// offset where they are stored, 0x56.
//
// Every offset comes from the file, so each one is checked against the
// data before it is followed, using 64-bit arithmetic so that a large
// count or offset can't wrap around. Only the Exif and GPS pointers are
// followed, so a file can't make the decoder loop.
//
// # References
//
//   - Exif spec (CIPA DC-008), which defines the tags and types:
//     https://www.cipa.jp/e/std/std-sec.html
//   - exiftool's tag tables, which list the same IDs under the names used
//     here: https://exiftool.org/TagNames/EXIF.html and
//     https://exiftool.org/TagNames/GPS.html
package exif
