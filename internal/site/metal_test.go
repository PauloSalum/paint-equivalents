package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

// isMetal decides two things that ship: which rows the metallics guide prints,
// and which paint pages carry the warning above their match list. The guide's
// argument is that the rule is deliberately narrow — it reads the range label
// and never the paint's name — so the cases it must not fire on are as much a
// part of the specification as the ones it must.
func TestIsMetalReadsTheLabelAndNotTheName(t *testing.T) {
	metal := []string{
		"Metal Color", "Mr Metal Color", "Metallics", "Metallic paints",
		"Metallic (3rd Gen)", "Mr Metallic Color GX", "Chameleon Colorshift Metallic",
		"Metal N Alchemy Range", "Email Color - Metallic", "Acrylics - Metallics",
	}
	for _, r := range metal {
		if !isMetal(r) {
			t.Errorf("isMetal(%q) = false, want true", r)
		}
	}
	notMetal := []string{
		// The ranges Citadel files its metals under. That these come back false
		// is not a bug being tolerated: it is the article's whole subject, and a
		// rule widened until they pass would fire on every Citadel paint there is.
		"Base", "Layer", "Dry", "Air", "Spray",
		// A metallic range whose label never says so, and the reason the rule
		// does not chase colour words: Pearl White and Gold Yellow are names for
		// how a paint looks, not statements about what is in the pot.
		"Liquid Gold", "Acrylic Pearl Paints", "Model Color", "Warpaints Fanatic",
	}
	for _, r := range notMetal {
		if isMetal(r) {
			t.Errorf("isMetal(%q) = true, want false", r)
		}
	}
}

// labelSizes serves the washes guide and the metallics one off a single scan,
// so the predicate is the only thing separating two published tables. Passing
// the wrong one, or letting the filter drift, swaps the evidence under an
// article without changing a word of its prose.
func TestLabelSizesSplitsTheTwoTablesByPredicateAlone(t *testing.T) {
	paint := func(brand, rng string) catalog.Paint {
		return catalog.Paint{Brand: brand, Range: rng}
	}
	var ps []catalog.Paint
	for i := 0; i < 18; i++ {
		ps = append(ps, paint("Vallejo", "Metal Color"))
	}
	for i := 0; i < 9; i++ {
		ps = append(ps, paint("Mr. Hobby", "Mr Metal Color"))
	}
	// Citadel's metals wear the labels of its flat paints, so no row of this
	// table can come from them however many of them there are.
	for i := 0; i < 40; i++ {
		ps = append(ps, paint("Citadel", "Base"))
	}
	// A wash range is a row of the other table and none of this one.
	for i := 0; i < 16; i++ {
		ps = append(ps, paint("Citadel", "Shade"))
	}
	// One pot under one label is a stray row, not a range.
	ps = append(ps, paint("Mr. Hobby", "Mr Color Super Metallic"))
	// A metallic label sharing a page with a flat one still counts for itself.
	for i := 0; i < 2; i++ {
		ps = append(ps, paint("Green Stuff World", "Acrylic Paints"+catalog.RangeSep+"Metallic Colors"))
	}

	g := &Generator{paints: ps}
	want := []rangeCount{
		{Brand: "Vallejo", Name: "Metal Color", N: 18},
		{Brand: "Mr. Hobby", Name: "Mr Metal Color", N: 9},
		{Brand: "Green Stuff World", Name: "Metallic Colors", N: 2},
	}
	got := g.labelSizes(isMetal)
	if len(got) != len(want) {
		t.Fatalf("labelSizes(isMetal) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %v, want %v", i, got[i], w)
		}
	}
	if wash := g.labelSizes(isWash); len(wash) != 1 || wash[0].Name != "Shade" {
		t.Errorf("labelSizes(isWash) = %v, want the one Shade row", wash)
	}
}

// {{with .Metal}} over an empty slice is silence, not an error: drop the field
// from the page data, rename it, or point the guide at .Washes in a copy-paste,
// and the article ships with the evidence for its central claim gone, the build
// green and nothing to say so.
func TestTheMetallicsGuidePrintsItsOwnTable(t *testing.T) {
	const slug = "metallic-paints"
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
	}{
		common: common{Title: gd.Title, Path: "/guides/" + slug + "/"},
		Guide:  gd,
		// The wash rows are here to be absent from the output: the two tables
		// have the same shape and the same field names, so the only thing that
		// catches the guide reading the wrong slice is the wrong slice printing.
		Washes: []rangeCount{{Brand: "Wrong Brand", Name: "Wrong Wash", N: 7777}},
		Metal:  []rangeCount{{Brand: "Marker Brand", Name: "Marker Metal", N: 9001}},
	}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Marker Brand", "Marker Metal", strconv.Itoa(9001)} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its metal table", want)
		}
	}
	if bytes.Contains(buf.Bytes(), []byte("Wrong Wash")) {
		t.Error("the metallics guide prints the washes table")
	}
}

// The warning is the third branch of one block, and the branches are ordered by
// how urgent the warning is. A label that reads as two of them must produce the
// first, not both: Green Stuff World's Candy Ink Metallic is a real range in
// this catalogue that answers to isWash and isMetal at once.
func TestPaintPageWarnsOnMetalRangesWithoutStealingTheOtherWarnings(t *testing.T) {
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
	const metalLink = "/guides/metallic-paints/"
	const washLink = "/guides/washes-and-shades/"
	const airLink = "/guides/airbrush-ranges/"

	if !bytes.Contains([]byte(render("Metal Color")), []byte(metalLink)) {
		t.Error("a metallic paint's page does not link to the metallics guide")
	}
	// The pages this guide is mostly about are the ones it cannot reach.
	if bytes.Contains([]byte(render("Base")), []byte(metalLink)) {
		t.Error("a flat paint's page links to the metallics guide")
	}
	ink := render("Candy Ink Metallic")
	if !bytes.Contains([]byte(ink), []byte(washLink)) {
		t.Error("a label reading as both loses the wash warning, which is the more urgent one")
	}
	if bytes.Contains([]byte(ink), []byte(metalLink)) {
		t.Error("a label reading as both carries two warnings at once")
	}
	air := render("Metallic Air")
	if !bytes.Contains([]byte(air), []byte(airLink)) {
		t.Error("a metallic air label loses the air warning")
	}
	if bytes.Contains([]byte(air), []byte(metalLink)) {
		t.Error("a metallic air label carries two warnings at once")
	}
}
