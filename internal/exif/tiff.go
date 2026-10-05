package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// fieldType is the type of an IFD entry's values (TIFF 6.0, section 2).
type fieldType uint16

// Field types that retrace reads.
const (
	typeByte      fieldType = 1
	typeASCII     fieldType = 2
	typeShort     fieldType = 3
	typeLong      fieldType = 4
	typeRational  fieldType = 5
	typeSRational fieldType = 10
	typeIFD       fieldType = 13 // a LONG offset to a sub-IFD (TIFF Technical Note 1)
)

// fieldTypes holds each type's name and the size in bytes of one value.
var fieldTypes = map[fieldType]struct {
	name string
	size uint64
}{
	typeByte:      {"BYTE", 1},
	typeASCII:     {"ASCII", 1},
	typeShort:     {"SHORT", 2},
	typeLong:      {"LONG", 4},
	typeRational:  {"RATIONAL", 8},
	typeSRational: {"SRATIONAL", 8},
	typeIFD:       {"IFD", 4},
}

// String returns the type's name, e.g. "SHORT" for 3, or just its number
// for a type retrace doesn't read, e.g. "99".
// Implements fmt.Stringer.
func (t fieldType) String() string {
	if ft, ok := fieldTypes[t]; ok {
		return ft.name
	}
	return strconv.Itoa(int(t))
}

// field is an IFD entry. Its values are located only when read, so entries
// retrace ignores can't cause errors.
type field struct {
	typ   fieldType
	count uint32
	value []byte // the 4-byte value-or-offset
}

// ifd maps tags to the fields of one image file directory.
type ifd map[tag]field

// rational is a RATIONAL (2 unsigned 32-bit ints: numerator, denominator)
// or SRATIONAL (2 signed 32-bit ints: numerator, denominator).
type rational struct {
	num, den int64
}

// decoder reads TIFF-structured EXIF data. It keeps the first error and
// turns later calls into no-ops, so decodeTIFF can read fields without
// checking each one (see go.dev/blog/errors-are-values).
type decoder struct {
	data  []byte
	order binary.ByteOrder
	err   error
}

// header reads the TIFF header: byte order, magic number, and IFD0's offset.
func (d *decoder) header() uint32 {
	if len(d.data) < 8 {
		d.setErr(errors.New("TIFF header too short"))
		return 0
	}
	switch string(d.data[:2]) {
	case "II":
		d.order = binary.LittleEndian
	case "MM":
		d.order = binary.BigEndian
	default:
		d.setErr(fmt.Errorf("invalid byte order %q", d.data[:2]))
		return 0
	}
	if magic := d.order.Uint16(d.data[2:]); magic != 42 {
		d.setErr(fmt.Errorf("invalid TIFF magic number %d", magic))
		return 0
	}
	return d.order.Uint32(d.data[4:])
}

// ifd reads the directory at off (its position in bytes from the start of the
// TIFF header). Each entry is 12 bytes: tag, type, count, and a value-or-offset.
func (d *decoder) ifd(off uint32) ifd {
	if d.err != nil {
		return nil
	}
	start := uint64(off) + 2 // entries start after the 2-byte entry count
	if start > uint64(len(d.data)) {
		d.setErr(fmt.Errorf("IFD offset %d out of range", off))
		return nil
	}
	n := uint64(d.order.Uint16(d.data[off:])) // entry count
	if start+12*n > uint64(len(d.data)) {
		d.setErr(fmt.Errorf("IFD at offset %d: %d entries run past the end", off, n))
		return nil
	}

	dir := make(ifd, n)
	for i := range n {
		e := d.data[start+12*i:]
		dir[tag(d.order.Uint16(e))] = field{
			typ:   fieldType(d.order.Uint16(e[2:])),
			count: d.order.Uint32(e[4:]),
			value: e[8:12],
		}
	}
	return dir
}

// lookup returns dir's field for t if its type is one of want. A missing
// field, or an earlier error, returns false; a field of another type is an error.
func (d *decoder) lookup(dir ifd, t tag, want ...fieldType) (field, bool) {
	f, ok := dir[t]
	if !ok || d.err != nil {
		return field{}, false
	}
	if !slices.Contains(want, f.typ) {
		names := make([]string, len(want))
		for i, w := range want {
			names[i] = w.String()
		}
		d.tagErrorf(t, "type %v, want %s", f.typ, strings.Join(names, " or "))
		return field{}, false
	}
	return f, true
}

// values returns the bytes of f's values: inline if they fit in 4 bytes,
// otherwise at the offset f holds. It returns nil if they run past the end.
func (d *decoder) values(t tag, f field) []byte {
	// lookup only returns fields of known types, so f.typ is in fieldTypes.
	n := uint64(f.count) * fieldTypes[f.typ].size // can't overflow: count < 2^32, size <= 8
	if n <= 4 {
		return f.value[:n]
	}
	off := uint64(d.order.Uint32(f.value)) // offset to the values
	if off+n > uint64(len(d.data)) {
		d.tagErrorf(t, "%d bytes at offset %d run past the end", n, off)
		return nil
	}
	return d.data[off : off+n]
}

// ascii returns the text of an ASCII field, or "" if dir doesn't have it.
// TIFF strings end with a NUL (zero) byte, so the text is cut there and trimmed.
func (d *decoder) ascii(dir ifd, t tag) string {
	f, ok := d.lookup(dir, t, typeASCII)
	if !ok {
		return ""
	}
	b := d.values(t, f)
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// unsigned returns the first value of a BYTE (8-bit), SHORT (16-bit), or
// LONG (32-bit) field, and whether dir has it.
func (d *decoder) unsigned(dir ifd, t tag) (uint32, bool) {
	f, ok := d.lookup(dir, t, typeByte, typeShort, typeLong, typeIFD)
	if !ok {
		return 0, false
	}
	if f.count == 0 {
		d.tagErrorf(t, "no values")
		return 0, false
	}
	b := d.values(t, f) // several values may not fit inline
	if b == nil {
		return 0, false
	}
	switch f.typ {
	case typeByte:
		return uint32(b[0]), true
	case typeShort:
		return uint32(d.order.Uint16(b)), true
	}
	return d.order.Uint32(b), true
}

// rationals returns the first n values of a RATIONAL or SRATIONAL field, or
// nil if dir doesn't have it.
func (d *decoder) rationals(dir ifd, t tag, n uint32) []rational {
	f, ok := d.lookup(dir, t, typeRational, typeSRational)
	if !ok {
		return nil
	}
	if f.count < n {
		d.tagErrorf(t, "%d values, want %d", f.count, n)
		return nil
	}
	b := d.values(t, f)
	if b == nil {
		return nil
	}
	rs := make([]rational, n)
	for i := range rs {
		num, den := d.order.Uint32(b[8*i:]), d.order.Uint32(b[8*i+4:])
		if f.typ == typeSRational {
			//nolint:gosec // SRATIONAL is two's complement; reinterpreting the bits is intended
			rs[i] = rational{int64(int32(num)), int64(int32(den))}
		} else {
			rs[i] = rational{int64(num), int64(den)}
		}
	}
	return rs
}

// setErr records err unless an error is already recorded.
func (d *decoder) setErr(err error) {
	if d.err == nil {
		d.err = err
	}
}

// tagErrorf records an error about tag t, named by its String method.
func (d *decoder) tagErrorf(t tag, format string, args ...any) {
	d.setErr(fmt.Errorf("%v: %s", t, fmt.Sprintf(format, args...)))
}
