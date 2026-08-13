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

//go:embed tmpl/*.html
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
	Title  string
	Desc   string
	Path   string
	Image  string
	JSONLD template.JS
	Site   Meta
	Finder bool
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
	// actually answers.
	coverage map[string]string

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
		coverage: map[string]string{},
	}
	for _, name := range []string{"home", "paint", "brand", "chart", "list", "about", "find", "privacy"} {
		t, err := template.ParseFS(tmplFS, "tmpl/layout.html", "tmpl/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		g.tmpl[name] = t
	}
	return g, nil
}

// Run writes every page and returns how many files it produced.
func (g *Generator) Run() (int, error) {
	if err := os.MkdirAll(g.cfg.Out, 0o755); err != nil {
		return 0, err
	}
	steps := []func() error{
		g.copyAssets, g.siteCard, g.home, g.paintPages, g.brandPages,
		g.chartPages, g.indexes, g.about, g.privacy, g.find, g.searchIndex, g.sitemap, g.wellKnown,
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
	"/paint/", "/brand/", "/brands/", "/charts/", "/convert/", "/about/", "/privacy/",
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

// popular seeds the home page with the pairs people actually search for,
// restricted to brands this build really has.
func (g *Generator) popular() []Link {
	want := [][2]string{
		{"Citadel", "Vallejo"}, {"Vallejo", "Citadel"},
		{"Citadel", "The Army Painter"}, {"The Army Painter", "Vallejo"},
		{"Citadel", "Scale75"}, {"Vallejo", "The Army Painter"},
		{"Citadel", "P3"}, {"Tamiya", "Vallejo"},
	}
	have := map[string]catalog.Brand{}
	for _, b := range g.brands {
		have[b.Name] = b
	}
	var out []Link
	for _, w := range want {
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
			Paint    catalog.Paint
			Table    match.Table
			Detailed []match.BrandMatches
			Rest     []match.BrandMatches
			Charts   map[string]bool
		}
		var buyBest, bestName string
		if len(t.Cross) > 0 {
			bestName = names.product(*t.Cross[0].Best[0].Paint)
			buyBest = g.buy(bestName)
		}
		desc := paintDesc(full, len(g.brands)-1, t.Cross)
		ld := g.crumbs(p.Brand, "/brand/"+p.BrandSlug+"/", p.Name)

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
				Site: g.meta, JSONLD: ld,
			},
			Paint: p, Table: t, Detailed: detailed, Rest: rest,
			Name: strings.TrimPrefix(full, p.Brand+" "),
			Buy:  g.buy(full), BuyBest: buyBest, BestName: bestName,
			Summary: summarise(p, t), Charts: charted[p.BrandSlug],
		})
		if err != nil {
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
	return template.JS(fmt.Sprintf(`{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[`+
		`{"@type":"ListItem","position":1,"name":%q,"item":%q},`+
		`{"@type":"ListItem","position":2,"name":%q}]}`,
		parent, g.cfg.BaseURL+parentURL, leaf))
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
		}
		err := g.render("brand", "brand/"+b.Slug+"/index.html", data{
			common: common{
				Title: fmt.Sprintf("%s paint conversions — every colour, matched across ranges", b.Name),
				Desc: fmt.Sprintf("All %d %s paints with their closest equivalents in other ranges, matched by CIEDE2000.",
					b.Count, b.Name),
				Path: "/brand/" + b.Slug + "/", Site: g.meta,
				JSONLD: g.crumbs("Paint ranges", "/brands/", b.Name),
			},
			Brand: b, Paints: own, Charts: charts,
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
	des := make([]float64, 0, len(rows))
	near, usable := 0, 0
	missed := map[string]int{}
	for _, r := range rows {
		des = append(des, r.DE)
		switch {
		case r.DE < 2:
			near++
			usable++
		case r.DE < 5:
			usable++
		default:
			missed[color.Hue(r.From.Lab())]++
		}
	}
	sort.Float64s(des)

	// "an equivalent in X" rather than "an X paint", because brands whose name
	// carries its own article ("The Army Painter") read as nonsense otherwise.
	s := fmt.Sprintf("%d of the %d %s colours below have an equivalent in %s within ΔE 5, the distance at which a substitution stops being obvious on a painted model. %d sit within ΔE 2 and are interchangeable outright. Across the whole range the typical distance is ΔE %.1f.",
		usable, len(rows), from.Name, to.Name, near, des[len(des)/2])

	if hue, n := commonest(missed); n > 2 {
		s += fmt.Sprintf(" The %d gaps are not spread evenly: %d of them are %ss, so that is the corner of the %s range %s does not answer.",
			len(rows)-usable, n, hue, from.Name, to.Name)
	}
	return s
}

// coverage is the one figure that separates a chart worth opening from one that
// is mostly gaps: the share of the source range that has a usable stand-in.
func coverage(rows []match.Pair) string {
	if len(rows) == 0 {
		return ""
	}
	usable := 0
	for _, r := range rows {
		if r.DE < 5 {
			usable++
		}
	}
	return fmt.Sprintf("%d of %d matched", usable, len(rows))
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
				From: from, To: to, Rows: rows,
				Summary: chartSummary(from, to, rows),
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
			note, ok := g.coverage[path]
			if !ok {
				continue
			}
			chartLinks = append(chartLinks, Link{URL: path, From: from.Name, To: to.Name, Note: note})
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
func trimBrand(p catalog.Paint) string {
	return strings.TrimSpace(strings.TrimPrefix(p.Range, p.Brand))
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
