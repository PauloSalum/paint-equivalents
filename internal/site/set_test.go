package site

import (
	"bytes"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

func setGen(tag string) *Generator {
	return &Generator{cfg: Config{AmazonTag: tag, AmazonHost: "www.amazon.com.br"}}
}

// A set link is the only thing on these pages worth an order of magnitude more
// than a pot, and everything about it is assembled from strings: a query that
// loses the range, or a link that loses the tag, still renders as a button.
func TestBuySetSearchesTheRangeAndKeepsTheTag(t *testing.T) {
	g := setGen("paintequiv-20")
	raw := g.buySet("Citadel", "Base")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("buySet produced an unparseable url %q: %v", raw, err)
	}
	if u.Host != "www.amazon.com.br" {
		t.Errorf("the link points at %q, not the store that issued the tag", u.Host)
	}
	if got := u.Query().Get("tag"); got != "paintequiv-20" {
		t.Errorf("tag is %q, so the click earns nothing", got)
	}
	// "kit" on the Brazilian store, "tinta" from buy(): a search for
	// "Citadel Base set paint" there returns almost nothing.
	if got := u.Query().Get("k"); got != "Citadel Base kit tinta" {
		t.Errorf("query is %q, want the brand, the range and the local words for a paint set", got)
	}
	if k := setGen("").buySet("Citadel", "Base"); k != "" {
		t.Errorf("with no tag configured the markup must carry no affiliate link, got %q", k)
	}
}

func TestBrandSetsRankRangesBySizeAndSkipTheUnboxable(t *testing.T) {
	var own []catalog.Paint
	add := func(rng string, n int) {
		for i := 0; i < n; i++ {
			own = append(own, catalog.Paint{Brand: "Citadel", Range: rng})
		}
	}
	add("Layer", 93)
	add("Air"+catalog.RangeSep+"Base", 57) // counts for both labels
	add("Contrast", 35)
	add("Shade", 16)                     // under setMinimum: nobody boxes it, the search would be empty
	add("Foundation (discontinued)", 44) // the one range that certainly has no box left
	add("s - UP", 30)                    // not a range name; the upstream data splits Tom Colors this way

	sets := setGen("paintequiv-20").brandSets(catalog.Brand{Name: "Citadel"}, own)
	var names []string
	for _, s := range sets {
		names = append(names, s.Name)
	}
	want := []string{"Citadel Layer", "Citadel Air", "Citadel Base", "Citadel Contrast"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("brandSets returned %v, want %v", names, want)
	}

	// A brand whose range field is only its own name still deserves one link.
	solo := []catalog.Paint{{Brand: "Reaper", Range: "Reaper"}}
	got := setGen("paintequiv-20").brandSets(catalog.Brand{Name: "Reaper"}, solo)
	if len(got) != 1 || got[0].Name != "Reaper" {
		t.Errorf("a brand with no sub-ranges got %+v, want a single brand-wide set link", got)
	}
	if n := setGen("").brandSets(catalog.Brand{Name: "Citadel"}, own); n != nil {
		t.Errorf("with no tag configured brandSets must return nothing, got %+v", n)
	}
}

// A paint page offers the box behind the closest match, and every part of that
// sentence can go wrong in silence: the wrong brand earns from a range the
// reader was not sent to, and a label with no box behind it renders as a button
// over an empty result page.
func TestSetForOffersTheBoxBehindTheClosestMatch(t *testing.T) {
	var ps []catalog.Paint
	add := func(brand, rng string, n int) {
		for i := 0; i < n; i++ {
			ps = append(ps, catalog.Paint{Brand: brand, Range: rng})
		}
	}
	add("Vallejo", "Game Color", 40)
	add("Vallejo", "Model Air", 25)
	add("The Army Painter", "Speedpaint Set", 22)
	add("Citadel", "Shade", 16)                    // under setMinimum: no box, no link
	add("Citadel", "Foundation (discontinued)", 44) // big enough and unbuyable
	add("Golden", "Golden", 30)                     // range field is the brand: not a range name

	boxed := boxedRanges(ps)
	g := setGen("paintequiv-20")

	// The first label with a box wins, not the first label: a pot sold as
	// "Shade / Game Color" is boxed only on the second half of that name.
	best := catalog.Paint{Brand: "Vallejo", Range: "Model Air"}
	got := g.setFor(best, boxed)
	if got == nil {
		t.Fatal("a match in a 25-colour range got no set link")
	}
	if got.Name != "Vallejo Model Air" {
		t.Errorf("the set is named %q, so the page sells a range the reader was not sent to", got.Name)
	}
	if k := queryOf(t, got.URL); k != "Vallejo Model Air kit tinta" {
		t.Errorf("the search is %q, want the brand, the label and the local words for a set", k)
	}

	for _, p := range []catalog.Paint{
		{Brand: "Citadel", Range: "Shade"},
		{Brand: "Citadel", Range: "Foundation (discontinued)"},
		{Brand: "Golden", Range: "Golden"},
		{Brand: "Reaper", Range: "Master Series"}, // not in the catalogue at all
	} {
		if s := g.setFor(p, boxed); s != nil {
			t.Errorf("%s %s has no box behind it and got %+v", p.Brand, p.Range, s)
		}
	}

	// A label that already ends in Set would have the markup put the word twice.
	// The button loses the duplicate; the search must not, because the store
	// files the product under the whole name.
	named := g.setFor(catalog.Paint{Brand: "The Army Painter", Range: "Speedpaint Set"}, boxed)
	if named == nil || named.Name != "The Army Painter Speedpaint" {
		t.Errorf("the Speedpaint box is labelled %+v, want the brand and the range without the doubled word", named)
	}
	if named != nil {
		if k := queryOf(t, named.URL); k != "The Army Painter Speedpaint Set kit tinta" {
			t.Errorf("the search is %q, and dropping a word there returns a different shelf", k)
		}
	}

	// Both halves of a two-label paint are checked, and the boxed one wins even
	// when it is second.
	two := catalog.Paint{Brand: "Vallejo", Range: "Primer" + catalog.RangeSep + "Game Color"}
	if s := g.setFor(two, boxed); s == nil || s.Name != "Vallejo Game Color" {
		t.Errorf("a paint labelled Primer / Game Color got %+v, want the Game Color box", s)
	}

	if s := setGen("").setFor(best, boxed); s != nil {
		t.Errorf("with no tag configured the page must carry no affiliate link, got %+v", s)
	}
}

