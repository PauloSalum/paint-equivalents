// Package catalog loads the paint rows and decides which of them this site is
// allowed to publish. The licence filter is the point of the package: the
// source dataset mixes rows that may be redistributed with rows that may not,
// and only the first kind is ever returned.
package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"unicode"

	"tintaequivalente/internal/color"
)

// sourceMIT marks the rows that came from Arcturus5404/miniature-paints, which
// is MIT licensed and may be redistributed with attribution. Every other value
// in the column is either harvested from retailer pages or contributed
// locally, and neither is cleared for publication.
const sourceMIT = "mit-dataset"

// blockedBrands are the Brazilian ranges sourced from ricbit/color-scrapers.
// That repository declares no licence and the attribution file records the
// terms as personal, non-commercial use with redistribution refused. They are
// excluded by name and not by the source column, because the column labels
// them mit-dataset and the attribution file says otherwise — when the two
// disagree, the stricter one wins.
//
// The list is deliberately narrower than "every Brazilian range". Checking the
// 34 brand files upstream publishes against the column showed it lying about
// six ranges and telling the truth about two: Acrilex and Tom Color really do
// ship in Arcturus5404/miniature-paints and are MIT like everything else here,
// so they are imported from that file and served. The six below appear in no
// upstream file at all and stay out until a licensed source for them exists.
var blockedBrands = map[string]bool{
	"corfix":       true,
	"daiara":       true,
	"silverbright": true,
	"smooth3d":     true,
	"talento":      true,
	"true colors":  true,
	"sem marca":    true,
}

// Paint is one publishable row.
type Paint struct {
	ID     string  `json:"id"`
	Brand  string  `json:"brand"`
	Code   string  `json:"manufacturer_code"`
	Range  string  `json:"range"`
	Name   string  `json:"name"`
	Hex    string  `json:"hex"`
	R      int     `json:"r"`
	G      int     `json:"g"`
	B      int     `json:"b"`
	HasCol bool    `json:"has_color"`
	Source string  `json:"source"`
	LabL   float64 `json:"lab_l"`
	LabA   float64 `json:"lab_a"`
	LabB   float64 `json:"lab_b"`

	Slug      string `json:"-"`
	BrandSlug string `json:"-"`

	// Folded counts the rows merged into this one. The pages those rows used to
	// have are already indexed, and the count is what tells the build how many
	// paths were left empty behind this paint.
	Folded int `json:"-"`
}

func (p Paint) Lab() color.Lab { return color.Lab{L: p.LabL, A: p.LabA, B: p.LabB} }

// URL is the canonical path of the paint's own page.
func (p Paint) URL() string { return "/paint/" + p.BrandSlug + "/" + p.Slug + "/" }

// Ink is the text colour that stays readable on this paint's swatch.
func (p Paint) Ink() string { return color.Readable(p.R, p.G, p.B) }

// Publishable reports whether a row may be served. A row without colour is
// useless to a colour-matching site even when its licence is clear.
func Publishable(p Paint) bool {
	if p.Source != sourceMIT {
		return false
	}
	if blockedBrands[strings.ToLower(strings.TrimSpace(p.Brand))] {
		return false
	}
	if !p.HasCol || p.Hex == "" || p.Name == "" || p.Brand == "" {
		return false
	}
	return true
}

// Load reads the export and returns only the publishable rows, each carrying
// the slugs its URLs are built from. Rows that are the same colour under two
// range labels are merged into one, and the slugs that remain are made unique
// per brand by suffixing a collision counter, so two paints that normalise to
// the same string never overwrite each other's page.
func Load(path string) ([]Paint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var all []Paint
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	out := make([]Paint, 0, len(all))
	for _, p := range all {
		if !Publishable(p) {
			continue
		}
		p.BrandSlug = Slug(p.Brand)
		p.Slug = Slug(p.Name)
		if p.Slug == "" || p.BrandSlug == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("catalog: nothing publishable")
	}
	out = merge(out)
	order(out)
	dedupe(out)
	return out, nil
}

// RangeSep joins the labels of a paint its maker sells in more than one range.
// Not the middot the pages separate fields with: "Citadel · Air · Base" reads as
// three things and "Citadel · Air / Base" as the two it is.
const RangeSep = " / "

// merge folds rows that are one colour wearing two range labels into a single
// page. Citadel's Balthasar Gold ships in Base and in Air at the same hex and
// the same Lab; published apart they are duplicates of each other, they split
// the ranking signal of every "balthasar gold equivalent" search between two
// URLs, and each one lists the other as an equivalent at ΔE 0.0, which answers
// nothing. Only rows of identical colour are folded — a name reused for a
// different pigment is a different paint and keeps its own page.
func merge(ps []Paint) []Paint {
	at := make(map[string]int, len(ps))
	out := make([]Paint, 0, len(ps))
	for _, p := range ps {
		key := p.BrandSlug + "\x00" + p.Slug + "\x00" + strings.ToLower(p.Hex)
		i, ok := at[key]
		if !ok {
			at[key] = len(out)
			out = append(out, p)
			continue
		}
		out[i].Range = addRange(out[i].Range, p.Range)
		out[i].Folded++
		if out[i].Code == "" {
			out[i].Code = p.Code
		}
	}
	return out
}

