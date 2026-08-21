package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

func craftPaint(brand, slug, name string) catalog.Paint {
	return catalog.Paint{Brand: brand, BrandSlug: slug, Name: name}
}

func nearestBrand(brand, slug string) match.Table {
	p := catalog.Paint{Brand: brand, BrandSlug: slug}
	return match.Table{Cross: []match.BrandMatches{{Brand: brand, Slug: slug, Best: []match.Match{{Paint: &p, DE: 1.0}}}}}
}

// craftSizes counts two different things per row and the guide's whole argument
// is the gap between them, so a row that reports its size in both columns says
// nothing. The second column also has to ignore the craft pages themselves,
// where the nearest range is nearly always another craft range and counting it
// would measure the aisle against itself.
func TestCraftSizesCountsColoursAndAnswersSeparately(t *testing.T) {
	var ps []catalog.Paint
	var ts []match.Table
	add := func(p catalog.Paint, near match.Table) {
		ps = append(ps, p)
		ts = append(ts, near)
	}
	// Two craft ranges of the same size, so only the answers can order them.
	for i := 0; i < 5; i++ {
		add(craftPaint("FolkArt", "folkart", "F"+strconv.Itoa(i)), nearestBrand("Golden", "golden"))
		add(craftPaint("Golden", "golden", "G"+strconv.Itoa(i)), nearestBrand("FolkArt", "folkart"))
	}
	// Miniature paints answered by the craft aisle: three go to FolkArt, one to
	// Golden, one to a miniature range that must not appear in the table.
	for i := 0; i < 3; i++ {
		add(craftPaint("Citadel", "citadel", "C"+strconv.Itoa(i)), nearestBrand("FolkArt", "folkart"))
	}
	add(craftPaint("Citadel", "citadel", "C4"), nearestBrand("Golden", "golden"))
	add(craftPaint("Citadel", "citadel", "C5"), nearestBrand("Vallejo", "vallejo"))
	// A paint with nothing to compare against must not panic or count.
	add(craftPaint("Citadel", "citadel", "C6"), match.Table{})

	g := &Generator{paints: ps, tables: ts}
	want := []rangeCount{
		{Brand: "FolkArt", N: 5, Nearest: 3},
		{Brand: "Golden", N: 5, Nearest: 1},
	}
	got := g.craftSizes()
	if len(got) != len(want) {
		t.Fatalf("craftSizes() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %v, want %v", i, got[i], w)
		}
	}
}

// {{with .Craft}} over an empty slice is silence, not an error: point the guide
// at another table in a copy-paste and it ships with the evidence for its
// central claim gone, the build green and nothing to say so. The Nearest column
// is the one that carries the argument, so it is checked on its own.
func TestTheCraftGuidePrintsItsOwnTable(t *testing.T) {
	const slug = "craft-paint-on-miniatures"
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
	}{
		common: common{Title: gd.Title, Path: "/guides/" + slug + "/"},
		Guide:  gd,
		// Here to be absent from the output: every one of these tables has the
		// same shape and the same field names, so the only thing that catches
		// the guide reading the wrong slice is the wrong slice printing.
		Metal: []rangeCount{{Brand: "Wrong Metal Brand", Name: "Wrong Metal", N: 7777}},
		Craft: []rangeCount{{Brand: "Marker Brand", N: 8123, Nearest: 9001}},
	}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Marker Brand", strconv.Itoa(8123), strconv.Itoa(9001)} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its craft table", want)
		}
	}
	// The craft table prints a brand and two counts, so a metal row wired in by
	// mistake would show its brand, not its label. Checking for the label would
	// pass on exactly the swap this is here to catch.
	if bytes.Contains(buf.Bytes(), []byte("Wrong Metal Brand")) {
		t.Error("the craft guide prints the metallics table")
	}
}

// The craft warning is the last branch of the chain above the match list, and
// it is the only one about the answer rather than about the paint. It must not
// take a warning away from the three in front of it, and it must not fire on
// the craft pages themselves.
func TestPaintPageWarnsWhenTheFirstAnswerIsCraft(t *testing.T) {
	tp, err := template.New("paint").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/paint.html")
	if err != nil {
		t.Fatal(err)
	}
	render := func(rng string, craftFirst bool) string {
		var buf bytes.Buffer
		data := struct {
			common
			Name       string
			Buy        string
			BuyBest    string
			BestName   string
			Summary    string
			FAQ        []qa
			Paint      catalog.Paint
			Table      match.Table
			Detailed   []match.BrandMatches
			Rest       []match.BrandMatches
			Charts     map[string]bool
			CraftFirst bool
		}{
			common:     common{Title: "t", Path: "/paint/citadel/x/"},
			Paint:      catalog.Paint{Brand: "Citadel", Range: rng, Hex: "#000000"},
			Name:       "X",
			CraftFirst: craftFirst,
		}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	const craftLink = "/guides/craft-paint-on-miniatures/"
	const washLink = "/guides/washes-and-shades/"

	if !bytes.Contains([]byte(render("Base", true)), []byte(craftLink)) {
		t.Error("a page whose first answer is a craft range does not link to the craft guide")
	}
	if bytes.Contains([]byte(render("Base", false)), []byte(craftLink)) {
		t.Error("a page answered by a miniature range links to the craft guide")
	}
	wash := render("Shade", true)
	if !bytes.Contains([]byte(wash), []byte(washLink)) {
		t.Error("a wash answered by a craft range loses the wash warning, which is about the pot in hand")
	}
	if bytes.Contains([]byte(wash), []byte(craftLink)) {
		t.Error("a wash answered by a craft range carries two warnings at once")
	}
}

// Charts are the one page where this warning belongs and the wash, air and
// metallic ones do not: a chart is brand to brand and cannot know what kind of
// pot the reader wants, but it does know both of its own columns.
func TestCraftChartsLinkToTheGuideAndOthersDoNot(t *testing.T) {
	tp, err := template.New("chart").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/chart.html")
	if err != nil {
		t.Fatal(err)
	}
	render := func(craft bool) string {
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
			common: common{Title: "t", Path: "/convert/citadel-to-folkart/"},
			From:   catalog.Brand{Name: "Citadel", Slug: "citadel"},
			To:     catalog.Brand{Name: "FolkArt", Slug: "folkart"},
			Craft:  craft,
		}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	const craftLink = "/guides/craft-paint-on-miniatures/"
	if !bytes.Contains([]byte(render(true)), []byte(craftLink)) {
		t.Error("a chart with a craft range in it does not link to the craft guide")
	}
	if bytes.Contains([]byte(render(false)), []byte(craftLink)) {
		t.Error("a chart between two miniature ranges links to the craft guide")
	}
}

// unsearched decides three things that ship: which pages ask not to be indexed,
// which rows the craft table holds, and which pages carry the warning. The
// second and third only make sense while the list is exactly the craft and
// fine-art lines, which is why they were allowed to share it.
func TestUnsearchedIsTheCraftAisle(t *testing.T) {
	for _, slug := range []string{"folkart", "apple-barrel", "golden", "liquitex", "arteza"} {
		if !unsearched[slug] {
			t.Errorf("%s is named in the craft guide and is not in unsearched", slug)
		}
	}
	if len(unsearched) != 5 {
		t.Errorf("unsearched holds %d ranges, but the craft guide names five", len(unsearched))
	}
}
