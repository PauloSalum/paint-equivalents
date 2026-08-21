package site

import (
	"fmt"
	"sort"

	"tintaequivalente/internal/catalog"
)

// guide is one written article. Everything else on this site is generated: a
// paint page is the same three sentences with different measurements in them,
// which is the right shape for "what replaces this pot" and the wrong shape for
// "why is that the answer". These are written by hand, one file per article
// under tmpl/guides/, and the metadata sits here so the index, the sitemap and
// the page itself cannot end up disagreeing about a title.
//
// Adding one is two steps: an entry here and tmpl/guides/<slug>.html defining
// "body". A slug with no file fails in New(), and a body that links to a page
// that does not exist fails in verify(), so neither mistake can ship.
type guide struct {
	Slug      string
	Title     string
	Desc      string
	Heading   string
	Lede      string
	Published string
}

var guides = []guide{{
	Slug:    "how-close-is-close-enough",
	Title:   "How close is close enough? What ΔE means at the workbench",
	Desc:    "Why subtracting two hex codes gives the wrong answer, what CIEDE2000 measures instead, and how close a paint match has to be before it survives on a painted model.",
	Heading: "How close is close enough?",
	Lede:    "Every match on this site carries a number. This is what it measures, and what it cannot see.",
	// The date the article was written. It is not a build timestamp: rebuilding
	// the site does not make a two-month-old article new, and telling Google it
	// does is the fastest way to be trusted on neither.
	Published: "2026-08-18",
}, {
	Slug:      "substituting-a-paint",
	Title:     "Substituting a paint: choosing a replacement that survives the model",
	Desc:      "How to replace a discontinued or unavailable miniature paint: name the job the pot is doing, set the tolerance from that, search the same range before you cross brands, and test before you commit.",
	Heading:   "Substituting a paint",
	Lede:      "The nearest colour is rarely the whole answer. This is the order to decide things in when the pot is discontinued, out of stock, or simply not the range you own.",
	Published: "2026-08-18",
}, {
	Slug:      "what-a-hex-code-misses",
	Title:     "What a hex code misses: opacity, finish, metallics and the drift as it dries",
	Desc:      "A colour value cannot tell you whether a paint covers, how its finish shifts the colour, why metallics need more than one measurement, or why two matching pots come apart under a different lamp. What to check instead.",
	Heading:   "What a hex code misses",
	Lede:      "Six digits record how a colour looked once, on a flat dry sample, under one light. This is everything about the pot that never reaches them.",
	Published: "2026-08-18",
}, {
	Slug:      "contrast-speedpaint-xpress",
	Title:     "Contrast, Speedpaint and Xpress: the one-coat paints, and what a match between them is worth",
	Desc:      "How Citadel Contrast, Army Painter Speedpaint, Vallejo Xpress Color and Scale75 Instant Colors work, why a transparent paint's colour depends on the undercoat under it, and why a close match between two one-coat ranges promises less than the same match between two opaque paints.",
	Heading:   "Contrast, Speedpaint and Xpress",
	Lede:      "Three ranges sell the same trick: base coat and shadow in one pass. That trick is also why a colour distance between two of them promises less than the same number anywhere else on this site.",
	Published: "2026-08-19",
}, {
	Slug:      "paint-ranges-explained",
	Title:     "Paint ranges explained: what Base, Layer, Air, Contrast and the rest are for",
	Desc:      "What a paint range actually is — consistency, pigment load, chemistry and the job the pot is built for — and what Citadel, Vallejo, The Army Painter and Mr. Hobby put in each of theirs. The half of a paint's name that a colour match cannot read.",
	Heading:   "Paint ranges explained",
	Lede:      "A brand is not one paint. It is several different products wearing the same logo, and the range is the word that tells them apart. A colour distance cannot read it, so you have to.",
	Published: "2026-08-19",
}, {
	Slug:  "washes-and-shades",
	Title: "Washes and shades: why the nearest colour to a wash is usually the wrong pot",
	// Short enough to survive Google's snippet, which the five above are not.
	Desc:      "Why a wash's recorded colour describes how it was sampled more than what it does, and how to choose a substitute for one on this site.",
	Heading:   "Washes and shades",
	Lede:      "A wash is not painted onto a surface. It is let into the recesses, and most of it ends up somewhere you did not put it. That changes what the number beside it is comparing.",
	Published: "2026-08-20",
}, {
	Slug:      "airbrush-ranges",
	Title:     "Air ranges: Model Air, Game Air, Warpaints Air, and when the Air pot is a different colour",
	Desc:      "What Air means on a paint label — thinned for the airbrush, or made for aircraft — and when a brand's Air pot is a different colour from the brush one.",
	Heading:   "Air ranges",
	Lede:      "Air on a label answers two different questions, and only one of them is about how the paint sprays. Which one you are looking at decides what a match to it is worth.",
	Published: "2026-08-21",
}, {
	Slug:      "metallic-paints",
	Title:     "Metallic paints: why the nearest colour to a silver is usually a flat grey",
	Desc:      "Why a metallic's recorded colour is only its flat-on tint, why the closest match to a silver is often a matte grey, and how to spot one in a match list.",
	Heading:   "Metallic paints",
	Lede:      "A metallic is a colour that changes with the angle you hold it at, and this site holds one number for it. Worse, the label beside that number usually does not say the pot has flake in it.",
	Published: "2026-08-21",
}, {
	Slug:      "craft-paint-on-miniatures",
	Title:     "Craft paint on miniatures: when the closest colour is a bottle from the art aisle",
	Desc:      "Why FolkArt, Golden, Apple Barrel, Liquitex and Arteza win so many matches here, what such a match promises, and where a craft bottle belongs on a model.",
	Heading:   "Craft paint on miniatures",
	Lede:      "Five of the ranges this site matches against are sold for decorating wood and painting canvas, not models. They are also, often enough, the closest colour to the pot you are trying to replace.",
	Published: "2026-08-21",
}}

