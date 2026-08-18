package site

import (
	"testing"

	"tintaequivalente/internal/catalog"
)

// The whole cut is one map, and its failure mode is silent: a slug that matches
// no brand keeps every page indexed and looks exactly like a slug that works.
func TestUnsearchedNamesRangesTheCatalogActuallyHas(t *testing.T) {
	out := []string{"FolkArt", "Apple Barrel", "Golden", "Liquitex", "Arteza"}
	for _, name := range out {
		if !unsearched[catalog.Slug(name)] {
			t.Errorf("%s should be out of the index, but %q is not in the map", name, catalog.Slug(name))
		}
	}
	if len(unsearched) != len(out) {
		t.Errorf("the map holds %d ranges, the list here names %d", len(unsearched), len(out))
	}
}

// Dropping a range that earns clicks costs traffic that took months to build,
// and the two Brazilian ranges are the only pages a Portuguese speaker lands on.
func TestUnsearchedKeepsTheRangesThatEarnClicks(t *testing.T) {
	for _, name := range []string{
		"Citadel", "Vallejo", "AK Interactive", "The Army Painter",
		"Scale75", "Tamiya", "Acrilex", "Tom Color",
	} {
		if unsearched[catalog.Slug(name)] {
			t.Errorf("%s is searched by name and must stay indexed", name)
		}
	}
}
