// Package color carries the CIELAB coordinate and the CIEDE2000 distance the
// whole site is ranked by. Adapted from the author's own implementation in
// PaintPot, kept to the subset this generator needs.
package color

import "math"

// Lab is a CIELAB coordinate (D65).
type Lab struct {
	L float64 `json:"lab_l"`
	A float64 `json:"lab_a"`
	B float64 `json:"lab_b"`
}

const pow25_7 = 6103515625.0

func deg(r float64) float64 { return r * 180 / math.Pi }
func rad(d float64) float64 { return d * math.Pi / 180 }

// Distance implements CIE 142:2001 in full, including the four bands of the
// mean hue angle. Reference values: Sharma, Wu & Dalal (2005).
func Distance(a, b Lab) float64 {
	c1 := math.Hypot(a.A, a.B)
	c2 := math.Hypot(b.A, b.B)
	cBar := (c1 + c2) / 2
	c7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(c7/(c7+pow25_7)))

	a1p := (1 + g) * a.A
	a2p := (1 + g) * b.A
	c1p := math.Hypot(a1p, a.B)
	c2p := math.Hypot(a2p, b.B)

	h1p := hueAngle(a.B, a1p)
	h2p := hueAngle(b.B, a2p)

	dLp := b.L - a.L
	dCp := c2p - c1p

	var dhp float64
	if c1p*c2p != 0 {
		dhp = h2p - h1p
		switch {
		case dhp > 180:
			dhp -= 360
		case dhp < -180:
			dhp += 360
		}
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(rad(dhp/2))

	lBarP := (a.L + b.L) / 2
	cBarP := (c1p + c2p) / 2

	var hBarP float64
	switch {
	case c1p*c2p == 0:
		hBarP = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hBarP = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hBarP = (h1p + h2p + 360) / 2
	default:
		hBarP = (h1p + h2p - 360) / 2
	}

	t := 1 -
		0.17*math.Cos(rad(hBarP-30)) +
		0.24*math.Cos(rad(2*hBarP)) +
		0.32*math.Cos(rad(3*hBarP+6)) -
		0.20*math.Cos(rad(4*hBarP-63))

	dTheta := 30 * math.Exp(-math.Pow((hBarP-275)/25, 2))
	cBarP7 := math.Pow(cBarP, 7)
	rc := 2 * math.Sqrt(cBarP7/(cBarP7+pow25_7))
	rt := -math.Sin(rad(2*dTheta)) * rc

	sl := 1 + (0.015*math.Pow(lBarP-50, 2))/math.Sqrt(20+math.Pow(lBarP-50, 2))
	sc := 1 + 0.045*cBarP
	sh := 1 + 0.015*cBarP*t

	tl := dLp / sl
	tc := dCp / sc
	th := dHp / sh

	return math.Sqrt(tl*tl + tc*tc + th*th + rt*tc*th)
}

func hueAngle(b, ap float64) float64 {
	if ap == 0 && b == 0 {
		return 0
	}
	h := deg(math.Atan2(b, ap))
	if h < 0 {
		h += 360
	}
	return h
}

// Quality names the perceptual band a distance falls in. The wording is what
// the reader sees next to every match, so it never shows a bare number alone.
func Quality(de float64) string {
	switch {
	case de < 1:
		return "indistinguishable"
	case de < 2:
		return "near-perfect"
	case de < 3.5:
		return "very close"
	case de < 5:
		return "close"
	case de < 10:
		return "similar"
	}
	return "far"
}

// Grade is the short label the swatch badge carries, coarser than Quality so a
// table can be scanned without reading words.
func Grade(de float64) string {
	switch {
	case de < 2:
		return "A"
	case de < 3.5:
		return "B"
	case de < 5:
		return "C"
	case de < 10:
		return "D"
	}
	return "E"
}

// Describe names a colour the way a painter would, from its own Lab
// coordinates: lightness band, chroma band, hue band. Every paint page needs a
// sentence that is about that paint and not about the template, and the only
// honest source for one is the measurement itself.
func Describe(c Lab) string {
	chroma := math.Hypot(c.A, c.B)
	if chroma < 5 {
		switch {
		case c.L < 15:
			return "a near-black neutral"
		case c.L < 35:
			return "a dark grey"
		case c.L < 65:
			return "a mid grey"
		case c.L < 88:
			return "a light grey"
		}
		return "an off-white"
	}

	var light string
	switch {
	case c.L < 25:
		light = "very dark"
	case c.L < 45:
		light = "dark"
	case c.L < 65:
		light = "mid-toned"
	case c.L < 82:
		light = "light"
	default:
		light = "very light"
	}

	var sat string
	switch {
	case chroma < 18:
		sat = "muted"
	case chroma < 38:
		sat = "moderately saturated"
	case chroma < 60:
		sat = "saturated"
	default:
		sat = "vivid"
	}

	return "a " + light + ", " + sat + " " + Hue(c)
}

// Hue is the colour-name band of a Lab coordinate. The boundaries follow the
// CIELAB hue circle rather than RGB intuition: a* positive is red, b* positive
// is yellow, and the named bands sit where painters expect the transitions.
func Hue(c Lab) string {
	if math.Hypot(c.A, c.B) < 5 {
		return "neutral"
	}
	h := deg(math.Atan2(c.B, c.A))
	if h < 0 {
		h += 360
	}
	// The bands are placed so the sRGB primaries and secondaries land on the
	// name a painter would use for them: red sits at 40°, orange 73°, yellow
	// 103°, green 136°, cyan 196°, blue 306°, magenta 328°. CIELAB compresses
	// the greens and stretches the blues, so evenly spaced bands would call
	// pure blue violet and pure red orange.
	switch {
	case h < 25:
		return "pink-red"
	case h < 56:
		return "red"
	case h < 88:
		return "orange"
	case h < 125:
		return "yellow"
	case h < 170:
		return "green"
	case h < 250:
		return "blue-green"
	case h < 317:
		return "blue"
	case h < 345:
		return "magenta"
	}
	return "pink-red"
}

// Readable picks black or white text for a swatch background, by the WCAG
// relative luminance of the fill.
func Readable(r, g, b int) string {
	lin := func(c int) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	if l > 0.179 {
		return "#111111"
	}
	return "#ffffff"
}
