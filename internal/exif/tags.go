package exif

import "fmt"

// tag identifies an IFD (image file directory) entry. IDs and types are
// defined by the Exif spec (CIPA DC-008, https://www.cipa.jp/e/std/std-sec.html).
// Names follow exiftool's tag tables (https://exiftool.org/TagNames/EXIF.html,
// .../GPS.html), which list the same IDs under the names users see in
// exiftool's output. Some Exif spec names are less readable compared to exiftool's
// names (PhotographicSensitivity vs ISO). Constant = "tag" + name.
type tag uint16

const (
	// IFD0
	tagMake        tag = 0x010F
	tagModel       tag = 0x0110
	tagOrientation tag = 0x0112
	tagExifOffset  tag = 0x8769
	tagGPSInfo     tag = 0x8825

	// Exif IFD
	tagExposureTime         tag = 0x829A
	tagFNumber              tag = 0x829D
	tagISO                  tag = 0x8827
	tagDateTimeOriginal     tag = 0x9003
	tagOffsetTimeOriginal   tag = 0x9011
	tagExposureCompensation tag = 0x9204
	tagFocalLength          tag = 0x920A
	tagSubSecTimeOriginal   tag = 0x9291
	tagLensModel            tag = 0xA434

	// GPS IFD
	tagGPSLatitudeRef  tag = 0x0001
	tagGPSLatitude     tag = 0x0002
	tagGPSLongitudeRef tag = 0x0003
	tagGPSLongitude    tag = 0x0004
	tagGPSAltitudeRef  tag = 0x0005
	tagGPSAltitude     tag = 0x0006
	tagGPSStatus       tag = 0x0009
)

var tagNames = map[tag]string{
	tagMake:                 "Make",
	tagModel:                "Model",
	tagOrientation:          "Orientation",
	tagExifOffset:           "ExifOffset",
	tagGPSInfo:              "GPSInfo",
	tagExposureTime:         "ExposureTime",
	tagFNumber:              "FNumber",
	tagISO:                  "ISO",
	tagDateTimeOriginal:     "DateTimeOriginal",
	tagOffsetTimeOriginal:   "OffsetTimeOriginal",
	tagExposureCompensation: "ExposureCompensation",
	tagFocalLength:          "FocalLength",
	tagSubSecTimeOriginal:   "SubSecTimeOriginal",
	tagLensModel:            "LensModel",
	tagGPSLatitudeRef:       "GPSLatitudeRef",
	tagGPSLatitude:          "GPSLatitude",
	tagGPSLongitudeRef:      "GPSLongitudeRef",
	tagGPSLongitude:         "GPSLongitude",
	tagGPSAltitudeRef:       "GPSAltitudeRef",
	tagGPSAltitude:          "GPSAltitude",
	tagGPSStatus:            "GPSStatus",
}

// String returns the name of the tag, or its hex value if unknown.
// Implements fmt.Stringer.
func (t tag) String() string {
	if name, ok := tagNames[t]; ok {
		return name
	}
	return fmt.Sprintf("tag 0x%04X", uint16(t))
}
