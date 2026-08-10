// Command build renders the whole static site into a directory ready to be
// served by any static host.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/match"
	"tintaequivalente/internal/site"
)

func main() {
	var (
		in    = flag.String("paints", "paints.json", "path to the paint catalog JSON")
		out   = flag.String("out", "dist", "output directory")
		base  = flag.String("base", "https://example.com", "canonical base URL, no trailing slash")
		ads   = flag.String("adsense", "", "AdSense client id, e.g. ca-pub-0000000000000000")
		amz   = flag.String("amazon-tag", "", "Amazon Associates tracking id; adds affiliate shop links and the required disclosure")
		dom   = flag.String("domain", "", "custom domain to write into CNAME")
		perBr = flag.Int("per-brand", 3, "matches listed per other brand on a paint page")
		same  = flag.Int("same-brand", 6, "nearest colours listed inside the paint's own brand")
		cmin  = flag.Int("chart-min", 60, "minimum paints a brand needs before it gets chart pages")
		det   = flag.Int("detail-brands", 12, "brands given a full block on a paint page before the rest are summarised")
		ikey  = flag.String("indexnow", "", "IndexNow key; written as <key>.txt for search-engine verification")
		exp   = flag.String("export", "", "write the publishable subset of the catalog here and exit")
	)
	flag.Parse()

	start := time.Now()

	paints, err := catalog.Load(*in)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if len(paints) == 0 {
		log.Fatal("catalog is empty after the licence filter; nothing publishable")
	}
	brands := catalog.Brands(paints)
	fmt.Printf("catalog   %d publishable paints, %d brands\n", len(paints), len(brands))

	// The repository must never carry the ranges whose source withheld
	// redistribution, so the committed catalog is the filtered one.
	if *exp != "" {
		if err := catalog.Export(*exp, paints); err != nil {
			log.Fatalf("export: %v", err)
		}
		fmt.Printf("exported  %s\n", *exp)
		return
	}

	tables := match.Build(paints, *perBr, *same)
	fmt.Printf("matched   %d tables in %s\n", len(tables), time.Since(start).Round(time.Millisecond))

	if err := os.RemoveAll(*out); err != nil {
		log.Fatalf("clean %s: %v", *out, err)
	}
	g, err := site.New(site.Config{
		BaseURL:       *base,
		Out:           *out,
		AdsenseClient: *ads,
		AmazonTag:     *amz,
		Domain:        *dom,
		IndexNowKey:   *ikey,
		PerBrand:      *perBr,
		SameBrand:     *same,
		ChartMinimum:  *cmin,
		DetailBrands:  *det,
	}, paints, tables)
	if err != nil {
		log.Fatalf("init generator: %v", err)
	}
	n, err := g.Run()
	if err != nil {
		log.Fatalf("render: %v", err)
	}
	fmt.Printf("rendered  %d files into %s in %s\n", n, *out, time.Since(start).Round(time.Millisecond))
}
