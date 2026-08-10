package color

import (
	"math"
	"testing"
)

// The official CIEDE2000 acceptance set from Sharma, Wu & Dalal (2005). The
// pairs sit exactly on the discontinuities where a naive implementation
// silently disagrees, so this is the only proof the four hue bands and the RT
// rotation term are wired correctly.
var sharmaPairs = []struct {
	a, b Lab
	want float64
}{
	{Lab{50.0000, 2.6772, -79.7751}, Lab{50.0000, 0.0000, -82.7485}, 2.0425},
	{Lab{50.0000, 3.1571, -77.2803}, Lab{50.0000, 0.0000, -82.7485}, 2.8615},
	{Lab{50.0000, 2.8361, -74.0200}, Lab{50.0000, 0.0000, -82.7485}, 3.4412},
	{Lab{50.0000, -1.3802, -84.2814}, Lab{50.0000, 0.0000, -82.7485}, 1.0000},
	{Lab{50.0000, -1.1848, -84.8006}, Lab{50.0000, 0.0000, -82.7485}, 1.0000},
	{Lab{50.0000, -0.9009, -85.5211}, Lab{50.0000, 0.0000, -82.7485}, 1.0000},
	{Lab{50.0000, 0.0000, 0.0000}, Lab{50.0000, -1.0000, 2.0000}, 2.3669},
	{Lab{50.0000, -1.0000, 2.0000}, Lab{50.0000, 0.0000, 0.0000}, 2.3669},
	{Lab{50.0000, 2.4900, -0.0010}, Lab{50.0000, -2.4900, 0.0009}, 7.1792},
	{Lab{50.0000, 2.4900, -0.0010}, Lab{50.0000, -2.4900, 0.0010}, 7.1792},
	{Lab{50.0000, 2.4900, -0.0010}, Lab{50.0000, -2.4900, 0.0011}, 7.2195},
	{Lab{50.0000, 2.4900, -0.0010}, Lab{50.0000, -2.4900, 0.0012}, 7.2195},
	{Lab{50.0000, -0.0010, 2.4900}, Lab{50.0000, 0.0009, -2.4900}, 4.8045},
	{Lab{50.0000, -0.0010, 2.4900}, Lab{50.0000, 0.0010, -2.4900}, 4.8045},
	{Lab{50.0000, -0.0010, 2.4900}, Lab{50.0000, 0.0011, -2.4900}, 4.7461},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{50.0000, 0.0000, -2.5000}, 4.3065},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{73.0000, 25.0000, -18.0000}, 27.1492},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{61.0000, -5.0000, 29.0000}, 22.8977},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{56.0000, -27.0000, -3.0000}, 31.9030},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{58.0000, 24.0000, 15.0000}, 19.4535},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{50.0000, 3.1736, 0.5854}, 1.0000},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{50.0000, 3.2972, 0.0000}, 1.0000},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{50.0000, 1.8634, 0.5757}, 1.0000},
	{Lab{50.0000, 2.5000, 0.0000}, Lab{50.0000, 3.2592, 0.3350}, 1.0000},
	{Lab{60.2574, -34.0099, 36.2677}, Lab{60.4626, -34.1751, 39.4387}, 1.2644},
	{Lab{63.0109, -31.0961, -5.8663}, Lab{62.8187, -29.7946, -4.0864}, 1.2630},
	{Lab{61.2901, 3.7196, -5.3901}, Lab{61.4292, 2.2480, -4.9620}, 1.8731},
	{Lab{35.0831, -44.1164, 3.7933}, Lab{35.0232, -40.0716, 1.5901}, 1.8645},
	{Lab{22.7233, 20.0904, -46.6940}, Lab{23.0331, 14.9730, -42.5619}, 2.0373},
	{Lab{36.4612, 47.8580, 18.3852}, Lab{36.2715, 50.5065, 21.2231}, 1.4146},
	{Lab{90.8027, -2.0831, 1.4410}, Lab{91.1528, -1.6435, 0.0447}, 1.4441},
	{Lab{90.9257, -0.5406, -0.9208}, Lab{88.6381, -0.8985, -0.7239}, 1.5381},
	{Lab{6.7747, -0.2908, -2.4247}, Lab{5.8714, -0.0985, -2.2286}, 0.6377},
	{Lab{2.0776, 0.0795, -1.1350}, Lab{0.9033, -0.0636, -0.5514}, 0.9082},
}

func TestDistanceSharma(t *testing.T) {
	for i, c := range sharmaPairs {
		if got := Distance(c.a, c.b); math.Abs(got-c.want) > 0.0001 {
			t.Errorf("pair %d: dE00 = %.4f, want %.4f", i+1, got, c.want)
		}
	}
}

// Pairs 7 and 8 of the official set exist precisely to catch an implementation
// that is not symmetric.
func TestDistanceSymmetric(t *testing.T) {
	for i, c := range sharmaPairs {
		fwd, rev := Distance(c.a, c.b), Distance(c.b, c.a)
		if math.Abs(fwd-rev) > 1e-9 {
			t.Errorf("pair %d: not symmetric, %.6f vs %.6f", i+1, fwd, rev)
		}
	}
}

func TestQualityBands(t *testing.T) {
	cases := []struct {
		de   float64
		want string
	}{
		{0.5, "indistinguishable"},
		{1.5, "near-perfect"},
		{3.0, "very close"},
		{4.9, "close"},
		{9.9, "similar"},
		{12.0, "far"},
	}
	for _, c := range cases {
		if got := Quality(c.de); got != c.want {
			t.Errorf("Quality(%.1f) = %q, want %q", c.de, got, c.want)
		}
	}
}

// A swatch must never print text it cannot be read against.
func TestReadableContrast(t *testing.T) {
	if got := Readable(255, 255, 255); got != "#111111" {
		t.Errorf("white swatch got %s, want dark ink", got)
	}
	if got := Readable(0, 0, 0); got != "#ffffff" {
		t.Errorf("black swatch got %s, want light ink", got)
	}
}

// Describe feeds a sentence on every paint page; a wrong hue band there is a
// factual error about the colour, not a cosmetic one.
func TestDescribeBands(t *testing.T) {
	cases := []struct {
		name string
		lab  Lab
		want string
	}{
		{"pure red", Lab{L: 53.24, A: 80.09, B: 67.20}, "a mid-toned, vivid red"},
		{"pure orange", Lab{L: 74.93, A: 23.94, B: 78.95}, "a light, vivid orange"},
		{"pure yellow", Lab{L: 97.14, A: -21.55, B: 94.48}, "a very light, vivid yellow"},
		{"pure green", Lab{L: 87.73, A: -86.18, B: 83.18}, "a very light, vivid green"},
		{"pure cyan", Lab{L: 91.11, A: -48.09, B: -14.13}, "a very light, saturated blue-green"},
		{"pure blue", Lab{L: 32.30, A: 79.19, B: -107.86}, "a dark, vivid blue"},
		{"pure magenta", Lab{L: 60.32, A: 98.25, B: -60.84}, "a mid-toned, vivid magenta"},
		{"mid grey", Lab{L: 53.59, A: 0, B: 0}, "a mid grey"},
		{"black", Lab{L: 0, A: 0, B: 0}, "a near-black neutral"},
		{"white", Lab{L: 100, A: 0, B: 0}, "an off-white"},
	}
	for _, c := range cases {
		if got := Describe(c.lab); got != c.want {
			t.Errorf("Describe(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}
