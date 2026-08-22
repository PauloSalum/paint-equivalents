// Package site renders the whole static site from a loaded catalog. Every page
// is written to disk once; nothing is computed at request time, because the
// output is meant to sit on static hosting with no server behind it.
package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	stdcolor "image/color"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/color"
	"tintaequivalente/internal/match"
)

// The pattern does not recurse, so the guide bodies need naming separately.
//
//go:embed tmpl/*.html tmpl/guides/*.html
var tmplFS embed.FS

//go:embed assets/*
var assetFS embed.FS

type Config struct {
	BaseURL       string
	Out           string
	AdsenseClient string
	AdsenseSlot   string
	AmazonTag     string
	AmazonHost    string
	Domain        string
	IndexNowKey   string
	PerBrand      int
	SameBrand     int
	ChartMinimum  int
	DetailBrands  int

	// Prefix is the subdirectory the site is served from, empty at a domain
	// root. A GitHub Pages project site lives under /<repo>/, where every
	// absolute path in the output would otherwise miss.
	Prefix string
}

// Meta is what every template can reach through .Site.
type Meta struct {
	BaseURL       string
	AdsenseClient string
	AdsenseSlot   string
	AmazonTag     string
	Disclosure    string
	PaintCount    int
	BrandCount    int
}

type common struct {
	Title   string
	Desc    string
	Path    string
	Image   string
	JSONLD  template.JS
	Site    Meta
	Finder  bool
	NoIndex bool
}

// unsearched are the ranges whose own pages will never earn a search click.
// They are craft and fine-art acrylics, and the people who buy them do not shop
// by paint name the way miniature painters do: every query this site has ranked
// for so far names a miniature range, and the craft ranges are 1,146 of the
// pages competing for the crawl.
//
// The pages stay on the site and stay linked. They are worth keeping as match
// targets — telling a Citadel painter which FolkArt bottle is closest is a real
// answer — they simply stop asking to be indexed, so what crawl budget a new
// site has goes to the pages that can rank. Reversing this is deleting a line.
//
// It doubles as the list of craft ranges, for the guide about them and for the
// warning a paint page carries when its first answer comes from one. That is
// not two ideas sharing a map by accident: these ranges are kept out of the
// index precisely because they are craft and fine-art lines rather than
// miniature ones, so a range joining this list is a range the warning should
// fire for.
var unsearched = map[string]bool{
	"folkart":      true,
	"apple-barrel": true,
	"golden":       true,
	"liquitex":     true,
	"arteza":       true,
}

type Link struct {
	URL  string
	From string
	To   string
	Note string
}

type Generator struct {
	cfg    Config
	paints []catalog.Paint
	tables []match.Table
	brands []catalog.Brand
	meta   Meta
	tmpl   map[string]*template.Template

	// coverage is filled in by chartPages and read by the index that lists
	// them, so 650 links stop being an undifferentiated wall: the one number
	// that decides whether a chart is worth opening is how much of the range it
	// actually answers. The brand-switching guide reads it too, which is why
	// guidePages runs after chartPages.
	coverage map[string]chartCover

	written int
}

func New(cfg Config, paints []catalog.Paint, tables []match.Table) (*Generator, error) {
	if cfg.PerBrand <= 0 {
		cfg.PerBrand = 3
	}
	if cfg.SameBrand <= 0 {
		cfg.SameBrand = 5
	}
	if cfg.DetailBrands <= 0 {
		cfg.DetailBrands = 12
	}
	if cfg.AmazonHost == "" {
		cfg.AmazonHost = "www.amazon.com.br"
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	// The prefix follows from the base URL, so the two can never disagree.
	if u, err := url.Parse(cfg.BaseURL); err == nil {
		cfg.Prefix = strings.TrimSuffix(u.Path, "/")
	}
	brands := catalog.Brands(paints)
	g := &Generator{
		cfg: cfg, paints: paints, tables: tables, brands: brands,
		meta: Meta{
			BaseURL:       cfg.BaseURL,
			AdsenseClient: cfg.AdsenseClient,
			AdsenseSlot:   cfg.AdsenseSlot,
			AmazonTag:     cfg.AmazonTag,
			Disclosure:    disclosure(cfg.AmazonHost),
			PaintCount:    len(paints),
			BrandCount:    len(brands),
		},
		tmpl:     map[string]*template.Template{},
		coverage: map[string]chartCover{},
	}
	for _, name := range []string{"home", "paint", "brand", "chart", "list", "about", "find", "privacy"} {
		t, err := template.New(name).Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		g.tmpl[name] = t
	}
	// One parse per guide, not one for all of them: every body file defines
	// "body", so a shared parse would leave whichever was read last standing and
	// print it under every other guide's title.
	for _, gd := range guides {
		t, err := template.New(gd.Slug).Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/guide.html", "tmpl/guides/"+gd.Slug+".html")
		if err != nil {
			return nil, fmt.Errorf("guide %s: %w", gd.Slug, err)
		}
		g.tmpl["guide:"+gd.Slug] = t
	}
	return g, nil
}

// Run writes every page and returns how many files it produced.
func (g *Generator) Run() (int, error) {
	if err := os.MkdirAll(g.cfg.Out, 0o755); err != nil {
		return 0, err
	}
	steps := []func() error{
		g.copyAssets, g.siteCard, g.home, g.paintPages, g.moved, g.brandPages,
		g.chartPages, g.indexes, g.guidePages, g.about, g.privacy, g.find, g.searchIndex, g.sitemap, g.wellKnown,
		g.verify,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return g.written, err
		}
	}
	return g.written, nil
}

func (g *Generator) write(rel string, body []byte) error {
	return g.writeRaw(rel, g.rebase(body))
}

// writeRaw is write without the path rewrite, for bytes that are not markup.
// Running rebase over a PNG would corrupt it the moment the site is built with
// a prefix.
func (g *Generator) writeRaw(rel string, body []byte) error {
	full := filepath.Join(g.cfg.Out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		return err
	}
	g.written++
	return nil
}

