package site

import (
	"bytes"
	"html/template"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

func TestRangeOfNamesTheProductAndDropsWhatIsNotARangeName(t *testing.T) {
	for _, c := range []struct {
		brand, rng, want string
		why              string
	}{
		{"Citadel", "Base", "Base", "the ordinary case"},
		{"Italeri", "Italeri Acrylic Paint", "Acrylic Paint",
			"a range that repeats the maker would print the brand twice on the row"},
		{"Citadel", "Air" + catalog.RangeSep + "Base", "Air",
			"a row gets one label; the paint's own page lists them all"},
		// Upstream splits "Tom Colors …" in the wrong place, so this site
		// receives the tail. The three shapes it arrives in are all in the data.
		{"Tom Color", "s", "", "240 rows of it"},
		{"Tom Color", "s - UP", "", "nothing of range-name length left after the dash"},
		{"Tom Color", "s - Primer", "Primer",
			"the leftover has to go without taking the range name with it"},
		{"Mr. Hobby", "Mr Color", "Mr Color",
			"a short first word is only a leftover when a dash follows it"},
		{"Mr. Paint", "Mr Paint - Car", "Mr Paint - Car",
			"the brand carries a dot the range does not, so nothing is trimmed here at all"},
		{"Reaper", "Reaper", "", "a range field holding only the brand says nothing"},
		{"AK Interactive", "Warfront  Range", "Warfront Range",
			"the double space is in the source data"},
		{"Citadel", "Foundation (discontinued)", "Foundation (discontinued)",
			"discontinued is the most useful word on a substitute's row, unlike on a buy link"},
	} {
		got := rangeOf(catalog.Paint{Brand: c.brand, Range: c.rng})
		if got != c.want {
			t.Errorf("rangeOf(%s / %q) = %q, want %q: %s", c.brand, c.rng, got, c.want, c.why)
		}
	}
}

// Every match list on the site used to give brand, name and code and stop
// there, so a reader replacing a Base could be handed a wash with nothing on
// the line to say so. The fix is four {{with rangeOf}} calls in two templates
// and nothing references them: delete one in a rewrite and the build stays
// green while the page goes back to hiding half of the answer.
//
// Each label below is prefixed with its own brand name, which rangeOf strips.
// A template that printed .Range straight would render "Vallejo Crossrange"
// and miss, which is the other way this can silently regress.
func TestMatchListsPrintTheRangeOfTheSuggestedPaint(t *testing.T) {
	pot := func(brand, name, rng string) *catalog.Paint {
		return &catalog.Paint{
			Brand: brand, BrandSlug: catalog.Slug(brand),
			Name: name, Slug: catalog.Slug(name),
			Range: brand + " " + rng, Hex: "#101010",
		}
	}
	rendered := func(t *testing.T, name string, files []string, data any) []byte {
		t.Helper()
		files = append([]string{"tmpl/layout.html"}, files...)
		tp, err := template.New(name).Funcs(tmplFuncs).ParseFS(tmplFS, files...)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	carries := func(t *testing.T, page []byte, label, where string) {
		t.Helper()
		if !bytes.Contains(page, []byte(`<em class="rng">`+label+`</em>`)) {
			t.Errorf("%s no longer names the range of the paint it suggests (%q)", where, label)
		}
	}

	subject := pot("Citadel", "Abaddon Black", "Base")
	cross := pot("Vallejo", "Black", "Crossrange")
	rest := pot("AK Interactive", "Coal Black", "Restrange")
	same := pot("Citadel", "Nuln Oil", "Samerange")

	page := rendered(t, "paint", []string{"tmpl/paint.html"}, struct {
		common
		Name     string
		Buy      string
		BuyBest  string
		BestName string
		Summary  string
		FAQ      []qa
		Paint    catalog.Paint
		Table    match.Table
		Detailed []match.BrandMatches
		Rest     []match.BrandMatches
		Charts   map[string]bool
	}{
		common: common{Title: "Abaddon Black", Path: subject.URL()},
		Name:   subject.Name, Paint: *subject,
		Table:    match.Table{SameRow: []match.Match{{Paint: same, DE: 2.1}}},
		Detailed: []match.BrandMatches{{Brand: "Vallejo", Slug: "vallejo", Best: []match.Match{{Paint: cross, DE: 1.2}}}},
		Rest:     []match.BrandMatches{{Brand: "AK Interactive", Slug: "ak-interactive", Best: []match.Match{{Paint: rest, DE: 9.4}}}},
	})
	carries(t, page, "Crossrange", "the cross-brand block")
	carries(t, page, "Restrange", "the remaining-ranges table")
	carries(t, page, "Samerange", "the nearest-inside-this-brand list")

	chart := rendered(t, "chart", []string{"tmpl/chart.html"}, struct {
		common
		Summary string
		From    catalog.Brand
		To      catalog.Brand
		Rows    []match.Pair
		Sets    []setLink
	}{
		common: common{Title: "Citadel to Vallejo", Path: "/convert/citadel-to-vallejo/"},
		From:   catalog.Brand{Name: "Citadel", Slug: "citadel"},
		To:     catalog.Brand{Name: "Vallejo", Slug: "vallejo"},
		Rows:   []match.Pair{{From: subject, To: cross, DE: 1.2}},
	})
	carries(t, chart, "Base", "the chart's own column")
	carries(t, chart, "Crossrange", "the chart's answer column")
}
