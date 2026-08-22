package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

// isAir decides two things that ship: which rows the airbrush guide prints, and
// which paint pages carry the warning above their match list. Both failures are
// silent — a label wrongly in gets a stray row, a label wrongly out loses the
// warning on exactly the pages that need it.
func TestIsAirReadsTheWordAndNotThePromise(t *testing.T) {
	air := []string{
		"Air", "Model Air", "Game Air", "Warpaints Air", "Air (3rd Gen)",
		"Real Colors - Air", "Premium Airbrush Color", "Airbrush Colours",
	}
	for _, r := range air {
		if !isAir(r) {
			t.Errorf("isAir(%q) = false, want true", r)
		}
	}
	notAir := []string{
		"Base", "Layer", "Model Color", "Shade", "Metal Color",
		// Aircraft colours in a rattle can: the word is neither the tool this
		// test is looking for nor a whole word, and the range is in the data.
		"Aircraft Spray",
		// "air" only counts standing alone, or a range of fairy-tale craft
		// acrylics reads as an airbrush line.
		"Fairy Tale Colours", "Chairman Range",
	}
	for _, r := range notAir {
		if isAir(r) {
			t.Errorf("isAir(%q) = true, want false", r)
		}
	}
}

// The second column is the article's evidence, and it is the one a refactor can
// quietly turn into a copy of the first: counting labels is easy, counting the
// pages where an air label and a brush label meet is the part that says
// anything. A row whose Shared equals its N means something entirely different
// from one where it is zero, so both have to be checked, not just the total.
func TestAirSizesCountsThePagesWhereAnAirLabelMeetsABrushLabel(t *testing.T) {
	paint := func(brand, rng string) catalog.Paint {
		return catalog.Paint{Brand: brand, Range: rng}
	}
	var ps []catalog.Paint
	// One colour, two labels, one page: the airbrush pot and the brush pot are
	// on record at the same value.
	for i := 0; i < 5; i++ {
		ps = append(ps, paint("Citadel", "Air"+catalog.RangeSep+"Base"))
	}
	// Same label, no brush twin. Counts for N and not for Shared.
	for i := 0; i < 2; i++ {
		ps = append(ps, paint("Citadel", "Air"))
	}
	for i := 0; i < 3; i++ {
		ps = append(ps, paint("Vallejo", "Model Air"))
	}
	// Two air labels on one page is not a brush twin, however merged the page is.
	ps = append(ps, paint("AK Interactive", "Air"+catalog.RangeSep+"Air (3rd Gen)"))
	ps = append(ps, paint("AK Interactive", "Air"+catalog.RangeSep+"Air (3rd Gen)"))
	// One colour under one label is a stray row, not a range.
	ps = append(ps, paint("Vallejo", "Game Air"))
	// A can of aircraft colours is not an airbrush range.
	for i := 0; i < 30; i++ {
		ps = append(ps, paint("Tamiya", "Aircraft Spray"))
	}
	// The label that is not a range name at all must not count as the brush
	// twin that turns Shared on.
	ps = append(ps, paint("Tom Color", "Air"+catalog.RangeSep+"s"))
	ps = append(ps, paint("Tom Color", "Air"+catalog.RangeSep+"s"))

	g := &Generator{paints: ps}
	want := []rangeCount{
		{Brand: "Citadel", Name: "Air", N: 7, Shared: 5},
		{Brand: "Vallejo", Name: "Model Air", N: 3, Shared: 0},
		{Brand: "AK Interactive", Name: "Air", N: 2, Shared: 0},
		{Brand: "AK Interactive", Name: "Air (3rd Gen)", N: 2, Shared: 0},
		{Brand: "Tom Color", Name: "Air", N: 2, Shared: 0},
	}
	got := g.airSizes()
	if len(got) != len(want) {
		t.Fatalf("airSizes() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %v, want %v", i, got[i], w)
		}
	}
}

// {{with .Air}} over an empty slice is silence, not an error: drop the field
// from the page data, rename it, or lose the Shared cell in a table edit, and
// the article ships with the evidence for its central claim gone, the build
// green and nothing to say so.
func TestTheAirbrushGuidePrintsItsTable(t *testing.T) {
	const slug = "airbrush-ranges"
	var gd guide
	for _, g := range guides {
		if g.Slug == slug {
			gd = g
		}
	}
	if gd.Slug == "" {
		t.Fatalf("no guide with slug %q", slug)
	}

	air := []rangeCount{{Brand: "Marker Brand", Name: "Marker Air", N: 9001, Shared: 4242}}
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
	}{common{Title: gd.Title, Path: "/guides/" + slug + "/"}, gd, nil, nil, air}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Marker Brand", "Marker Air", strconv.Itoa(9001), strconv.Itoa(4242)} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its air table", want)
		}
	}
}

// The warning shares its block with the washes one, so the branch that has to
// be checked is not only "does an air page link" but "does a wash page still
// get the wash link instead". An {{if}} turned into two of them stacks two
// warnings on any label that reads as both.
func TestPaintPageWarnsOnAirRangesWithoutStealingTheWashWarning(t *testing.T) {
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
			Charts   map[string]bool
			Set      *setLink
			// Left false throughout: this test is about the three warnings that
			// describe the pot, and the craft one describes the answer.
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
	const airLink = "/guides/airbrush-ranges/"
	const washLink = "/guides/washes-and-shades/"
	if !bytes.Contains([]byte(render("Model Air")), []byte(airLink)) {
		t.Error("an airbrush paint's page does not link to the airbrush guide")
	}
	if bytes.Contains([]byte(render("Base")), []byte(airLink)) {
		t.Error("a base paint's page links to the airbrush guide")
	}
	wash := render("Air Wash")
	if !bytes.Contains([]byte(wash), []byte(washLink)) {
		t.Error("a label reading as both loses the wash warning, which is the more urgent one")
	}
	if bytes.Contains([]byte(wash), []byte(airLink)) {
		t.Error("a label reading as both carries two warnings at once")
	}
}
