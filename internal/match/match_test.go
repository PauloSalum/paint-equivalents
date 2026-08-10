package match

import (
	"testing"

	"tintaequivalente/internal/catalog"
)

func p(brand, name string, l, a, b float64) catalog.Paint {
	return catalog.Paint{
		Brand: brand, Name: name, BrandSlug: catalog.Slug(brand), Slug: catalog.Slug(name),
		LabL: l, LabA: a, LabB: b, Hex: "#000000", HasCol: true,
	}
}

func fixture() []catalog.Paint {
	return []catalog.Paint{
		p("Citadel", "Mephiston Red", 40, 60, 40),
		p("Vallejo", "Bloody Red", 41, 59, 39), // nearly the same red
		p("Vallejo", "Sky Blue", 70, -10, -30), // far
		p("Army Painter", "Dragon Red", 45, 55, 35),
		p("Citadel", "Evil Sunz Scarlet", 43, 62, 45), // same brand as the source
	}
}

func TestBuildPutsNearestBrandFirst(t *testing.T) {
	ps := fixture()
	tables := Build(ps, 3, 3)

	got := tables[0].Cross
	if len(got) != 2 {
		t.Fatalf("expected 2 other brands, got %d", len(got))
	}
	if got[0].Brand != "Vallejo" {
		t.Errorf("nearest brand = %s, want Vallejo", got[0].Brand)
	}
	if got[0].Best[0].Paint.Name != "Bloody Red" {
		t.Errorf("nearest pot = %s, want Bloody Red", got[0].Best[0].Paint.Name)
	}
	// Sky Blue is in the same brand as Bloody Red and must rank behind it.
	if len(got[0].Best) > 1 && got[0].Best[1].DE < got[0].Best[0].DE {
		t.Error("matches inside a brand are not sorted nearest first")
	}
}

// A paint must never be offered as its own equivalent.
func TestBuildExcludesSelf(t *testing.T) {
	ps := fixture()
	tables := Build(ps, 5, 5)
	for _, bm := range tables[0].Cross {
		for _, m := range bm.Best {
			if m.Paint.Name == ps[0].Name && m.Paint.Brand == ps[0].Brand {
				t.Fatal("paint matched itself")
			}
		}
	}
	for _, m := range tables[0].SameRow {
		if m.Paint.Name == ps[0].Name {
			t.Fatal("paint appeared in its own same-brand list")
		}
	}
}

func TestSameBrandListStaysInBrand(t *testing.T) {
	ps := fixture()
	tables := Build(ps, 3, 3)
	for _, m := range tables[0].SameRow {
		if m.Paint.Brand != "Citadel" {
			t.Errorf("same-brand list leaked %s", m.Paint.Brand)
		}
	}
}

func TestPerBrandCap(t *testing.T) {
	ps := fixture()
	tables := Build(ps, 1, 1)
	for _, bm := range tables[0].Cross {
		if len(bm.Best) != 1 {
			t.Errorf("%s returned %d matches, cap was 1", bm.Brand, len(bm.Best))
		}
	}
}

func TestChartCoversEveryPaintInTheFromBrand(t *testing.T) {
	ps := fixture()
	tables := Build(ps, 3, 3)
	rows := Chart(ps, tables, "Citadel", "Vallejo")
	if len(rows) != 2 {
		t.Fatalf("chart has %d rows, want one per Citadel paint (2)", len(rows))
	}
	for _, r := range rows {
		if r.From.Brand != "Citadel" || r.To.Brand != "Vallejo" {
			t.Errorf("row crosses the wrong brands: %s -> %s", r.From.Brand, r.To.Brand)
		}
	}
}
