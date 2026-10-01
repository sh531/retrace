//go:build ignore

// mkjpeg writes a tiny JPEG with no metadata to stdout. gen.sh saves it as
// no_exif.jpg and builds the other fixtures from copies of it.
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
)

func main() {
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 4)
	}
	img.Set(0, 0, color.Gray{Y: 255})
	if err := jpeg.Encode(os.Stdout, img, nil); err != nil {
		log.Fatal(err)
	}
}