// addRange records one more label for a merged paint, alphabetically so the
// same catalog always renders the same string.
func addRange(have, add string) string {
	if add == "" || have == add {
		return have
	}
	if have == "" {
		return add
	}
	parts := strings.Split(have, RangeSep)
	for _, r := range parts {
		if r == add {
			return have
		}
	}
	parts = append(parts, add)
	sort.Strings(parts)
	return strings.Join(parts, RangeSep)
}

// order fixes the sequence dedupe walks, and with it which paint of a same-name
// group owns the unsuffixed URL. Brand and name alone left that group in
// whatever order sort.Slice happened to leave it — the sort is not stable, so
// /paint/citadel/leadbelcher/ could answer with the Spray pot on one build and
// the Base pot on the next, and a URL that moves under the reader is a URL a
// search engine drops. The widest variant takes the clean path, because a
// colour a maker carries in several ranges is the one searched for by bare
// name, and every tie after it is broken on a field that cannot drift.
func order(ps []Paint) {
	sort.Slice(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		switch {
		case a.Brand != b.Brand:
			return a.Brand < b.Brand
		case a.Name != b.Name:
			return a.Name < b.Name
		case strings.Count(a.Range, RangeSep) != strings.Count(b.Range, RangeSep):
			return strings.Count(a.Range, RangeSep) > strings.Count(b.Range, RangeSep)
		case a.Range != b.Range:
			return a.Range < b.Range
		}
		return a.ID < b.ID
	})
}

// dedupe gives every paint its own page. The suffix has to be checked against
// the taken slugs too: a range that already ships a "Dark green 2" would
// otherwise have its page overwritten by the second "Dark green".
func dedupe(ps []Paint) {
	seen := map[string]bool{}
	for i := range ps {
		base := ps[i].Slug
		slug := base
		for n := 2; seen[ps[i].BrandSlug+"/"+slug]; n++ {
			slug = base + "-" + itoa(n)
		}
		ps[i].Slug = slug
		seen[ps[i].BrandSlug+"/"+slug] = true
	}
}

// Vacated maps the paint paths this catalog stopped filling to the page that
// answers for them now. Folding two rows into one frees the numbered path the
// absorbed row used to hold, and that path is already indexed and linked from
// outside: left empty it is a 404 for a page whose content only moved next
// door. A name occupies the slugs base, base-2 … base-n, so the freed ones are
// always the tail of that run.
func Vacated(ps []Paint) map[string]string {
	type group struct {
		rows, folded int
		clean        string
	}
	taken := make(map[string]bool, len(ps))
	groups := map[string]*group{}
	for _, p := range ps {
		taken[p.URL()] = true
		key := p.BrandSlug + "/" + Slug(p.Name)
		gr := groups[key]
		if gr == nil {
			gr = &group{}
			groups[key] = gr
		}
		gr.rows++
		gr.folded += p.Folded
		if p.Slug == Slug(p.Name) {
			gr.clean = p.URL()
		}
	}
	out := map[string]string{}
	for key, gr := range groups {
		if gr.folded == 0 || gr.clean == "" {
			continue
		}
		for n := gr.rows + 1; n <= gr.rows+gr.folded; n++ {
			path := "/paint/" + key + "-" + itoa(n) + "/"
			// A paint genuinely named "Dark green 2" owns that path. Nothing
			// may be written over a page that exists.
			if taken[path] {
				continue
			}
			out[path] = gr.clean
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Slug reduces a name to the lowercase ASCII form used in URLs. Accented
// letters fold to their base letter so a search engine and a reader see the
// same path.
func Slug(s string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if f, ok := fold[r]; ok {
			r = f
		}
		switch {
		case r >= 'a' && r <= 'z', unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

var fold = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'õ': 'o', 'ô': 'o', 'ö': 'o', 'ø': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// Brands returns the distinct brand names, sorted, with their slugs.
type Brand struct {
	Name  string
	Slug  string
	Count int
}

func Brands(ps []Paint) []Brand {
	idx := map[string]*Brand{}
	for _, p := range ps {
		b, ok := idx[p.Brand]
		if !ok {
			b = &Brand{Name: p.Brand, Slug: p.BrandSlug}
			idx[p.Brand] = b
		}
		b.Count++
	}
	out := make([]Brand, 0, len(idx))
	for _, b := range idx {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Export writes the publishable subset back out. Running the site build from
// this file instead of the original guarantees that no unlicensed range can
// reach the repository, whatever the source data later grows to contain.
func Export(path string, ps []Paint) error {
	b, err := json.MarshalIndent(ps, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
