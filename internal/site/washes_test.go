package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

// isWash decides two things that ship: which rows the washes guide prints, and
// which of the 9,000 paint pages carry the warning above their match list. Both
// failures are silent — a label wrongly in gets a stray row, a label wrongly
// out loses the warning on exactly the pages that need it.
func TestIsWashReadsTheLabelTheWayAReaderDoes(t *testing.T) {
	wash := []string{
		"Shade", "Washes", "Game Color Wash", "Foundation Wash (discontinued)",
		"Quickshade Washes Set", "Inks for Shading and Glazing", "Dipping Inks",
		"Ink (3rd Gen)", "Inktensity Range", "Fantasy Range - Ink Wash",
	}
	for _, r := range wash {
		if !isWash(r) {
			t.Errorf("isWash(%q) = false, want true", r)
		}
	}
	notWash := []string{
		"Base", "Layer", "Model Air", "Contrast", "Speedpaint 2.0", "Metal Color",
		// "ink" and "dip" only count at the start of a word, or a range of pink
		// craft acrylics reads as a shading line.
		"Pink Tones", "Twinkle Colours",
	}
	for _, r := range notWash {
		if isWash(r) {
			t.Errorf("isWash(%q) = true, want false", r)
		}
	}
}

// The washes table is the one place on the site where a discontinued range has
// to survive the filter: the two Citadel wash generations are the article's
// whole argument, and the reader holding the dead pot is the one shopping.
func TestWashSizesKeepsDiscontinuedAndDropsEverythingElse(t *testing.T) {
	paint := func(brand, rng string) catalog.Paint {
		return catalog.Paint{Brand: brand, Range: rng}
	}
	var ps []catalog.Paint
	for i := 0; i < 16; i++ {
		ps = append(ps, paint("Citadel", "Shade"))
	}
	for i := 0; i < 8; i++ {
		ps = append(ps, paint("Citadel", "Foundation Wash (discontinued)"))
	}
	for i := 0; i < 30; i++ {
		// Not a wash, however large: the table is not an inventory.
		ps = append(ps, paint("Citadel", "Base"))
	}
	// One pot under one label is a stray row, not a range.
	ps = append(ps, paint("Reaper", "Master Series Paints Core Colors Wash"))
	// A wash sharing a page with an opaque paint still counts for its own label.
	for i := 0; i < 2; i++ {
		ps = append(ps, paint("Vallejo", "Model Color"+catalog.RangeSep+"Game Color Wash"))
	}

	g := &Generator{paints: ps}
	want := []rangeCount{
		{Brand: "Citadel", Name: "Shade", N: 16},
		{Brand: "Citadel", Name: "Foundation Wash (discontinued)", N: 8},
		{Brand: "Vallejo", Name: "Game Color Wash", N: 2},
	}
	got := g.labelSizes(isWash)
	if len(got) != len(want) {
		t.Fatalf("labelSizes(isWash) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %v, want %v", i, got[i], w)
		}
	}
}

// {{with .Washes}} over an empty slice is silence, not an error: drop the field
// from the page data or rename it and the article ships with its reference
// table gone, the build green and nothing to say so.
func TestTheWashesGuidePrintsItsTable(t *testing.T) {
	const slug = "washes-and-shades"
	var gd guide
	for _, g := range guides {
		if g.Slug == slug {
			gd = g
		}
	}
	if gd.Slug == "" {
		t.Fatalf("no guide with slug %q", slug)
	}

	washes := []rangeCount{{Brand: "Marker Brand", Name: "Marker Wash", N: 9001}}
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
	}{common{Title: gd.Title, Path: "/guides/" + slug + "/"}, gd, nil, washes}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Marker Brand", "Marker Wash", strconv.Itoa(9001)} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its wash table", want)
		}
	}
}

// The warning on a wash's page is the best-aimed internal link on the site: it
// reaches the few hundred pages whose match list is misleading and no others.
// Deleting the {{if}} in a refactor costs nothing that any other test notices.
func TestPaintPageWarnsOnWashesAndOnlyOnWashes(t *testing.T) {
	tp, err := template.New("paint").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/paint.html")
	if err != nil {
		t.Fatal(err)
	}
	render := func(rng string) string {
		var buf bytes.Buffer
		data := struct {
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
			Charts     map[string]bool
			CraftFirst bool
		}{
			common: common{Title: "t", Path: "/paint/citadel/x/"},
			Paint:  catalog.Paint{Brand: "Citadel", Range: rng, Hex: "#000000"},
			Name:   "X",
		}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	const link = "/guides/washes-and-shades/"
	if !bytes.Contains([]byte(render("Shade")), []byte(link)) {
		t.Error("a wash's page does not link to the washes guide")
	}
	if bytes.Contains([]byte(render("Base")), []byte(link)) {
		t.Error("a base paint's page links to the washes guide")
	}
}
