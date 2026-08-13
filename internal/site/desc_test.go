package site

import (
	"strings"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
)

func brandMatch(brand, name string, de float64) match.BrandMatches {
	p := catalog.Paint{Name: name}
	return match.BrandMatches{Brand: brand, Best: []match.Match{{Paint: &p, DE: de}}}
}

func TestPaintDescNamesSeveralBrandsWithoutOverflowing(t *testing.T) {
	cases := []struct {
		name  string
		full  string
		cross []match.BrandMatches
		want  int // brands the snippet should name
	}{
		{
			name: "short names leave room for three",
			full: "Citadel Balthasar Gold (Base)",
			cross: []match.BrandMatches{
				brandMatch("Arteza", "Bronze", 1.3),
				brandMatch("The Army Painter", "Warrior Skin", 1.3),
				brandMatch("Vallejo", "Raw Sienna", 1.6),
				brandMatch("Reaper", "Bronze", 1.9),
			},
			want: 3,
		},
		{
			name: "one long name spends the budget",
			full: "Citadel Thunderhawk Blue (Dry)",
			cross: []match.BrandMatches{
				brandMatch("AK Interactive", "Aggressor Blue Fs 35109", 0.9),
				brandMatch("Mr. Hobby", "Grayish Blue Fs35237", 2.6),
				brandMatch("Apple Barrel", "Turquoise", 3.1),
			},
			want: 2,
		},
		{
			name:  "a paint with no match still gets a sentence",
			full:  "Citadel Corvus Black (Air)",
			cross: nil,
			want:  0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := paintDesc(c.full, 28, c.cross)

			if n := len([]rune(got)); n > descRunes {
				t.Errorf("description is %d runes, over the %d Google shows:\n%s", n, descRunes, got)
			}
			if !strings.HasPrefix(got, c.full) {
				t.Errorf("description should open with the paint name, got:\n%s", got)
			}
			if !strings.HasSuffix(got, ".") {
				t.Errorf("description should end in a full stop, got:\n%s", got)
			}

			named := 0
			for _, b := range c.cross {
				if strings.Contains(got, b.Brand+" ") {
					named++
				}
			}
			if named != c.want {
				t.Errorf("named %d brands, want %d:\n%s", named, c.want, got)
			}

			// A name cut in half is the failure that never shows up in the
			// HTML, only in the search result.
			for i, b := range c.cross {
				if i >= named {
					break
				}
				if !strings.Contains(got, b.Best[0].Paint.Name) {
					t.Errorf("%s %s was cut mid-name:\n%s", b.Brand, b.Best[0].Paint.Name, got)
				}
			}
		})
	}
}
