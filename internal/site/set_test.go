package site

import (
	"bytes"
	"net/url"
	"strings"
	"testing"

	"tintaequivalente/internal/catalog"
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
