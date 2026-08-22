package site

import (
	"bytes"
	"html/template"
	"strconv"
	"testing"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/color"
	"tintaequivalente/internal/match"
)

// qualityBands repeats the thresholds that live inside color.Quality, because a
// switch cannot be asked where its own edges are. The table in the methodology
// guide prints those edges next to counts taken by calling Quality, so a
// threshold moved on one side and not the other publishes a row claiming to
// count something it does not. Both edges of every band are checked: a band is
// wrong if it starts late as easily as if it ends early.
func TestQualityBandsMatchTheScale(t *testing.T) {
	lo := 0.0
	for _, b := range qualityBands {
		if b.Upper == 0 {
			// The open-ended band, which has no edge to test above it.
			if got := color.Quality(lo); got != b.Name {
				t.Errorf("Quality(%v) = %q, want %q", lo, got, b.Name)
			}
			if got := color.Quality(1000); got != b.Name {
				t.Errorf("Quality(1000) = %q, want %q", got, b.Name)
			}
			continue
		}
		if got := color.Quality(lo); got != b.Name {
			t.Errorf("Quality(%v) = %q, but the table opens the %q band there", lo, got, b.Name)
		}
		// Half-open, the way every other bracket on this site is: ΔE 2.0 is
		// near-perfect, not indistinguishable.
		if got := color.Quality(b.Upper - 0.0001); got != b.Name {
			t.Errorf("Quality(%v) = %q, but the table runs %q up to %v", b.Upper-0.0001, got, b.Name, b.Upper)
		}
		if got := color.Quality(b.Upper); got == b.Name {
			t.Errorf("Quality(%v) is still %q, but the table ends that band at %v", b.Upper, got, b.Upper)
		}
		lo = b.Upper
	}
}

func tablesWithBest(des ...float64) []match.Table {
	out := make([]match.Table, 0, len(des)+1)
	for _, de := range des {
		p := catalog.Paint{Name: "n"}
		out = append(out, match.Table{Cross: []match.BrandMatches{
			{Brand: "Other", Best: []match.Match{{Paint: &p, DE: de}}},
		}})
	}
	// A paint no other maker answers at all. It is not a row in any band, and
	// counting it in the denominator would understate every share on the page.
	return append(out, match.Table{})
}

// The article's whole argument rests on the shares, so an off-by-one in either
// the numerator or the denominator changes what it claims. The counting has to
// use the same brackets the rest of the site does, and the paint with no
// cross-brand answer has to leave the sums alone.
func TestAnswerBandsCountEveryAnsweredColourOnce(t *testing.T) {
	g := &Generator{tables: tablesWithBest(0.5, 0.9, 1.0, 1.9, 2.0, 3.4, 3.5, 4.9, 5.0, 9.9, 10.0, 40)}
	rows := g.answerBands()
	if len(rows) != len(qualityBands) {
		t.Fatalf("answerBands() gave %d rows, want %d", len(rows), len(qualityBands))
	}

	want := []struct {
		name string
		span string
		n    int
	}{
		{"indistinguishable", "under 1", 2},
		{"near-perfect", "1 to 2", 2},
		{"very close", "2 to 3.5", 2},
		{"close", "3.5 to 5", 2},
		{"similar", "5 to 10", 2},
		{"far", "10 and over", 2},
	}
	total := 0
	for i, w := range want {
		if rows[i].Name != w.name || rows[i].N != w.n {
			t.Errorf("row %d = %q/%d, want %q/%d", i, rows[i].Name, rows[i].N, w.name, w.n)
		}
		if rows[i].Span != w.span {
			t.Errorf("row %d span = %q, want %q", i, rows[i].Span, w.span)
		}
		total += rows[i].N
	}
	if total != 12 {
		t.Errorf("the bands hold %d colours, want the 12 that have an answer", total)
	}
	for i, r := range rows {
		if want := 100 * float64(r.N) / 12; r.Share != want {
			t.Errorf("row %d share = %v, want %v out of the 12 answered", i, r.Share, want)
		}
	}

	// Nothing to divide by, rather than a page of NaN. guidePages turns this
	// into a failed build.
	if e := (&Generator{tables: []match.Table{{}}}).answerBands(); e != nil {
		t.Errorf("answerBands() with nothing answered = %+v, want nil", e)
	}
}

// folded is the one number in the article's prose that comes from the dataset,
// and it is printed inside a {{if}}: a wrong field renders as silence rather
// than as an error.
// The fixture is four paints folding three rows, and neither number may be
// three of anything else: a first version of this test had three paints folding
// three rows, and folded() rewritten to return len(g.paints) passed it.
func TestFoldedCountsTheMergedRows(t *testing.T) {
	g := &Generator{paints: []catalog.Paint{{Folded: 2}, {}, {Folded: 1}, {}}}
	if got := g.folded(); got != 3 {
		t.Errorf("folded() = %d, want the 3 rows the two merged paints absorbed", got)
	}
}

// {{with}} over an empty slice is silence, not an error, and every guide table
// on this site has the same shape. Pointing the article at the wrong one in a
// copy-paste ships it with the evidence for its central claim missing and the
// build green.
func TestTheMethodGuidePrintsItsOwnTable(t *testing.T) {
	const slug = "how-the-matching-works"
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
		Switch []pairCover
		Bands  []bandCount
		Folded int
	}{
		common: common{Title: gd.Title, Path: "/guides/" + slug + "/"},
		Guide:  gd,
		// Here to be absent: the craft table also prints a name and a count, so
		// the wrong slice renders as a plausible table rather than as an error.
		Craft:  []rangeCount{{Brand: "Wrong Craft Brand", N: 7777, Nearest: 6666}},
		Bands:  []bandCount{{Name: "Marker Band", Span: "Marker Span", N: 8123, Share: 42.5}},
		Folded: 4242,
	}
	if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatal(err)
	}
	// Every column, because each carries a different part of the claim and two
	// of the four would be filled in just as plausibly by the wrong field.
	for _, want := range []string{"Marker Band", "Marker Span", strconv.Itoa(8123), "42.5%", strconv.Itoa(4242)} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("the guide never prints %q from its own table", want)
		}
	}
	if bytes.Contains(buf.Bytes(), []byte("Wrong Craft Brand")) {
		t.Error("the methodology guide prints the craft table")
	}
}
