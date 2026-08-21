package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
)

func TestRangeSizesCountsEveryLabelAPaintCarries(t *testing.T) {
	paint := func(brand, rng string) catalog.Paint {
		return catalog.Paint{Brand: brand, Range: rng}
	}
	var ps []catalog.Paint
	for i := 0; i < 12; i++ {
		// A colour sold in two lines is a pot in each of them, so it counts twice.
		ps = append(ps, paint("Citadel", "Air"+catalog.RangeSep+"Base"))
	}
	for i := 0; i < 20; i++ {
		ps = append(ps, paint("Citadel", "Layer"))
	}
	for i := 0; i < 30; i++ {
		// Discontinued: nothing to buy, and the article says so in prose instead.
		ps = append(ps, paint("Citadel", "Foundation (discontinued)"))
	}
	for i := 0; i < 30; i++ {
		// A brand the guide does not talk about is not measured for it.
		ps = append(ps, paint("AK Interactive", "General"))
	}
	for i := 0; i < 5; i++ {
		// Under the floor: too few pots for the row to say anything.
		ps = append(ps, paint("Citadel", "Glaze"))
		ps = append(ps, paint("Vallejo", "Liquid Gold"))
	}

	g := &Generator{paints: ps}
	got := g.rangeSizes()

	want := []rangeCount{{Name: "Layer", N: 20}, {Name: "Air", N: 12}, {Name: "Base", N: 12}}
	if len(got["Citadel"]) != len(want) {
		t.Fatalf("Citadel ranges = %v, want %v", got["Citadel"], want)
	}
	for i, w := range want {
		if got["Citadel"][i] != w {
			t.Errorf("Citadel range %d = %v, want %v", i, got["Citadel"][i], w)
		}
	}
	if _, ok := got["AK Interactive"]; ok {
		t.Error("a brand the guide never mentions was measured for it")
	}
	if got["Vallejo"] != nil {
		t.Errorf("Vallejo = %v, want nothing: every label is under the floor", got["Vallejo"])
	}
}

// The guide asks for each table by a brand name typed into the template. A name
// that rangeSizes never fills — a typo, a brand renamed upstream, a brand
// dropped from rangeGuideBrands — renders nothing at all: {{with}} over a
// missing key is silence, not an error. The article would ship with its tables
// quietly gone and every other test still green.
func TestEveryRangeTableInTheGuideGetsFilled(t *testing.T) {
	const slug = "paint-ranges-explained"
	var gd guide
	for _, g := range guides {
		if g.Slug == slug {
			gd = g
		}
	}
	if gd.Slug == "" {
		t.Fatalf("no guide with slug %q", slug)
	}

	// A different count per brand, so a table filled from the wrong key cannot
	// pass for the right one.
	ranges := map[string][]rangeCount{}
	for i, brand := range rangeGuideBrands {
		ranges[brand] = []rangeCount{{Name: "Marker Range", N: 9000 + i}}
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
	}{common{Title: gd.Title, Path: "/guides/" + slug + "/"}, gd, ranges}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	for i, brand := range rangeGuideBrands {
		if !bytes.Contains(buf.Bytes(), []byte(strconv.Itoa(9000+i))) {
			t.Errorf("the guide never prints the range table for %s", brand)
		}
	}
}
