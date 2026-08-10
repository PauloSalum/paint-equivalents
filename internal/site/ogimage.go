package site

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

const (
	ogW = 1200
	ogH = 630
)

// ogImage draws the share card a paint page hands to Discord, Reddit and
// Twitter: the paint itself on the left, its nearest equivalents stacked on the
// right. A colour site whose link previews show no colour throws away the one
// thing that makes someone click.
func ogImage(main color.RGBA, matches []color.RGBA) ([]byte, error) {
	// Gutters in the site's own paper colour. Without them a paint whose five
	// nearest neighbours are all within ΔE 2 renders as one flat rectangle,
	// which reads as a broken image rather than as a very good match.
	const gap = 10
	paper := color.RGBA{R: 0xfa, G: 0xf9, B: 0xf7, A: 0xff}

	img := image.NewRGBA(image.Rect(0, 0, ogW, ogH))
	fill(img, img.Bounds(), paper)

	split := ogW / 2
	fill(img, image.Rect(gap, gap, split-gap/2, ogH-gap), main)
	if len(matches) == 0 {
		fill(img, image.Rect(split+gap/2, gap, ogW-gap, ogH-gap), main)
	}
	for i, c := range matches {
		top := gap + i*(ogH-2*gap)/len(matches)
		bottom := gap + (i+1)*(ogH-2*gap)/len(matches)
		if i > 0 {
			top += gap / 2
		}
		fill(img, image.Rect(split+gap/2, top, ogW-gap, bottom), c)
	}

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}