const guideAuthor = "Paulo Salum"

// rangeCount is one range label and how many of this site's colours carry it.
// It exists so the ranges guide can print sizes without any number being typed
// into the prose: a figure written into an article is correct on the day it is
// written and quietly wrong after the next dataset update, with nothing failing
// to say so. Counted from the same catalogue the pages are built from, it can
// only ever disagree with the site if the site is wrong too.
type rangeCount struct {
	// Brand is set only where the table crosses brands. The ranges guide prints
	// one table per brand and has already said whose it is in the sentence above
	// it; the washes guide puts every maker in one list and cannot.
	Brand string
	Name  string
	N     int
	// Shared is set only by airSizes: how many of the N colours sit on a page
	// that also carries a label outside the air ranges, which is this site's way
	// of saying the catalogue records the airbrush pot and a brush pot at one
	// identical value. It is the whole point of that table and meaningless in
	// the other two, so it stays zero there.
	Shared int
	// Nearest is set only by craftSizes: how many of the site's other colours
	// this range is the first one offered for. It is a different question from
	// N — not how much of the catalogue a maker fills, but how much of the
	// site's answer it wins — and the craft guide is built on the gap between
	// the two.
	Nearest int
}

// rangeGuideBrands are the brands the ranges guide walks through in prose. A
// brand this build does not carry is skipped rather than invented, the way
// popular() skips a pairing it has no paints for.
var rangeGuideBrands = []string{"Citadel", "Vallejo", "The Army Painter", "Mr. Hobby"}

const (
	// Below this the label is a handful of specialist pots, and listing it says
	// less about the brand than the row costs to read.
	rangeGuideMinimum = 10
	// Where a table stops being a summary of a brand and starts being its
	// inventory. The labels past it are the long tail the prose does not cover.
	rangeGuideRows = 8
)