// The paint page is the highest-traffic page type on the site and its set link
// hangs off a {{with}}, which renders as nothing at all when the field behind it
// is dropped: the build stays green and nine thousand pages quietly go back to
// selling one pot.
func TestThePaintTemplateRendersTheSetLink(t *testing.T) {
	tp, err := template.New("paint").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/paint.html")
	if err != nil {
		t.Fatal(err)
	}
	render := func(set *setLink) string {
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
			Set        *setLink
			CraftFirst bool
		}{
			common: common{Title: "t", Path: "/paint/citadel/x/"},
			Paint:  catalog.Paint{Brand: "Citadel", Hex: "#000000"},
			Name:   "X",
			Buy:    "https://example.test/pot",
			Set:    set,
		}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	const want = `<a href="https://example.test/box" rel="nofollow sponsored noopener" target="_blank">Shop Vallejo Game Color set</a>`
	if page := render(&setLink{Name: "Vallejo Game Color", URL: "https://example.test/box"}); !strings.Contains(page, want) {
		t.Error("the paint page does not offer the boxed set behind its closest match")
	}
	if page := render(nil); strings.Contains(page, "example.test/box") {
		t.Error("a page with no box behind its closest match still printed a set link")
	}
}

// setFor compiles just as well called with the paint the page is about as with
// the paint that answers it, and the wrong one is not a broken link: it is a
// box of the range the reader already owns, offered on every page, with every
// other test in this file green. Nothing but the wiring decides which, so the
// wiring is what this renders.
func TestThePaintPageOffersTheBoxOfItsMatchAndNotItsOwn(t *testing.T) {
	var ps []catalog.Paint
	add := func(brand, slug, rng string) {
		// Exactly enough of each to clear setMinimum, so the page offering the
		// wrong range still offers something and the mistake surfaces as a name
		// rather than as a missing button.
		for i := 0; i < setMinimum; i++ {
			ps = append(ps, catalog.Paint{
				Brand: brand, BrandSlug: slug, Range: rng,
				Name: rng + strconv.Itoa(i), Slug: slug + "-" + strconv.Itoa(i),
				Hex: "#808080", R: 128, G: 128, B: 128, LabL: 53.6,
			})
		}
	}
	add("Citadel", "citadel", "Base")
	add("Vallejo", "vallejo", "Game Color")

	tables := make([]match.Table, len(ps))
	tables[0] = match.Table{Cross: []match.BrandMatches{{
		Brand: "Vallejo", Slug: "vallejo",
		Best: []match.Match{{Paint: &ps[setMinimum], DE: 1.2}},
	}}}

	g, err := New(Config{
		Out: t.TempDir(), BaseURL: "https://example.test",
		AmazonTag: "paintequiv-20", AmazonHost: "www.amazon.com.br",
	}, ps, tables)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.paintPages(); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(g.cfg.Out, "paint", "citadel", "citadel-0", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page, []byte("Shop Vallejo Game Color set")) {
		t.Error("the page does not offer the box of the range its closest match comes from")
	}
	if bytes.Contains(page, []byte("Shop Citadel Base set")) {
		t.Error("the page offers a box of the range the reader already has open")
	}
}

func queryOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("unparseable url %q: %v", raw, err)
	}
	return u.Query().Get("k")
}

// The set links live behind a {{if .Sets}} in two templates and nothing else
// references them: a rewrite that drops the block leaves the build green and
// the pages earning pot money instead of set money.
func TestBrandAndChartTemplatesRenderTheSetLinks(t *testing.T) {
	for _, name := range []string{"tmpl/brand.html", "tmpl/chart.html"} {
		body, err := tmplFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("{{range .Sets}}")) {
			t.Errorf("%s no longer renders the boxed-set links", name)
		}
		if !bytes.Contains(body, []byte(`rel="nofollow sponsored noopener"`)) {
			t.Errorf("%s carries a paid link without the rel the programme requires", name)
		}
	}
}
