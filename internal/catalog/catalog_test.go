package catalog

import "testing"

func mit(brand, name string) Paint {
	return Paint{Brand: brand, Name: name, Hex: "#AABBCC", HasCol: true, Source: sourceMIT}
}

// The licence filter is the only thing standing between this site and
// republishing data whose source refused redistribution, so it is tested
// brand by brand rather than in bulk.
func TestPublishableRejectsUnlicensedBrands(t *testing.T) {
	for _, brand := range []string{
		"Corfix", "Daiara", "Silverbright",
		"Smooth3D", "Talento", "True Colors",
	} {
		if Publishable(mit(brand, "Any")) {
			t.Errorf("%s is not cleared for redistribution but passed the filter", brand)
		}
	}
}

// Acrilex and Tom Color are Brazilian and were blocked alongside the six above
// until the upstream file listing was checked: both ship in
// Arcturus5404/miniature-paints, so both are MIT and may be served. They are
// named here so that re-blocking them by reflex — "it is Brazilian, it must be
// the unlicensed scrape" — fails the build instead of silently dropping 366
// paints.
func TestPublishableAcceptsTheBrazilianRangesThatAreMIT(t *testing.T) {
	for _, brand := range []string{"Acrilex", "Tom Color"} {
		if !Publishable(mit(brand, "Any")) {
			t.Errorf("%s ships in the MIT upstream and must be published", brand)
		}
	}
}

// The blocked list is matched case- and space-insensitively, because the
// export is not guaranteed to spell a brand the same way twice.
func TestPublishableBlocksRegardlessOfCasing(t *testing.T) {
	for _, brand := range []string{"corfix", "  CORFIX  ", "TaLeNtO"} {
		if Publishable(mit(brand, "Any")) {
			t.Errorf("%q passed the filter", brand)
		}
	}
}

func TestPublishableAcceptsMITBrands(t *testing.T) {
	for _, brand := range []string{"Citadel", "Vallejo", "The Army Painter", "Scale75"} {
		if !Publishable(mit(brand, "Any")) {
			t.Errorf("%s is MIT licensed but was filtered out", brand)
		}
	}
}

// Anything the owner harvested or typed in locally is outside the MIT grant.
func TestPublishableRejectsNonMITSources(t *testing.T) {
	for _, src := range []string{"harvested", "official", "user", ""} {
		p := mit("Citadel", "Any")
		p.Source = src
		if Publishable(p) {
			t.Errorf("source %q passed the filter", src)
		}
	}
}