// rangeSizes measures each guide brand's range labels for the table in the
// ranges guide.
func (g *Generator) rangeSizes() map[string][]rangeCount {
	own := map[string][]catalog.Paint{}
	for _, name := range rangeGuideBrands {
		own[name] = nil
	}
	for _, p := range g.paints {
		if _, ok := own[p.Brand]; ok {
			own[p.Brand] = append(own[p.Brand], p)
		}
	}
	out := map[string][]rangeCount{}
	for brand, ps := range own {
		var rows []rangeCount
		for r, n := range subRanges(ps, sellable) {
			if n >= rangeGuideMinimum {
				rows = append(rows, rangeCount{Name: r, N: n})
			}
		}
		// Ties break on the name so the page is identical between builds.
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].N != rows[j].N {
				return rows[i].N > rows[j].N
			}
			return rows[i].Name < rows[j].Name
		})
		if len(rows) > rangeGuideRows {
			rows = rows[:rangeGuideRows]
		}
		if len(rows) > 0 {
			out[brand] = rows
		}
	}
	return out
}

// labelTableMinimum keeps a label out of a cross-brand table when only one
// colour on this site carries it. One pot is a stray row in a reference table,
// and the tail of them is long: a wash that shares a page with an opaque paint
// brings its label along, and that page counts for both.
const labelTableMinimum = 2

// labelSizes lists every range on this site whose label answers is, across all
// brands, largest first. Two guides are built around such a table — washes and
// metallics — and both exist for the same reason rangeSizes does: the argument
// is that the label is the only warning a match row carries, so the list of
// labels had better come from the build rather than from what was true on the
// day it was written.
//
// Discontinued labels stay in. The Citadel washes that carry the most useful
// evidence in that whole article are discontinued, and a reader who owns one is
// exactly who is looking for a replacement.
func (g *Generator) labelSizes(is func(string) bool) []rangeCount {
	own := map[string][]catalog.Paint{}
	for _, p := range g.paints {
		own[p.Brand] = append(own[p.Brand], p)
	}
	var rows []rangeCount
	for brand, ps := range own {
		for r, n := range subRanges(ps, rangeName) {
			if n >= labelTableMinimum && is(r) {
				rows = append(rows, rangeCount{Brand: brand, Name: r, N: n})
			}
		}
	}
	// Ties break on brand then label so the page is identical between builds.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].N != rows[j].N {
			return rows[i].N > rows[j].N
		}
		if rows[i].Brand != rows[j].Brand {
			return rows[i].Brand < rows[j].Brand
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

// airGuideMinimum keeps a stray label out of the airbrush table for the same
// reason washGuideMinimum does: one colour under a label is a row that costs
// more to read than it says.
const airGuideMinimum = 2

// airSizes lists every range on this site whose label carries the word Air,
// with how many colours it holds and how many of those share their page with a
// pot from outside the air ranges. That second figure is the article's evidence
// and it cannot be written into the prose, because it is not a fact about
// paint: it is a fact about whether this catalogue happens to record the
// airbrush version and the brush version of one colour at the same value. It
// moves whenever the dataset does, and a number typed into a sentence would not.
func (g *Generator) airSizes() []rangeCount {
	type tally struct {
		brand, label string
		n, shared    int
	}
	at := map[string]*tally{}
	for _, p := range g.paints {
		labels := labelsOf(p, rangeName)
		var air []string
		brush := false
		for _, l := range labels {
			if isAir(l) {
				air = append(air, l)
			} else {
				brush = true
			}
		}
		for _, l := range air {
			key := p.Brand + "\x00" + l
			t := at[key]
			if t == nil {
				t = &tally{brand: p.Brand, label: l}
				at[key] = t
			}
			t.n++
			if brush {
				t.shared++
			}
		}
	}
	var rows []rangeCount
	for _, t := range at {
		if t.n >= airGuideMinimum {
			rows = append(rows, rangeCount{Brand: t.brand, Name: t.label, N: t.n, Shared: t.shared})
		}
	}
	// Ties break on brand then label so the page is identical between builds.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].N != rows[j].N {
			return rows[i].N > rows[j].N
		}
		if rows[i].Brand != rows[j].Brand {
			return rows[i].Brand < rows[j].Brand
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

// craftSizes measures what the craft aisle answers on this site: for each of
// the craft and fine-art lines, how many colours it holds here and how many of
// the site's other colours it is the first range offered for.
//
// That second figure is the craft guide's evidence, and it is the reason the
// table is counted rather than written. It is not a fact about paint. It is a
// fact about how densely a decorative range happens to cover the colour space
// beside the miniature ranges it is being ranked against, and it moves whenever
// either side of that does — which a number typed into a sentence would not.
func (g *Generator) craftSizes() []rangeCount {
	colours := map[string]int{}
	nearest := map[string]int{}
	for i, p := range g.paints {
		// A craft page's own nearest range is usually another craft range, and
		// counting that would measure the aisle against itself.
		if unsearched[p.BrandSlug] {
			colours[p.Brand]++
			continue
		}
		if t := g.tables[i]; len(t.Cross) > 0 && unsearched[t.Cross[0].Slug] {
			nearest[t.Cross[0].Brand]++
		}
	}
	var rows []rangeCount
	for b, n := range colours {
		rows = append(rows, rangeCount{Brand: b, N: n, Nearest: nearest[b]})
	}
	// Sorted by what the table is arguing, not by size: ties break on the name
	// so the page is identical between builds.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Nearest != rows[j].Nearest {
			return rows[i].Nearest > rows[j].Nearest
		}
		return rows[i].Brand < rows[j].Brand
	})
	return rows
}

