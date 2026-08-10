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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tintaequivalente/internal/catalog"
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
	Domain        string
	PerBrand      int
	SameBrand     int
	ChartMinimum  int
	DetailBrands  int
}

// Meta is what every template can reach through .Site.
type Meta struct {
	BaseURL       string
	AdsenseClient string
	PaintCount    int
	BrandCount    int
}

type common struct {
	Title  string
	Desc   string
	Path   string
	JSONLD template.JS
	Site   Meta
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
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	brands := catalog.Brands(paints)
	g := &Generator{
		cfg: cfg, paints: paints, tables: tables, brands: brands,
		meta: Meta{
			BaseURL:       cfg.BaseURL,
			AdsenseClient: cfg.AdsenseClient,
			PaintCount:    len(paints),
			BrandCount:    len(brands),
		},
		tmpl: map[string]*template.Template{},
	}
	for _, name := range []string{"home", "paint", "brand", "chart", "list", "about"} {
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
		g.copyAssets, g.home, g.paintPages, g.brandPages,
		g.chartPages, g.indexes, g.about, g.searchIndex, g.sitemap, g.wellKnown,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return g.written, err
		}
	}
	return g.written, nil
}

func (g *Generator) write(rel string, body []byte) error {
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

func (g *Generator) paintPages() error {
	for i, p := range g.paints {
		t := g.tables[i]
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
			Paint    catalog.Paint
			Table    match.Table
			Detailed []match.BrandMatches
			Rest     []match.BrandMatches
		}
		best := ""
		if len(t.Cross) > 0 {
			best = fmt.Sprintf(" Closest match: %s %s (ΔE %.1f).",
				t.Cross[0].Brand, t.Cross[0].Best[0].Paint.Name, t.Cross[0].Best[0].DE)
		}
		ld := fmt.Sprintf(`{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[`+
			`{"@type":"ListItem","position":1,"name":%q,"item":%q},`+
			`{"@type":"ListItem","position":2,"name":%q}]}`,
			p.Brand, g.cfg.BaseURL+"/brand/"+p.BrandSlug+"/", p.Name)

		err := g.render("paint", strings.TrimPrefix(p.URL(), "/")+"index.html", data{
			common: common{
				Title: fmt.Sprintf("%s %s equivalents — closest paint in every range", p.Brand, p.Name),
				Desc: fmt.Sprintf("Equivalents for %s %s (%s) across %d paint ranges, matched by CIEDE2000.%s",
					p.Brand, p.Name, p.Hex, len(g.brands)-1, best),
				Path: p.URL(), Site: g.meta, JSONLD: template.JS(ld),
			},
			Paint: p, Table: t, Detailed: detailed, Rest: rest,
		})
		if err != nil {
			return err
		}
	}
	return nil
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
			},
			Brand: b, Paints: own, Charts: charts,
		})
		if err != nil {
			return err
		}
	}
	return nil
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
				From catalog.Brand
				To   catalog.Brand
				Rows []match.Pair
			}
			path := chartPath(from.Slug, to.Slug)
			err := g.render("chart", strings.TrimPrefix(path, "/")+"index.html", data{
				common: common{
					Title: fmt.Sprintf("%s to %s paint conversion chart (%d paints)", from.Name, to.Name, len(rows)),
					Desc: fmt.Sprintf("Complete %s to %s conversion chart: %d paints with their closest %s equivalent and the colour distance for each.",
						from.Name, to.Name, len(rows), to.Name),
					Path: path, Site: g.meta,
				},
				From: from, To: to, Rows: rows,
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
			chartLinks = append(chartLinks, Link{URL: chartPath(from.Slug, to.Slug), From: from.Name, To: to.Name})
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
		Heading: "Conversion charts", Lede: "Full tables, one row per paint, with the colour distance for every match.",
		Links: chartLinks,
	})
}

func (g *Generator) about() error {
	type data struct{ common }
	return g.render("about", "about/index.html", data{common{
		Title: "About the data and the colour matching",
		Desc:  "How the paint matching works, what it cannot tell you, and the licence the paint data is published under.",
		Path:  "/about/", Site: g.meta,
	}})
}

// --- machine-readable output ---

type searchRow struct {
	N string `json:"n"`
	B string `json:"b"`
	H string `json:"h"`
	I string `json:"i"`
	U string `json:"u"`
}

func (g *Generator) searchIndex() error {
	rows := make([]searchRow, 0, len(g.paints))
	for _, p := range g.paints {
		rows = append(rows, searchRow{N: p.Name, B: p.Brand, H: p.Hex, I: p.Ink(), U: p.URL()})
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
	add("/brands/", "0.8")
	add("/charts/", "0.8")
	add("/about/", "0.3")
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
	if g.cfg.AdsenseClient != "" {
		pub := strings.TrimPrefix(g.cfg.AdsenseClient, "ca-")
		line := fmt.Sprintf("google.com, %s, DIRECT, f08c47fec0942fa0\n", pub)
		if err := g.write("ads.txt", []byte(line)); err != nil {
			return err
		}
	}
	return nil
}

func chartPath(from, to string) string { return "/convert/" + from + "-to-" + to + "/" }
