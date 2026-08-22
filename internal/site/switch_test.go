package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

func chartRows(des ...float64) []match.Pair {
	rows := make([]match.Pair, 0, len(des))
	for _, de := range des {
		rows = append(rows, match.Pair{DE: de})
	}
	return rows
}

// Three pages quote these four numbers — the chart's own summary, the note
// beside its link on the index, and the brand-switching table — and they only
// agree because they come from one measurement. The brackets are half-open on
// purpose: ΔE 2.0 is not "interchangeable outright" and ΔE 5.0 is not "close
// enough not to show", and an off-by-one at either edge is invisible in the
// published sentence.
func TestCoverageCountsTheBracketsAndTheMiddleRow(t *testing.T) {
	c := coverage(chartRows(0.4, 1.9, 2.0, 3.0, 4.9, 5.0, 9.0))
	if c.Total != 7 {
		t.Errorf("Total = %d, want 7", c.Total)
	}
	if c.Near != 2 {
		t.Errorf("Near = %d, want 2: ΔE 2.0 is outside the interchangeable bracket", c.Near)
	}
	if c.Usable != 5 {
		t.Errorf("Usable = %d, want 5: ΔE 5.0 is outside the usable bracket", c.Usable)
	}
	if c.Median != 3.0 {
		t.Errorf("Median = %v, want 3.0", c.Median)
	}
	if got := c.note(); got != "5 of 7 matched" {
		t.Errorf("note() = %q, want %q", got, "5 of 7 matched")
	}

	// Rows arrive sorted by paint name, not by distance, so the median has to
	// sort before it picks. The same distances scrambled must give the same
	// answer — and scrambled rather than reversed, because reversing an
	// odd-length list leaves the middle element exactly where it was and a
	// median that never sorts would pass.
	if r := coverage(chartRows(3.0, 9.0, 0.4, 5.0, 1.9, 4.9, 2.0)); r != c {
		t.Errorf("coverage of the same rows scrambled = %+v, want %+v", r, c)
	}

	// chartPages skips a pair with no rows before it ever gets here, but a
	// median read off an empty slice panics rather than being wrong quietly.
	if e := coverage(nil); e != (chartCover{}) {
		t.Errorf("coverage(nil) = %+v, want the zero value", e)
	}
}

// The guide's argument is that the two directions of one pair are different
// numbers, so a row on its own proves nothing and the pair has to arrive
// together, in that order. The popular list names some pairs twice, once each
// way, and printing those four rows would say the site cannot count.
func TestSwitchCoveragePairsBothDirectionsOnce(t *testing.T) {
	g := &Generator{
		brands: []catalog.Brand{
			{Name: "Citadel", Slug: "citadel"},
			{Name: "Vallejo", Slug: "vallejo"},
			{Name: "The Army Painter", Slug: "the-army-painter"},
			{Name: "Scale75", Slug: "scale75"},
		},
		coverage: map[string]chartCover{
			chartPath("citadel", "vallejo"):          {Total: 100, Usable: 90, Near: 30, Median: 2.5},
			chartPath("vallejo", "citadel"):          {Total: 300, Usable: 210, Near: 40, Median: 3.9},
			chartPath("citadel", "the-army-painter"): {Total: 100, Usable: 80, Near: 20, Median: 3.0},
			// The Army Painter to Citadel is missing, so that pair cannot be
			// shown at all. Scale75 has no chart in either direction.
		},
	}
	got := g.switchCoverage()
	want := []pairCover{
		{From: "Citadel", To: "Vallejo", chartCover: chartCover{Total: 100, Usable: 90, Near: 30, Median: 2.5}},
		{From: "Vallejo", To: "Citadel", chartCover: chartCover{Total: 300, Usable: 210, Near: 40, Median: 3.9}},
	}
	if len(got) != len(want) {
		t.Fatalf("switchCoverage() returned %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// A brand the popular list names but this build does not carry must be skipped
// rather than printed as an empty row, the way popular() skips the same pairing
// on the home page.
func TestSwitchCoverageSkipsBrandsThisBuildLacks(t *testing.T) {
	g := &Generator{
		brands:   []catalog.Brand{{Name: "Citadel", Slug: "citadel"}},
		coverage: map[string]chartCover{},
	}
	if got := g.switchCoverage(); len(got) != 0 {
		t.Errorf("switchCoverage() = %+v, want nothing", got)
	}
}

// {{with .Switch}} over an empty slice is silence, not an error, and every
// guide table on this site has the same shape. Pointing the article at the
// wrong one in a copy-paste ships it with the evidence for its central claim
// missing and the build green.
func TestTheSwitchingGuidePrintsItsOwnTable(t *testing.T) {
	const slug = "switching-brands"
	var gd guide
	for _, g := range guides {
		if g.Slug == slug {
			gd = g
		}
	}
	if gd.Slug == "" {
		t.Fatalf("no guide with slug %q", slug)
	}

	tp, err := template.ParseFS(tmplFS, "tmpl/layout.html", "tmpl/guide.html", "tmpl/guides/"+slug+".html")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	data := struct {
		common
		Guide  guide
		Ranges map[string][]rangeCount
		Washes []rangeCount
		Air    []rangeCount
		Metal  []rangeCount
		Craft  []rangeCount
		Switch []pairCover
	}{
		common: common{Title: gd.Title, Path: "/guides/" + slug + "/"},
		Guide:  gd,
		// Here to be absent: the craft table also prints a brand and two counts,
		// so the wrong slice would render as a plausible table rather than as an
		// error.
		Craft: []rangeCount{{Brand: "Wrong Craft Brand", N: 7777, Nearest: 6666}},
		Switch: []pairCover{{
			From: "Marker Brand", To: "Other Marker",
			chartCover: chartCover{Total: 8123, Usable: 9001, Near: 4242, Median: 6.7},
		}},
	}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	// Every column, because each one carries a different part of the argument
	// and three of the four are plain integers that a wrong field would still
	// fill in.
	for _, want := range []string{"Marker Brand", "Other Marker", strconv.Itoa(8123), strconv.Itoa(9001), strconv.Itoa(4242), "6.7"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its own table", want)
		}
	}
	if bytes.Contains(buf.Bytes(), []byte("Wrong Craft Brand")) {
		t.Error("the switching guide prints the craft table")
	}
}

// Every chart page carries this link, because the reverse chart it sits beside
// is the thing the guide explains. It is the only guide link on the page that
// is not conditional, so the check is that it is really unconditional.
func TestEveryChartLinksToTheSwitchingGuide(t *testing.T) {
	tp, err := template.New("chart").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/chart.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, craft := range []bool{true, false} {
		var buf bytes.Buffer
		data := struct {
			common
			Summary string
			From    catalog.Brand
			To      catalog.Brand
			Rows    []match.Pair
			Sets    []setLink
			Craft   bool
		}{
			common: common{Title: "t", Path: "/convert/citadel-to-vallejo/"},
			From:   catalog.Brand{Name: "Citadel", Slug: "citadel"},
			To:     catalog.Brand{Name: "Vallejo", Slug: "vallejo"},
			Craft:  craft,
		}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("/guides/switching-brands/")) {
			t.Errorf("a chart page with Craft=%v does not link to the switching guide", craft)
		}
		// The guide is about this specific pair of pages, so the link is worth
		// nothing if the reverse chart it explains is not beside it.
		if !bytes.Contains(buf.Bytes(), []byte("/convert/vallejo-to-citadel/")) {
			t.Errorf("a chart page with Craft=%v lost its reverse link", craft)
		}
	}
}