func (g *Generator) guidePages() error {
	type data struct {
		common
		Guide  guide
		Ranges map[string][]rangeCount
		Washes []rangeCount
		Air    []rangeCount
		Metal  []rangeCount
		Craft  []rangeCount
	}
	sizes := g.rangeSizes()
	washes := g.labelSizes(isWash)
	air := g.airSizes()
	metal := g.labelSizes(isMetal)
	craft := g.craftSizes()
	// Each of these guides is built around its table and reads as an argument
	// with its evidence removed without it. {{with}} over an empty slice is
	// silence, so a dataset that stops carrying those labels — or a wiring
	// mistake here — would publish the article gutted and leave the build green.
	if len(washes) == 0 {
		return fmt.Errorf("no wash ranges in this catalogue: the washes guide would publish without its table")
	}
	if len(air) == 0 {
		return fmt.Errorf("no air ranges in this catalogue: the airbrush guide would publish without its table")
	}
	if len(metal) == 0 {
		return fmt.Errorf("no metallic ranges in this catalogue: the metallics guide would publish without its table")
	}
	if len(craft) == 0 {
		return fmt.Errorf("no craft ranges in this catalogue: the craft guide would publish without its table")
	}
	for _, gd := range guides {
		path := "/guides/" + gd.Slug + "/"
		article := fmt.Sprintf(
			`{"@type":"Article","headline":%q,"description":%q,"datePublished":%q,`+
				`"author":{"@type":"Person","name":%q},"mainEntityOfPage":%q}`,
			gd.Title, gd.Desc, gd.Published, guideAuthor, g.cfg.BaseURL+path)
		err := g.render("guide:"+gd.Slug, "guides/"+gd.Slug+"/index.html", data{
			common: common{
				Title: gd.Title, Desc: gd.Desc, Path: path, Site: g.meta,
				JSONLD: graph(g.crumbList("Guides", "/guides/", gd.Heading), article),
			},
			Guide: gd, Ranges: sizes, Washes: washes, Air: air, Metal: metal, Craft: craft,
		})
		if err != nil {
			return err
		}
	}

	links := make([]Link, 0, len(guides))
	for _, gd := range guides {
		links = append(links, Link{URL: "/guides/" + gd.Slug + "/", From: gd.Heading, Note: gd.Lede})
	}
	type list struct {
		common
		Heading string
		Lede    string
		Links   []Link
	}
	return g.render("list", "guides/index.html", list{
		common: common{
			Title: "Paint matching guides — how to read a colour distance",
			Desc:  "How cross-brand paint matching works, what a colour distance can and cannot tell you, and how to choose a substitute that survives on the model.",
			Path:  "/guides/", Site: g.meta,
		},
		Heading: "Guides",
		Lede:    "The pages on this site answer which paint is closest. These answer whether closest is close enough, and what the number is measuring when it says so.",
		Links:   links,
	})
}
