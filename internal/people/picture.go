// SPDX-License-Identifier: Apache-2.0

package people

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif"  // decoders for uploaded pictures
	_ "image/jpeg" // decoders for uploaded pictures
	"image/png"

	"github.com/diffluence/diffluence/internal/apperr"
)

const (
	pictureSide   = 256
	maxMegapixels = 40_000_000
)

// reencode makes a clean PNG from a crop of an upload. Decoding and drawing again
// drops everything but the pixels, such as location data (spec 013, D7).
func reencode(data []byte, crop Crop) ([]byte, error) {
	bad := apperr.Invalid("Upload a PNG, JPEG or GIF picture.")
	size, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, bad
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, bad
	}
	// The size comes from the header, before any memory is spent on the pixels.
	if size.Width*size.Height > maxMegapixels {
		return nil, apperr.TooLarge("A picture can have at most 40 megapixels.")
	}
	if crop.Size == 0 {
		crop = Crop{Size: min(size.Width, size.Height)}
	}
	if crop.X < 0 || crop.Y < 0 || crop.Size < 1 || crop.X+crop.Size > size.Width || crop.Y+crop.Size > size.Height {
		return nil, apperr.Invalid("The crop does not fit the picture.")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, bad
	}
	var out bytes.Buffer
	if err := png.Encode(&out, scaled(img, crop)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// scaled draws the crop on a fixed square. Each new pixel is the average of the
// source pixels below it, which keeps small text and lines readable.
func scaled(img image.Image, crop Crop) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, pictureSide, pictureSide))
	origin := img.Bounds().Min
	for y := range pictureSide {
		top := crop.Y + y*crop.Size/pictureSide
		bottom := max(crop.Y+(y+1)*crop.Size/pictureSide, top+1)
		for x := range pictureSide {
			left := crop.X + x*crop.Size/pictureSide
			right := max(crop.X+(x+1)*crop.Size/pictureSide, left+1)
			out.SetNRGBA(x, y, averageOf(img, origin, image.Rect(left, top, right, bottom)))
		}
	}
	return out
}

func averageOf(img image.Image, origin image.Point, area image.Rectangle) color.NRGBA {
	var r, g, b, a, n uint64
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			pr, pg, pb, pa := img.At(origin.X+x, origin.Y+y).RGBA()
			r, g, b, a, n = r+uint64(pr), g+uint64(pg), b+uint64(pb), a+uint64(pa), n+1
		}
	}
	if a == 0 {
		return color.NRGBA{}
	}
	// RGBA gives premultiplied 16-bit values, and NRGBA wants plain 8-bit ones.
	return color.NRGBA{R: uint8(r * 255 / a), G: uint8(g * 255 / a), B: uint8(b * 255 / a), A: uint8(a / n >> 8)}
}
