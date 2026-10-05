#!/bin/sh
# Regenerates the EXIF test fixtures: a tiny JPEG from mkjpeg.go,
# with tags written by exiftool (an independent EXIF writer). Requires Go and exiftool to regenerate.
# Synthetic values used instead of real photos, which can be megabytes and contain GPS and serial numbers.
set -eu
cd "$(dirname "$0")"

go run mkjpeg.go > no_exif.jpg

fixture() {
	out=$1
	shift
	cp no_exif.jpg "$out"
	exiftool -q -overwrite_original "$@" "$out"
}

# Modelled on a real iPhone 13 Pro photo: big-endian, sub-seconds, timezone, GPS with altitude,
# held in portrait (Orientation 6: rotate 90° clockwise to display).
fixture apple.jpg -ExifByteOrder=Big-endian -Orientation#=6 \
	-Make=Apple -Model='iPhone 13 Pro' -LensModel='iPhone 13 Pro back triple camera 5.7mm f/1.5' \
	-DateTimeOriginal='2025:08:02 20:32:09' -SubSecTimeOriginal=236 -OffsetTimeOriginal=-07:00 \
	-ExposureTime=1/452 -FNumber=1.5 -ISO=50 -ExposureCompensation=0 -FocalLength=5.7 \
	-GPSLatitude=48.8961 -GPSLatitudeRef=N -GPSLongitude=121.6617 -GPSLongitudeRef=W \
	-GPSAltitude=1650.5 -GPSAltitudeRef#=0

# Modelled on a Lightroom-exported Sony ILCE-9 JPEG: little-endian, timezone, no GPS.
fixture sony.jpg -ExifByteOrder=Little-endian \
	-Make=SONY -Model=ILCE-9 -LensModel='FE 16-35mm F2.8 GM' \
	-DateTimeOriginal='2025:08:02 20:37:16' -OffsetTimeOriginal=-07:00 \
	-ExposureTime=1/125 -FNumber=3.2 -ISO=200 -ExposureCompensation=-0.7 -FocalLength=17

# Southern and eastern hemispheres; no OffsetTimeOriginal, so the time is assumed to be UTC.
fixture gps_south_east.jpg -ExifByteOrder=Little-endian \
	-Make=Apple -DateTimeOriginal='2025:01:15 06:00:00' \
	-GPSLatitude=33.8568 -GPSLatitudeRef=S -GPSLongitude=151.2153 -GPSLongitudeRef=E

# A position with GPSStatus V ("void") isn't a fix.
fixture gps_void.jpg -Make=Apple -GPSStatus#=V \
	-GPSLatitude=48.8961 -GPSLatitudeRef=N -GPSLongitude=121.6617 -GPSLongitudeRef=W

# Exactly (0,0) is treated as no fix.
fixture gps_zero.jpg -Make=Apple \
	-GPSLatitude=0 -GPSLatitudeRef=N -GPSLongitude=0 -GPSLongitudeRef=E

# Latitude without longitude is malformed.
fixture gps_lat_only.jpg -Make=Apple -GPSLatitude=48.8961 -GPSLatitudeRef=N
