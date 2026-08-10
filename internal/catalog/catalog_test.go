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
		"Acrilex", "Corfix", "Daiara", "Silverbright",
		"Smooth3D", "Talento", "True Colors", "Tom Color",
	} {
		if Publishable(mit(brand, "Any")) {
			t.Errorf("%s is not cleared for redistribution but passed the filter", brand)
		}
	}
}

// The blocked list is matched case- and space-insensitively, because the
// export is not guaranteed to spell a brand the same way twice.
func TestPublishableBlocksRegardlessOfCasing(t *testing.T) {
	for _, brand := range []string{"acrilex", "  ACRILEX  ", "TaLeNtO"} {
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
