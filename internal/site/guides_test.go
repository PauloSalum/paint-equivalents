package site

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// Every guide has to parse, and each has to bring its own "body" — a slug whose
// file is missing, or a file that forgot the define, produces a page with a
// title and no article, which reads as fine in the build log.
func TestEveryGuideRendersItsOwnBody(t *testing.T) {
	seen := map[string]bool{}
	for _, gd := range guides {
		if seen[gd.Slug] {
			t.Fatalf("two guides share the slug %q, so one overwrites the other", gd.Slug)
		}
		seen[gd.Slug] = true
		if gd.Title == "" || gd.Desc == "" || gd.Heading == "" || gd.Lede == "" || gd.Published == "" {
			t.Errorf("guide %q is missing metadata the index and the sitemap need", gd.Slug)
		}

		tp, err := template.ParseFS(tmplFS, "tmpl/layout.html", "tmpl/guide.html", "tmpl/guides/"+gd.Slug+".html")
		if err != nil {
			t.Errorf("guide %q: %v", gd.Slug, err)
			continue
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
		}{common{Title: gd.Title, Path: "/guides/" + gd.Slug + "/"}, gd, nil, nil, nil, nil, nil, nil}
		if err := tp.ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Errorf("guide %q: %v", gd.Slug, err)
			continue
		}
		// Layout and wrapper alone are over 2 kB, so a length floor would pass on
		// an empty article. The <h2> is the cheapest proof a body actually ran.
		if !bytes.Contains(buf.Bytes(), []byte("<h2>")) {
			t.Errorf("guide %q rendered without an article body", gd.Slug)
		}
	}
}

// A guide's Desc is written by hand, and an overlong one is the mistake nothing
// else catches: the page validates, the build passes, and the only place it goes
// wrong is the search result, where Google cuts the sentence off mid-word and the
// reader decides whether to click on half a promise. Every generated page on this
// site already keeps to descRunes — paintDesc counts against it — so the written
// articles are the only descriptions that can drift past it.
func TestGuideDescFitsTheSnippet(t *testing.T) {
	for _, gd := range guides {
		if n := len([]rune(gd.Desc)); n > descRunes {
			t.Errorf("guide %q: description is %d runes, over the %d Google shows:\n%s", gd.Slug, n, descRunes, gd.Desc)
		}
	}
}

// The two templates that carry 9,000-odd pages link to a guide by a slug typed
// into the markup. Renaming the guide would leave every one of those pages
// pointing at nothing, and the only thing that catches it today is verify() at
// the end of a six-minute build.
func TestPagesLinkToGuidesThatExist(t *testing.T) {
	have := map[string]bool{}
	for _, gd := range guides {
		have["/guides/"+gd.Slug+"/"] = true
	}
	names := []string{"tmpl/paint.html", "tmpl/chart.html", "tmpl/home.html", "tmpl/find.html", "tmpl/guide.html"}
	// The guides cite each other, so a rename breaks its siblings' bodies too —
	// the same failure, one directory over, and just as invisible until verify().
	for _, gd := range guides {
		names = append(names, "tmpl/guides/"+gd.Slug+".html")
	}
	for _, name := range names {
		body, err := tmplFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range hrefRE.FindAllSubmatch(body, -1) {
			target := string(m[1])
			if !strings.HasPrefix(target, "/guides/") || target == "/guides/" {
				continue
			}
			if !have[target] {
				t.Errorf("%s links to %s, which no guide publishes", name, target)
			}
		}
	}
}
