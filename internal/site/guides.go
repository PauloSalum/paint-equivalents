package site

import "fmt"

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
}}

const guideAuthor = "Paulo Salum"

func (g *Generator) guidePages() error {
	type data struct {
		common
		Guide guide
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
			Guide: gd,
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