func (g *Generator) render(name, rel string, data any) error {
	var buf bytes.Buffer
	if err := g.tmpl[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	return g.write(rel, trimIndent(buf.Bytes()))
}

// roots are the absolute paths the output can contain: every internal link,
// plus the two files the client fetches by path. They are rewritten rather
// than templated so a path added later cannot silently escape the prefix.
var roots = []string{
	"/paint/", "/brand/", "/brands/", "/charts/", "/convert/", "/guides/", "/about/", "/privacy/",
	"/style.css", "/search.js", "/search.json", "/favicon.svg", "/sitemap.xml",
	"/find/", "/find.js",
}

// rebase moves every absolute path under the deployment prefix.
func (g *Generator) rebase(body []byte) []byte {
	if g.cfg.Prefix == "" || len(body) == 0 {
		return body
	}
	for _, r := range roots {
		body = bytes.ReplaceAll(body, []byte(`"`+r), []byte(`"`+g.cfg.Prefix+r))
	}
	return bytes.ReplaceAll(body, []byte(`href="/"`), []byte(`href="`+g.cfg.Prefix+`/"`))
}

// trimIndent drops template indentation, which is a third of the bytes on a
// page that is mostly a repeated list. It stops at <pre>, where whitespace is
// content rather than layout.
func trimIndent(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inPre := false
	for _, line := range bytes.Split(b, []byte("\n")) {
		if inPre {
			out = append(out, line...)
			out = append(out, '\n')
			if bytes.Contains(line, []byte("</pre>")) {
				inPre = false
			}
			continue
		}
		t := bytes.TrimLeft(line, " \t")
		if len(t) == 0 {
			continue
		}
		out = append(out, t...)
		out = append(out, '\n')
		if bytes.Contains(t, []byte("<pre")) && !bytes.Contains(t, []byte("</pre>")) {
			inPre = true
		}
	}
	return out
}

func (g *Generator) copyAssets() error {
	entries, err := assetFS.ReadDir("assets")
	if err != nil {
		return err
	}
	for _, e := range entries {
		b, err := assetFS.ReadFile("assets/" + e.Name())
		if err != nil {
			return err
		}
		if err := g.write(e.Name(), b); err != nil {
			return err
		}
	}
	return nil
}

// --- pages ---

func (g *Generator) home() error {
	type data struct {
		common
		Brands  []catalog.Brand
		Popular []Link
	}
	ld := fmt.Sprintf(`{"@context":"https://schema.org","@type":"WebSite","name":"Paint Equivalents","url":%q}`, g.cfg.BaseURL+"/")
	return g.render("home", "index.html", data{
		common: common{
			Title: "Miniature Paint Equivalents — cross-brand conversion by CIEDE2000",
			Desc: fmt.Sprintf("Find the closest equivalent of any miniature paint across %d ranges. %d paints matched by CIEDE2000, with the colour distance shown for every match.",
				len(g.brands), len(g.paints)),
			Path: "/", Site: g.meta, JSONLD: template.JS(ld),
		},
		Brands:  g.brands,
		Popular: g.popular(),
	})
}

// popularPairs are the conversions people actually search for. The home page
// links them and the brand-switching guide measures them, and it matters that
// both work from one list: the guide's argument is about the charts a reader
// is most likely to have already opened, not about a set of pairs chosen to
// make the argument look good.
var popularPairs = [][2]string{
	{"Citadel", "Vallejo"}, {"Vallejo", "Citadel"},
	{"Citadel", "The Army Painter"}, {"The Army Painter", "Vallejo"},
	{"Citadel", "Scale75"}, {"Vallejo", "The Army Painter"},
	{"Citadel", "P3"}, {"Tamiya", "Vallejo"},
}

// popular seeds the home page with the pairs people actually search for,
// restricted to brands this build really has.
func (g *Generator) popular() []Link {
	have := map[string]catalog.Brand{}
	for _, b := range g.brands {
		have[b.Name] = b
	}
	var out []Link
	for _, w := range popularPairs {
		a, okA := have[w[0]]
		b, okB := have[w[1]]
		if !okA || !okB {
			continue
		}
		out = append(out, Link{URL: chartPath(a.Slug, b.Slug), From: a.Name, To: b.Name})
	}
	return out
}

// How much of a paint page's meta description Google shows before it truncates,
// and how many brands are worth naming inside it even when they all fit.
const (
	descRunes  = 155
	descBrands = 3
)

// paintDesc writes the meta description for one paint, naming as many brands as
// the snippet has room for rather than one. The single nearest match is
// whatever range happens to sit closest in Lab, and that is often a craft or
// scale paint nobody reaches for on a miniature — a snippet answering "Corvus
// Black" with "Creature Brown" reads as a wrong answer and the click goes
// elsewhere. Listing several puts a range the reader stocks in front of them
// without ranking by anything but distance.
//
// The budget is what Google shows before it truncates, and a name like
// "Monument AdeptiCon Spray-Team Satin Black" costs two ordinary ones, so this
// counts characters rather than matches. Truncation mid-name is the thing to
// avoid: it is invisible in the HTML and only shows up in the search result.
func paintDesc(full string, ranges int, cross []match.BrandMatches) string {
	desc := fmt.Sprintf("%s equivalents in %d ranges.", full, ranges)
	for i, c := range cross {
		if i == descBrands {
			break
		}
		sep := ", "
		if i == 0 {
			sep = " Closest: "
		}
		m := fmt.Sprintf("%s%s %s ΔE %.1f", sep, c.Brand, c.Best[0].Paint.Name, c.Best[0].DE)
		// +1 leaves room for the full stop that closes the list.
		if len([]rune(desc))+len([]rune(m))+1 > descRunes {
			break
		}
		desc += m
	}
	if len(cross) > 0 && strings.Contains(desc, "Closest: ") {
		desc += "."
	}
	return desc
}

func (g *Generator) paintPages() error {
	// A range under the chart minimum never gets a chart page. Without this the
	// blocks below still offer "full chart" links for those ranges, and every
	// paint in a small range ships a handful of 404s.
	charted := map[string]map[string]bool{}
	for _, from := range g.brands {
		to := map[string]bool{}
		for _, other := range g.brands {
			if other.Name != from.Name && g.charted(from, other) {
				to[other.Slug] = true
			}
		}
		charted[from.Slug] = to
	}

	names := newLabels(g.paints)
	// Brands biggest first. The second question on a paint page has to name one
	// other range, and nothing in the data says which range readers own; the
	// widest catalog is the closest honest stand-in for the one a shop stocks.
	bySize := append([]catalog.Brand(nil), g.brands...)
	sort.Slice(bySize, func(i, j int) bool { return bySize[i].Count > bySize[j].Count })

	boxed := boxedRanges(g.paints)

	for i, p := range g.paints {
		t := g.tables[i]
		full := names.product(p)
		// Only the nearest ranges get a full block. The rest still appear, as
		// one row each: a reader scrolling 28 headings finds nothing, and the
		// page has to stay light enough to load fast on a phone.
		detailed, rest := t.Cross, []match.BrandMatches(nil)
		if len(detailed) > g.cfg.DetailBrands {
			rest = detailed[g.cfg.DetailBrands:]
			detailed = detailed[:g.cfg.DetailBrands]
		}
		type data struct {
			common
			Name     string
			Buy      string
			BuyBest  string
			BestName string
			Summary  string
			FAQ      []qa
			Paint    catalog.Paint
			Table    match.Table
			Detailed []match.BrandMatches
			Rest     []match.BrandMatches
			Charts   map[string]bool
			// Set is the boxed set behind the closest match, when there is one.
			// It is the only link on this page worth more than a pot, and it
			// belongs to the range the reader is being sent to rather than the
			// one they already have open.
			Set *setLink
			// CraftFirst is the one warning on this page about the answer rather
			// than about the paint: the block at the top of the list is a craft
			// or fine-art range. It is decided here and not in the markup
			// because the template would have to index an empty slice to ask.
			CraftFirst bool
		}
		var buyBest, bestName string
		var set *setLink
		if len(t.Cross) > 0 {
			best := *t.Cross[0].Best[0].Paint
			bestName = names.product(best)
			buyBest = g.buy(bestName)
			set = g.setFor(best, boxed)
		}
		desc := paintDesc(full, len(g.brands)-1, t.Cross)

		big := ""
		for _, b := range bySize {
			if b.Name != p.Brand {
				big = b.Name
				break
			}
		}
		faq := ask(full, p.Brand, big, t.Cross)
		ld := graph(g.crumbList(p.Brand, "/brand/"+p.BrandSlug+"/", p.Name))
		if len(faq) > 0 {
			ld = graph(g.crumbList(p.Brand, "/brand/"+p.BrandSlug+"/", p.Name), faqList(faq))
		}

		if err := g.paintCard(p, t); err != nil {
			return err
		}

		err := g.render("paint", strings.TrimPrefix(p.URL(), "/")+"index.html", data{
			common: common{
				Title: fmt.Sprintf("%s equivalents — closest paint in every range", full),
				// The hex used to sit here and bought nothing: nobody searches a
				// Citadel pot by hex, and the matches need the room.
				Desc: desc,
				Path: p.URL(), Image: g.cfg.BaseURL + p.URL() + "card.png",
				Site: g.meta, JSONLD: ld, NoIndex: unsearched[p.BrandSlug],
			},
			Paint: p, Table: t, Detailed: detailed, Rest: rest,
			Name: strings.TrimPrefix(full, p.Brand+" "),
			Buy:  g.buy(full), BuyBest: buyBest, BestName: bestName, Set: set,
			Summary: summarise(p, t), FAQ: faq, Charts: charted[p.BrandSlug],
			// Not on the craft pages themselves, where the nearest range is
			// usually another craft one and the reader is already in the aisle.
			CraftFirst: !unsearched[p.BrandSlug] && len(t.Cross) > 0 && unsearched[t.Cross[0].Slug],
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// moved answers the paths that folding duplicate paints left empty. Static
// hosting cannot serve a 301, so the page says the same thing the header would:
// a canonical pointing at the paint that absorbed this one, and a refresh for
// the reader who followed an old link. Deleting an indexed URL outright throws
// away whatever standing it had and spends crawl budget on 404s, which this
// site has none to spare.
func (g *Generator) moved() error {
	for from, to := range catalog.Vacated(g.paints) {
		page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Moved to %s</title>
<link rel="canonical" href="%s">
<meta http-equiv="refresh" content="0;url=%s">
</head>
<body><p>This paint is now listed at <a href="%s">%s</a>.</p></body>
</html>
`, to, g.cfg.BaseURL+to, to, to, to)
		if err := g.write(strings.TrimPrefix(from, "/")+"index.html", []byte(page)); err != nil {
			return err
		}
	}
	return nil
}

// siteCard is the share image for every page that is not about one paint. It is
// built from real catalog colours, one per hue band, so the card shows the
// range the site actually covers.
func (g *Generator) siteCard() error {
	seen := map[string]bool{}
	var picked []stdcolor.RGBA
	for _, p := range g.paints {
		h := color.Hue(p.Lab())
		if h == "neutral" || seen[h] || len(picked) == 6 {
			continue
		}
		seen[h] = true
		picked = append(picked, stdcolor.RGBA{R: uint8(p.R), G: uint8(p.G), B: uint8(p.B), A: 0xff})
	}
	if len(picked) == 0 {
		return nil
	}
	card, err := ogImage(picked[0], picked[1:])
	if err != nil {
		return err
	}
	return g.writeRaw("card.png", card)
}

// paintCard writes the share image next to the page it belongs to, showing the
// paint against the five ranges that come closest to it.
func (g *Generator) paintCard(p catalog.Paint, t match.Table) error {
	var near []stdcolor.RGBA
	for _, b := range t.Cross {
		if len(near) == 5 {
			break
		}
		m := b.Best[0].Paint
		near = append(near, stdcolor.RGBA{R: uint8(m.R), G: uint8(m.G), B: uint8(m.B), A: 0xff})
	}
	card, err := ogImage(stdcolor.RGBA{R: uint8(p.R), G: uint8(p.G), B: uint8(p.B), A: 0xff}, near)
	if err != nil {
		return err
	}
	return g.writeRaw(strings.TrimPrefix(p.URL(), "/")+"card.png", card)
}

// summarise turns the numbers already on the page into the sentence a person
// would say about them: what the colour is, how well it is covered elsewhere,
// and whether the ranges that miss it miss it badly. Every clause is read off
// this paint's own measurements, so no two pages get the same paragraph.
func summarise(p catalog.Paint, t match.Table) string {
	lab := color.Lab{L: p.LabL, A: p.LabA, B: p.LabB}
	s := p.Brand + " " + p.Name + " is " + color.Describe(lab) + "."
	if len(t.Cross) == 0 {
		return s
	}

	near, usable := 0, 0
	for _, b := range t.Cross {
		switch de := b.Best[0].DE; {
		case de < 2:
			near++
			usable++
		case de < 5:
			usable++
		}
	}
	first := t.Cross[0].Best[0]

	switch {
	case near > 1:
		s += fmt.Sprintf(" %d ranges carry a near-perfect stand-in for it, the closest being %s %s at ΔE %.1f.",
			near, t.Cross[0].Brand, first.Paint.Name, first.DE)
	case near == 1:
		s += fmt.Sprintf(" Only one range matches it closely enough to swap without thinking: %s %s, ΔE %.1f.",
			t.Cross[0].Brand, first.Paint.Name, first.DE)
	default:
		s += fmt.Sprintf(" Nothing matches it exactly. The nearest is %s %s at ΔE %.1f — %s.",
			t.Cross[0].Brand, first.Paint.Name, first.DE, color.Quality(first.DE))
	}
	if miss := len(t.Cross) - usable; miss > 0 {
		s += fmt.Sprintf(" %d of the %d other ranges have nothing within ΔE 5, so a substitution from those will read as a different colour.",
			miss, len(t.Cross))
	}
	return s
}

// crumbs is the markup that makes a result print "Paint Equivalents > Conversion
// charts > Citadel to Vallejo" instead of a bare URL. Only the parent is linked;
// the leaf is the page itself, which schema.org wants left without an item.
func (g *Generator) crumbs(parent, parentURL, leaf string) template.JS {
	return graph(g.crumbList(parent, parentURL, leaf))
}

func (g *Generator) crumbList(parent, parentURL, leaf string) string {
	return fmt.Sprintf(`{"@type":"BreadcrumbList","itemListElement":[`+
		`{"@type":"ListItem","position":1,"name":%q,"item":%q},`+
		`{"@type":"ListItem","position":2,"name":%q}]}`,
		parent, g.cfg.BaseURL+parentURL, leaf)
}

// graph wraps the objects a page describes in the single block it carries. A
// page that says two things about itself says them in one script tag; two tags
// each with their own @context parse, but only the first is reliably read.
func graph(objs ...string) template.JS {
	return template.JS(`{"@context":"https://schema.org","@graph":[` + strings.Join(objs, ",") + `]}`)
}

// qa is one question a paint page answers in the words it was asked in.
type qa struct{ Q, A string }

// ask writes the questions the search results for these pages are actually made
// of. "What is the closest equivalent to X" is a query, a People-also-ask entry
// and the shape an AI overview quotes, and the competing sites that outrank this
// one answer it in a sentence near the top of the page while this one buried the
// answer in a table. The second question names the largest other range, because
// "the Vallejo equivalent of X" is asked far more often than "the nearest paint
// in any of 28 ranges to X". Both answers are read off the same measurements the
// table below them shows, so neither can drift from the page it sits on.
func ask(full, brand, big string, cross []match.BrandMatches) []qa {
	if len(cross) == 0 {
		return nil
	}
	best := cross[0].Best[0]
	answer := fmt.Sprintf("%s %s is the closest equivalent to %s in any other range, at ΔE %.1f — %s.",
		cross[0].Brand, best.Paint.Name, full, best.DE, verdict(best.DE))
	// The single nearest paint is whatever sits closest in Lab, and that is
	// often a craft range nobody reaches for on a miniature. Naming the runners
	// up costs one clause and puts a range the reader stocks in the answer.
	if len(cross) > 1 {
		var next []string
		for _, c := range cross[1:] {
			if len(next) == 2 {
				break
			}
			next = append(next, fmt.Sprintf("%s %s (ΔE %.1f)", c.Brand, c.Best[0].Paint.Name, c.Best[0].DE))
		}
		answer += " Next nearest: " + strings.Join(next, " and ") + "."
	}
	out := []qa{{
		Q: "What is the closest equivalent to " + full + "?",
		A: answer,
	}}
	for _, c := range cross {
		if c.Brand != big {
			continue
		}
		m := c.Best[0]
		out = append(out, qa{
			Q: fmt.Sprintf("What is the %s alternative to %s?", big, full),
			A: fmt.Sprintf("The nearest %s paint is %s, ΔE %.1f — %s. The full %s to %s conversion chart lists the rest.",
				big, m.Paint.Name, m.DE, verdict(m.DE), brand, big),
		})
		break
	}
	return out
}

// verdict turns a distance into what a painter can do with it. Under ΔE 5 the
// swap survives on a painted model, so the answer may call it a substitute;
// above it the honest answer is that it is not one.
func verdict(de float64) string {
	if de < 5 {
		q := color.Quality(de)
		article := "a "
		if strings.ContainsRune("aeiou", rune(q[0])) {
			article = "an "
		}
		return article + q + " substitute"
	}
	return fmt.Sprintf("%s, close enough to stand in only where the colour is not the point", color.Quality(de))
}

func faqList(qs []qa) string {
	items := make([]string, 0, len(qs))
	for _, q := range qs {
		items = append(items, fmt.Sprintf(
			`{"@type":"Question","name":%q,"acceptedAnswer":{"@type":"Answer","text":%q}}`, q.Q, q.A))
	}
	return `{"@type":"FAQPage","mainEntity":[` + strings.Join(items, ",") + `]}`
}

func (g *Generator) brandPages() error {
	for _, b := range g.brands {
		var own []catalog.Paint
		for _, p := range g.paints {
			if p.Brand == b.Name {
				own = append(own, p)
			}
		}
		var charts []Link
		for _, other := range g.brands {
			if other.Name == b.Name || !g.charted(b, other) {
				continue
			}
			charts = append(charts, Link{URL: chartPath(b.Slug, other.Slug), From: b.Name, To: other.Name})
		}
		type data struct {
			common
			Brand  catalog.Brand
			Paints []catalog.Paint
			Charts []Link
			Sets   []setLink
		}
		err := g.render("brand", "brand/"+b.Slug+"/index.html", data{
			common: common{
				Title: fmt.Sprintf("%s paint conversions — every colour, matched across ranges", b.Name),
				Desc: fmt.Sprintf("All %d %s paints with their closest equivalents in other ranges, matched by CIEDE2000.",
					b.Count, b.Name),
				Path: "/brand/" + b.Slug + "/", Site: g.meta,
				JSONLD: g.crumbs("Paint ranges", "/brands/", b.Name),
			},
			Brand: b, Paints: own, Charts: charts, Sets: g.brandSets(b, own),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// chartSummary says what the table underneath it adds up to: how much of one
// range a painter can really replace with another, and where the gaps sit. The
// hue that misses most is the useful part — it names the shelf they still have
// to buy from, which no other page on the site can tell them.
func chartSummary(from, to catalog.Brand, rows []match.Pair) string {
	if len(rows) == 0 {
		return ""
	}
	c := coverage(rows)
	missed := map[string]int{}
	for _, r := range rows {
		if r.DE >= 5 {
			missed[color.Hue(r.From.Lab())]++
		}
	}

	// "an equivalent in X" rather than "an X paint", because brands whose name
	// carries its own article ("The Army Painter") read as nonsense otherwise.
	s := fmt.Sprintf("%d of the %d %s colours below have an equivalent in %s within ΔE 5, the distance at which a substitution stops being obvious on a painted model. %d sit within ΔE 2 and are interchangeable outright. Across the whole range the typical distance is ΔE %.1f.",
		c.Usable, c.Total, from.Name, to.Name, c.Near, c.Median)

	if hue, n := commonest(missed); n > 2 {
		s += fmt.Sprintf(" The %d gaps are not spread evenly: %d of them are %ss, so that is the corner of the %s range %s does not answer.",
			c.Total-c.Usable, n, hue, from.Name, to.Name)
	}
	return s
}

// chartCover is what one conversion chart adds up to, kept as numbers rather
// than as the sentence the index prints beside it. Two readers need it: the
// index, which shows the one figure that separates a chart worth opening from
// one that is mostly gaps, and the brand-switching guide, which sets the two
// directions of a pair against each other. Two formatted strings cannot be
// compared, and measuring the same chart twice invites the two answers to drift.
type chartCover struct {
	// Total is one row per colour in the source range.
	Total int
	// Usable is the rows inside ΔE 5, where the substitution stops being
	// obvious on a painted model, and Near the rows inside ΔE 2, where the two
	// pots are interchangeable outright. Near counts inside Usable.
	Usable int
	Near   int
	// Median is where the middle row of the chart sits. It says what the chart
	// is like to use in a way the counts do not: a median past 5 means the
	// typical row is already a miss.
	Median float64
}

func coverage(rows []match.Pair) chartCover {
	c := chartCover{Total: len(rows)}
	if len(rows) == 0 {
		return c
	}
	des := make([]float64, 0, len(rows))
	for _, r := range rows {
		des = append(des, r.DE)
		if r.DE < 5 {
			c.Usable++
		}
		if r.DE < 2 {
			c.Near++
		}
	}
	sort.Float64s(des)
	c.Median = des[len(des)/2]
	return c
}

// note is the coverage figure as the chart index prints it beside each link.
func (c chartCover) note() string {
	return fmt.Sprintf("%d of %d matched", c.Usable, c.Total)
}

func commonest(counts map[string]int) (string, int) {
	best, n := "", 0
	for k, v := range counts {
		// Ties break on the name so the sentence is stable between builds.
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best, n
}

// charted keeps the chart pages to ranges with enough paints to make a table
// worth reading; a two-colour range produces a page that says nothing.
func (g *Generator) charted(a, b catalog.Brand) bool {
	return a.Count >= g.cfg.ChartMinimum && b.Count >= g.cfg.ChartMinimum
}

func (g *Generator) chartPages() error {
	for _, from := range g.brands {
		for _, to := range g.brands {
			if from.Name == to.Name || !g.charted(from, to) {
				continue
			}
			rows := match.Chart(g.paints, g.tables, from.Name, to.Name)
			if len(rows) == 0 {
				continue
			}
			type data struct {
				common
				Summary string
				From    catalog.Brand
				To      catalog.Brand
				Rows    []match.Pair
				Sets    []setLink
				// Craft says one side of this chart is a craft or fine-art
				// range. The wash, air and metallic warnings are deliberately
				// kept off the charts, because a chart is brand to brand and
				// cannot know what kind of pot the reader wants. This one it
				// does know: both columns are named at the top of the page.
				Craft bool
			}
			// The range being converted *to* comes first: a reader on this page
			// already owns the left column and is deciding whether the right one
			// is worth buying into.
			var sets []setLink
			if g.cfg.AmazonTag != "" {
				sets = []setLink{
					{Name: to.Name, URL: g.buySet(to.Name, "")},
					{Name: from.Name, URL: g.buySet(from.Name, "")},
				}
			}
			path := chartPath(from.Slug, to.Slug)
			g.coverage[path] = coverage(rows)
			err := g.render("chart", strings.TrimPrefix(path, "/")+"index.html", data{
				common: common{
					Title: fmt.Sprintf("%s to %s paint conversion chart (%d paints)", from.Name, to.Name, len(rows)),
					Desc: fmt.Sprintf("Complete %s to %s conversion chart: %d paints with their closest %s equivalent and the colour distance for each.",
						from.Name, to.Name, len(rows), to.Name),
					Path: path, Site: g.meta,
					JSONLD: g.crumbs("Conversion charts", "/charts/", from.Name+" to "+to.Name),
				},
				From: from, To: to, Rows: rows, Sets: sets,
				Summary: chartSummary(from, to, rows),
				Craft:   unsearched[from.Slug] || unsearched[to.Slug],
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *Generator) indexes() error {
	type data struct {
		common
		Heading string
		Lede    string
		Links   []Link
	}

	var brandLinks []Link
	for _, b := range g.brands {
		brandLinks = append(brandLinks, Link{
			URL: "/brand/" + b.Slug + "/", From: b.Name,
			Note: fmt.Sprintf("%d paints", b.Count),
		})
	}
	err := g.render("list", "brands/index.html", data{
		common: common{
			Title: fmt.Sprintf("All %d miniature paint ranges", len(g.brands)),
			Desc:  "Every paint range covered, with the number of colours in each.",
			Path:  "/brands/", Site: g.meta,
		},
		Heading: "Paint ranges", Lede: "Every range in the database. Open one to browse its colours or jump to a conversion chart.",
		Links: brandLinks,
	})
	if err != nil {
		return err
	}

	var chartLinks []Link
	for _, from := range g.brands {
		for _, to := range g.brands {
			if from.Name == to.Name || !g.charted(from, to) {
				continue
			}
			// An empty coverage entry means chartPages found nothing to tabulate
			// and wrote no page, so linking to it would be a dead end.
			path := chartPath(from.Slug, to.Slug)
			c, ok := g.coverage[path]
			if !ok {
				continue
			}
			chartLinks = append(chartLinks, Link{URL: path, From: from.Name, To: to.Name, Note: c.note()})
		}
	}
	sort.Slice(chartLinks, func(i, j int) bool {
		if chartLinks[i].From != chartLinks[j].From {
			return chartLinks[i].From < chartLinks[j].From
		}
		return chartLinks[i].To < chartLinks[j].To
	})
	return g.render("list", "charts/index.html", data{
		common: common{
			Title: "Paint conversion charts — every range to every range",
			Desc:  "Complete cross-brand conversion charts for miniature paints, matched by CIEDE2000.",
			Path:  "/charts/", Site: g.meta,
		},
		Heading: "Conversion charts", Lede: "Full tables, one row per paint, with the colour distance for every match. The count beside each chart is how many of the source paints have a stand-in close enough to pass on a model (ΔE under 5), so a chart that answers most of a range is worth opening before one that answers a fraction of it.",
		Links: chartLinks,
	})
}

// round2 keeps the search payload small; two decimals on a Lab axis is far
// finer than the sampling error already in the colour values.
func round2(f float64) float64 { return math.Round(f*100) / 100 }

func (g *Generator) find() error {
	type data struct{ common }
	return g.render("find", "find/index.html", data{common{
		Title: "What paint is this colour? Find a miniature paint from a hex code",
		Desc: fmt.Sprintf("Pick a colour or paste a hex code and rank all %d miniature paints against it by CIEDE2000. Runs in the browser.",
			len(g.paints)),
		Path: "/find/", Site: g.meta, Finder: true,
	}})
}

func (g *Generator) about() error {
	type data struct{ common }
	return g.render("about", "about/index.html", data{common{
		Title: "About the data and the colour matching",
		Desc:  "How the paint matching works, what it cannot tell you, and the licence the paint data is published under.",
		Path:  "/about/", Site: g.meta,
	}})
}

// privacy is a policy page, not a nicety: AdSense will not approve a site that
// does not disclose what its advertising partners do with cookies, and the
// affiliate disclosure is required by the Associates agreement.
func (g *Generator) privacy() error {
	type data struct{ common }
	return g.render("privacy", "privacy/index.html", data{common{
		Title: "Privacy policy",
		Desc:  "What this site collects, who serves its ads, and how affiliate links work.",
		Path:  "/privacy/", Site: g.meta,
	}})
}

// --- machine-readable output ---

// searchRow is deliberately terse: it is downloaded whole by every visitor who
// uses the search box or the colour finder. L/A/D carry the Lab value so the
// finder can rank the whole catalogue client-side; "d" rather than "b" because
// "b" is already the brand.
type searchRow struct {
	N string  `json:"n"`
	B string  `json:"b"`
	H string  `json:"h"`
	I string  `json:"i"`
	U string  `json:"u"`
	L float64 `json:"l"`
	A float64 `json:"a"`
	D float64 `json:"d"`
}

func (g *Generator) searchIndex() error {
	rows := make([]searchRow, 0, len(g.paints))
	for _, p := range g.paints {
		rows = append(rows, searchRow{
			N: p.Name, B: p.Brand, H: p.Hex, I: p.Ink(), U: p.URL(),
			L: round2(p.LabL), A: round2(p.LabA), D: round2(p.LabB),
		})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return g.write("search.json", b)
}

func (g *Generator) sitemap() error {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	add := func(path string, priority string) {
		fmt.Fprintf(&b, "<url><loc>%s%s</loc><priority>%s</priority></url>\n",
			g.cfg.BaseURL, path, priority)
	}
	add("/", "1.0")
	add("/find/", "0.9")
	add("/brands/", "0.8")
	add("/charts/", "0.8")
	// The guides are the only written pages here, so they get the priority the
	// generated ones cannot argue for.
	add("/guides/", "0.9")
	for _, gd := range guides {
		add("/guides/"+gd.Slug+"/", "0.9")
	}
	add("/about/", "0.3")
	add("/privacy/", "0.1")
	for _, br := range g.brands {
		add("/brand/"+br.Slug+"/", "0.7")
		for _, to := range g.brands {
			if br.Name != to.Name && g.charted(br, to) {
				add(chartPath(br.Slug, to.Slug), "0.9")
			}
		}
	}
	for _, p := range g.paints {
		// A page carrying noindex has no business in the sitemap: submitting it
		// asks Google to spend a crawl on a page that tells it to go away.
		if unsearched[p.BrandSlug] {
			continue
		}
		add(p.URL(), "0.6")
	}
	b.WriteString("</urlset>\n")
	return g.write("sitemap.xml", b.Bytes())
}

func (g *Generator) wellKnown() error {
	robots := "User-agent: *\nAllow: /\n\nSitemap: " + g.cfg.BaseURL + "/sitemap.xml\n"
	if err := g.write("robots.txt", []byte(robots)); err != nil {
		return err
	}
	// GitHub Pages runs Jekyll over the output unless told not to, which would
	// swallow any directory starting with an underscore.
	if err := g.write(".nojekyll", nil); err != nil {
		return err
	}
	if g.cfg.Domain != "" {
		if err := g.write("CNAME", []byte(g.cfg.Domain+"\n")); err != nil {
			return err
		}
	}
	// IndexNow proves control of the directory by serving the key from it, and
	// is the only search-engine submission that needs no account.
	if g.cfg.IndexNowKey != "" {
		if err := g.write(g.cfg.IndexNowKey+".txt", []byte(g.cfg.IndexNowKey)); err != nil {
			return err
		}
	}
	if g.cfg.AdsenseClient != "" {
		pub := strings.TrimPrefix(g.cfg.AdsenseClient, "ca-")
		line := fmt.Sprintf("google.com, %s, DIRECT, f08c47fec0942fa0\n", pub)
		if err := g.write("ads.txt", []byte(line)); err != nil {
			return err
		}
	}
	return nil
}

// buy is an Amazon search link rather than a product link: the catalogue has
// no ASINs, and a search for the exact paint name survives a product going out
// of stock, which a dead product link does not. Empty when no tag is set, so
// the markup carries no affiliate machinery until there is an account behind
// it.
func (g *Generator) buy(product string) string {
	if g.cfg.AmazonTag == "" {
		return ""
	}
	// A tag is issued by one marketplace and earns nothing on any other, so the
	// store the links point at has to follow the tag. The qualifier follows the
	// store rather than the page: the catalogue is in English either way, but
	// "paint" matches almost nothing on the Brazilian store.
	word := "paint"
	if strings.HasSuffix(g.cfg.AmazonHost, ".com.br") {
		word = "tinta"
	}
	q := url.Values{}
	q.Set("k", product+" "+word)
	q.Set("tag", g.cfg.AmazonTag)
	return "https://" + g.cfg.AmazonHost + "/s?" + q.Encode()
}

// setLink is one boxed-set search: the range it covers and where it points.
type setLink struct{ Name, URL string }

// buySet is buy() one level up: a set instead of a single pot. A brand page and
// a conversion chart are read by someone choosing a *range*, not a colour, and
// a set is the listing that answers that — at ten to twenty times the basket of
// one pot, on the same visit. The query names the range and lets the store list
// whatever boxes it actually stocks; naming a kit this catalogue has no record
// of would send the reader to an empty result page.
func (g *Generator) buySet(brand, rng string) string {
	word := "set"
	if strings.HasSuffix(g.cfg.AmazonHost, ".com.br") {
		word = "kit"
	}
	return g.buy(strings.TrimSpace(brand+" "+rng) + " " + word)
}

// setMinimum is the size below which a sub-range is not offered as a set. A
// label with a handful of colours is not boxed by anyone, and a search that
// returns nothing spends the click for nothing. The number is a judgement, not
// a measurement: it is low enough to keep Citadel Shade (16) out and every
// range a starter box exists for in.
//
// ponytail: fixed floor, per-brand tuning only if the click data ever says so.
const setMinimum = 20

// rangeName reduces a range field to the name of the range, and reports whether
// anything is left. Tom Color's 265 rows arrive as "s", "s - UP" and
// "s - Primer": upstream splits "Tom Colors …" in the wrong place and this site
// receives the tail. The rule for the leftover is that a word of one or two
// letters standing in front of a dash is not part of a range name — which is
// narrow enough to leave "Mr Color" alone, where the short word is the name.
// What survives with no word of three letters in it is not a range name at all.
// Whitespace is squeezed on the way through because "Warfront  Range" is in the
// data too.
func rangeName(r string) (string, bool) {
	words := strings.Fields(r)
	if len(words) > 2 && len(words[0]) < 3 && isSeparator(words[1]) {
		words = words[2:]
	}
	for _, w := range words {
		if len(w) >= 3 {
			return strings.Join(words, " "), true
		}
	}
	return "", false
}

func isSeparator(w string) bool {
	switch w {
	case "-", "–", "—", ":", "/", "·":
		return true
	}
	return false
}

// sellable filters the labels there is no box to sell behind, which a paid
// click would spend on an empty result page. On top of the labels that are not
// range names it drops the discontinued ones, which are exactly what nobody can
// still buy — the one difference from what a match row prints, where
// "discontinued" is the most useful word on the line.
func sellable(r string) (string, bool) {
	if strings.Contains(strings.ToLower(r), "discontinued") {
		return "", false
	}
	return rangeName(r)
}

// tmplFuncs is what the markup can call. rangeOf is here rather than in the
// page data because every match row needs it and the rows are pointers into one
// shared catalog: precomputing it would copy a string onto ten million rankings
// to print a few hundred thousand of them.
var tmplFuncs = template.FuncMap{"rangeOf": rangeOf, "isWash": isWash, "isAir": isAir, "isMetal": isMetal}

// rangeOf is the range label a match row shows for a suggested paint. Until
// this existed the lists gave brand, name and code and nothing else, so a
// reader replacing a Base could be handed a wash, a spray or an airbrush paint
// with no warning on the line — the half of a paint's name that the colour
// distance cannot read. The paint's own page still lists every label it ships
// under; a row gets the first one, the way product() qualifies a title, because
// a list is read down the left edge and "Air / Base / Layer" is a paragraph.
func rangeOf(p catalog.Paint) string {
	r, _ := rangeName(trimBrand(p))
	return r
}

// subRanges counts how many of these paints carry each range label. A paint
// sold under several labels counts for each of them, because each label is a
// separate product: one page, two ranges, two things you could be holding.
//
// keep decides which labels survive, because the callers disagree about
// discontinued ones: there is no boxed set to sell behind a dead range, and
// there is very much a substitute to look for inside one.
func subRanges(own []catalog.Paint, keep func(string) (string, bool)) map[string]int {
	count := map[string]int{}
	for _, p := range own {
		for _, r := range labelsOf(p, keep) {
			count[r]++
		}
	}
	return count
}

// labelsOf splits one paint's range field into the labels it ships under. It is
// separate from subRanges because the airbrush guide needs the labels of a
// paint together rather than a count across paints: what it asks is whether the
// same page carries an air label and a brush label at once, and a tally cannot
// answer that.
func labelsOf(p catalog.Paint, keep func(string) (string, bool)) []string {
	var out []string
	for _, r := range strings.Split(p.Range, catalog.RangeSep) {
		// Per part, the way trimBrand does it for a single label: some
		// ranges repeat the maker's name and would otherwise be searched
		// for as "Vallejo Vallejo Model Color".
		r = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r), p.Brand))
		if r, ok := keep(r); ok {
			out = append(out, r)
		}
	}
	return out
}

// isWash reports whether a range label names a shading product — a wash, a
// shade, an ink, a dip. Nothing in the catalogue marks a pot as transparent, so
// the label is the only evidence there is, which is the whole argument of the
// washes guide: it is also the only warning the reader gets.
//
// "wash" and "shade" match anywhere, because Quickshade and Greenshade need
// them to. "ink" and "dip" have to start a word, or every Pink range in the
// catalogue joins the list.
func isWash(label string) bool {
	l := strings.ToLower(label)
	if strings.Contains(l, "wash") || strings.Contains(l, "shade") {
		return true
	}
	for _, w := range strings.Fields(strings.ReplaceAll(l, "-", " ")) {
		if strings.HasPrefix(w, "ink") || strings.HasPrefix(w, "dip") {
			return true
		}
	}
	return false
}

// isAir reports whether a range label carries the word Air. It deliberately
// does not report whether the pot is thinned for an airbrush, because the label
// does not say: Citadel, Vallejo and The Army Painter use Air for the tool,
// while AK Interactive uses it for the subject — aircraft, as against its AFV,
// Figures and Naval ranges, which are armour, people and ships. Both meanings
// are in this catalogue under the same three letters, and that ambiguity is the
// airbrush guide's argument rather than a defect to filter away here.
//
// The word has to stand alone, so that Aircraft Spray — a can of aircraft
// colours, neither an airbrush line nor a subject this test should claim — does
// not qualify. Airbrush may start a word, for Vallejo's Premium Airbrush Color.
func isAir(label string) bool {
	for _, w := range strings.Fields(strings.ReplaceAll(strings.ToLower(label), "-", " ")) {
		w = strings.Trim(w, "()")
		if w == "air" || strings.HasPrefix(w, "airbrush") {
			return true
		}
	}
	return false
}

// isMetal reports whether a range label says the pots in it are metallic. One
// substring, because one substring is all the evidence there is: "metal" covers
// Metal Color, Mr Metallic Color GX, Chameleon Colorshift Metallic and every
// other label here that uses the word, and nothing in this catalogue carries it
// without meaning it.
//
// What it deliberately does not do is guess. Pearl and colour-shift ranges are
// the same optics under another word, and Scale75's Liquid Gold is a metallic
// range whose label never says so — but "gold" and "pearl" are colour names at
// least as often as they are product classes, and a warning that fires on
// Pearl White would be worth less than no warning. The gap is not a defect to
// widen the rule over: it is the metallics guide's argument, which is that the
// label is the only warning a match row carries and for metallics it is usually
// missing. Citadel files its metals in Base, Layer, Dry, Air and Spray.
func isMetal(label string) bool {
	return strings.Contains(strings.ToLower(label), "metal")
}

// brandSets lists the sets worth offering for a brand, largest sub-range first.
// Brands whose range field is just the brand name, and brands whose labels are
// all too small, fall back to one search for the brand.
func (g *Generator) brandSets(b catalog.Brand, own []catalog.Paint) []setLink {
	if g.cfg.AmazonTag == "" {
		return nil
	}
	count := subRanges(own, sellable)
	var ranges []string
	for r, n := range count {
		if n >= setMinimum {
			ranges = append(ranges, r)
		}
	}
	// Ties break on the name so the page is identical between builds.
	sort.Slice(ranges, func(i, j int) bool {
		if count[ranges[i]] != count[ranges[j]] {
			return count[ranges[i]] > count[ranges[j]]
		}
		return ranges[i] < ranges[j]
	})
	if len(ranges) == 0 {
		return []setLink{{Name: b.Name, URL: g.buySet(b.Name, "")}}
	}
	// Six is where a row of buttons stops being a choice and starts being a
	// wall; the ranges past it are the ones nobody starts with anyway.
	if len(ranges) > 6 {
		ranges = ranges[:6]
	}
	sets := make([]setLink, 0, len(ranges))
	for _, r := range ranges {
		sets = append(sets, setLink{Name: setName(b.Name, r), URL: g.buySet(b.Name, r)})
	}
	return sets
}

// setName is the button text for a boxed set. The markup follows it with the
// word "set", and two of The Army Painter's labels are called Speedpaint Set,
// which came out as "Speedpaint Set set". Only the label the reader sees is
// cleaned: the search keeps the words the store files the product under, and
// dropping one of them there would return a different shelf.
func setName(brand, label string) string {
	return strings.TrimSpace(brand + " " + strings.TrimSpace(strings.Replace(label, " Set", "", 1)))
}

// brandRange names one label of one brand, which is the unit a boxed set is
// sold as: a box is somebody's Game Color, never Game Color on its own and
// never a brand entire. Both halves are the key for that reason.
type brandRange struct{ Brand, Range string }

// boxedRanges is every label with a real box behind it, by the same rule
// brandSets applies on a brand page: sellable, and big enough to clear
// setMinimum. A paint page needs the answer for *another* brand's range, so it
// is computed once over the whole catalogue rather than rescanned nine
// thousand times.
func boxedRanges(paints []catalog.Paint) map[brandRange]bool {
	byBrand := map[string][]catalog.Paint{}
	for _, p := range paints {
		byBrand[p.Brand] = append(byBrand[p.Brand], p)
	}
	boxed := map[brandRange]bool{}
	for brand, own := range byBrand {
		for r, n := range subRanges(own, sellable) {
			if n >= setMinimum {
				boxed[brandRange{brand, r}] = true
			}
		}
	}
	return boxed
}

// setFor is the set link a paint page offers, and it is deliberately about the
// closest match rather than the paint in the title: someone reading this page
// has the paint at the top already and is deciding whether to buy into the
// range that answers it. Offering the box of the range they came in with would
// be selling them their own shelf.
//
// A paint ships under several labels and only some of them are boxed, so the
// first label with a box wins. When none has one the page keeps the pot links
// and nothing else — a brand-wide fallback would put "Golden kit" on thousands
// of pages, and a search with nothing behind it spends the click for nothing.
func (g *Generator) setFor(p catalog.Paint, boxed map[brandRange]bool) *setLink {
	if g.cfg.AmazonTag == "" {
		return nil
	}
	for _, r := range labelsOf(p, sellable) {
		if boxed[brandRange{p.Brand, r}] {
			return &setLink{Name: setName(p.Brand, r), URL: g.buySet(p.Brand, r)}
		}
	}
	return nil
}

// disclosure returns the sentence the operating agreement of that marketplace
// requires verbatim. Clause 5 of the Brazilian contract dictates its wording and
// clause 6 makes a breach of clause 5 a material one, so the sentence follows the
// store the tag belongs to even though every other word on the site is English.
func disclosure(host string) string {
	if strings.HasSuffix(host, ".com.br") {
		return "Como participante do Programa de Associados da Amazon, sou remunerado pelas compras qualificadas efetuadas."
	}
	return "As an Amazon Associate this site earns from qualifying purchases."
}

// labels counts how often a paint name repeats inside its brand, so a page can
// be given exactly as much qualification as it needs to be distinct and no more.
type labels struct{ byName, byRange map[string]int }

func newLabels(paints []catalog.Paint) labels {
	l := labels{byName: map[string]int{}, byRange: map[string]int{}}
	for _, p := range paints {
		l.byName[p.Brand+" "+p.Name]++
		l.byRange[p.Brand+" "+p.Name+"|"+trimBrand(p)]++
	}
	return l
}

// trimBrand drops a brand name the range already repeats — Italeri's range is
// recorded as "Italeri Acrylic Paint" — which would otherwise print it twice.
// A paint sold under several labels is qualified by the first of them alone:
// the bracket exists to tell two pages apart, and "Vallejo Black (Game Air /
// Game Color / Hobby Paint)" spends the whole title doing it. The page header
// still lists every range.
func trimBrand(p catalog.Paint) string {
	r := strings.TrimSpace(strings.TrimPrefix(p.Range, p.Brand))
	if i := strings.Index(r, catalog.RangeSep); i > 0 {
		r = r[:i]
	}
	return r
}

// product names a paint the way a shop lists it. The qualifier goes in brackets
// after the name rather than in the middle: "Citadel Abaddon Black" is the
// phrase people search for, and splitting it would cost the exact match to save
// nothing. 2,437 paints share a brand and a name with another — Golden sells a
// Naphthol Red Light in five ranges, three of them different colours — and
// without this their pages carry the same title, which is how a search engine
// decides to keep one and drop the rest.
func (l labels) product(p catalog.Paint) string {
	base := p.Brand + " " + p.Name
	r := trimBrand(p)
	if l.byName[base] < 2 || r == "" {
		return base
	}
	// A handful of ranges carry the same name twice over. There the catalogue
	// number is the only thing left that tells the two pots apart, and it is
	// what a modeller searching for one would type anyway.
	if l.byRange[base+"|"+r] > 1 && p.Code != "" {
		return base + " (" + r + ", " + p.Code + ")"
	}
	return base + " (" + r + ")"
}

func chartPath(from, to string) string { return "/convert/" + from + "-to-" + to + "/" }