func TestPublishableNeedsColour(t *testing.T) {
	p := mit("Citadel", "Any")
	p.HasCol = false
	if Publishable(p) {
		t.Error("a row without colour is useless here and must be dropped")
	}
	p = mit("Citadel", "Any")
	p.Hex = ""
	if Publishable(p) {
		t.Error("a row without a hex must be dropped")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Abaddon Black":     "abaddon-black",
		"Mephiston Red":     "mephiston-red",
		"Väljero  Ölive":    "valjero-olive",
		"Ação & Cor":        "acao-cor",
		"  spaced  out  ":   "spaced-out",
		"P3 Coal Black (2)": "p3-coal-black-2",
		"---":               "",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// Two paints in one brand whose names normalise to the same slug must not
// collide, or one page silently overwrites the other.
func TestDedupeKeepsBothPages(t *testing.T) {
	ps := []Paint{
		{Brand: "Citadel", BrandSlug: "citadel", Slug: "red"},
		{Brand: "Citadel", BrandSlug: "citadel", Slug: "red"},
		{Brand: "Vallejo", BrandSlug: "vallejo", Slug: "red"},
	}
	dedupe(ps)
	if ps[0].Slug == ps[1].Slug {
		t.Fatalf("collision kept: both are %q", ps[0].Slug)
	}
	if ps[2].Slug != "red" {
		t.Errorf("a different brand must keep its slug, got %q", ps[2].Slug)
	}
}

// The same colour sold in two ranges is one paint, and two pages for it are
// duplicates competing for the same search.
func TestMergeFoldsOneColourSoldInTwoRanges(t *testing.T) {
	ps := []Paint{
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Balthasar Gold", Slug: "balthasar-gold", Hex: "#A77353", Range: "Air"},
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Balthasar Gold", Slug: "balthasar-gold", Hex: "#a77353", Range: "Base"},
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Leadbelcher", Slug: "leadbelcher", Hex: "#969696", Range: "Base"},
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Leadbelcher", Slug: "leadbelcher", Hex: "#868686", Range: "Spray"},
	}
	got := merge(ps)
	if len(got) != 3 {
		t.Fatalf("merged to %d paints, want 3: %+v", len(got), got)
	}
	if got[0].Range != "Air"+RangeSep+"Base" {
		t.Errorf("merged paint should carry both ranges, got %q", got[0].Range)
	}
	// A different hex under the same name is a different pigment, not a label.
	if got[1].Range != "Base" || got[2].Range != "Spray" {
		t.Errorf("paints of different colour must stay apart, got %q and %q", got[1].Range, got[2].Range)
	}
}

// The unsuffixed URL has to land on the same paint on every build, whatever
// order the dataset lists the rows in, and it should be the variant sold in
// most ranges rather than whichever one sorted first.
func TestOrderIsStableAndGivesTheWidestVariantTheCleanSlug(t *testing.T) {
	wide := Paint{ID: "citadel:base:leadbelcher", Brand: "Citadel", BrandSlug: "citadel", Name: "Leadbelcher", Slug: "leadbelcher", Range: "Air" + RangeSep + "Base"}
	spray := Paint{ID: "citadel:spray:leadbelcher", Brand: "Citadel", BrandSlug: "citadel", Name: "Leadbelcher", Slug: "leadbelcher", Range: "Spray"}

	for _, in := range [][]Paint{{wide, spray}, {spray, wide}} {
		ps := append([]Paint(nil), in...)
		order(ps)
		dedupe(ps)
		if ps[0].ID != wide.ID {
			t.Fatalf("clean slug went to %q, want the two-range variant", ps[0].ID)
		}
		if ps[0].Slug != "leadbelcher" || ps[1].Slug != "leadbelcher-2" {
			t.Errorf("slugs are %q and %q", ps[0].Slug, ps[1].Slug)
		}
	}
}

// Merging frees the numbered path the absorbed row held, and that path is
// already indexed: it has to answer with the page that took the content over.
func TestVacatedCoversThePathsMergingEmptied(t *testing.T) {
	ps := []Paint{
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Balthasar Gold", Slug: "balthasar-gold", Hex: "#A77353", Range: "Air"},
		{Brand: "Citadel", BrandSlug: "citadel", Name: "Balthasar Gold", Slug: "balthasar-gold", Hex: "#a77353", Range: "Base"},
	}
	got := merge(ps)
	order(got)
	dedupe(got)

	v := Vacated(got)
	if want := "/paint/citadel/balthasar-gold/"; v["/paint/citadel/balthasar-gold-2/"] != want {
		t.Errorf("the freed path points at %q, want %q", v["/paint/citadel/balthasar-gold-2/"], want)
	}
	if len(v) != 1 {
		t.Errorf("wrote %d redirects, want 1: %v", len(v), v)
	}
}

// A page that exists must never be overwritten by a redirect to somewhere else.
func TestVacatedLeavesARealPaintNamedTwoAlone(t *testing.T) {
	ps := []Paint{
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green", Slug: "dark-green", Hex: "#0A0"},
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green", Slug: "dark-green", Hex: "#0a0"},
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green 2", Slug: "dark-green-2", Hex: "#0B0"},
	}
	got := merge(ps)
	order(got)
	dedupe(got)

	if to, ok := Vacated(got)["/paint/tamiya/dark-green-2/"]; ok {
		t.Errorf("redirected over a real paint's page, to %q", to)
	}
}

func TestDedupeAvoidsCollidingWithAnExistingSuffixedName(t *testing.T) {
	ps := []Paint{
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green", Slug: "dark-green"},
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green", Slug: "dark-green"},
		{Brand: "Tamiya", BrandSlug: "tamiya", Name: "Dark green 2", Slug: "dark-green-2"},
	}
	dedupe(ps)
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Slug] {
			t.Fatalf("duplicate slug %q after dedupe: %+v", p.Slug, ps)
		}
		seen[p.Slug] = true
	}
}
