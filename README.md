# Paint Equivalents

A static site that answers one question: *I own this pot of paint, the recipe
calls for a different range — what is the closest thing I can actually buy?*

9,327 miniature paints across 29 ranges, every one of them compared against
every other by **CIEDE2000**, published as ~10,000 pre-rendered pages. No
server, no database, no request-time work.

## Why CIEDE2000 and not RGB distance

Comparing RGB numbers does not match human vision: two greens separated by the
same RGB distance as two blues do not look equally different. CIEDE2000
(CIE 142:2001) corrects for that with lightness, chroma and hue weighting plus
a rotation term in the blue region.

The implementation is verified against the 34-pair acceptance set from Sharma,
Wu & Dalal (2005) — `internal/color/color_test.go`. That set exists precisely
to catch implementations that get the hue-band and rotation terms wrong, which
most naive ports do.

## Build

```sh
go build ./... && go vet ./... && go test ./...
go run ./cmd/build -paints paints.json -out dist -base https://paints.paulosalum.com
```

Flags worth knowing:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-per-brand` | 3 | alternatives listed per range on a paint page |
| `-detail-brands` | 12 | ranges given a full block before the rest are summarised |
| `-chart-min` | 60 | paints a range needs before it gets conversion charts |
| `-adsense` | — | AdSense client id; injects the script and writes `ads.txt` |
| `-amazon-tag` | — | Associates tracking id; adds shop links and the required disclosure |
| `-indexnow` | — | IndexNow key, served as `<key>.txt` |
| `-domain` | — | custom domain, written to `CNAME` |
| `-export` | — | write the publishable subset of the catalog and exit |

## Layout

```
internal/color     CIEDE2000, sRGB→Lab, WCAG contrast for swatch text
internal/catalog   loading and the licence filter
internal/match     ranks every paint against every other, top-N per range
internal/site      templates, assets, all page rendering
cmd/build          generator entry point
```

`internal/match` compares 9,327 × 9,327 pairs. It keeps only a bounded top-N
per range as it goes and stores pointers rather than paint copies — the naive
version allocates 87 million structs and exhausts a 32-bit heap.

## Monetisation

Both channels are off until an id is passed, and neither changes what the site
says: the match order is the colour distance and nothing else. Set
`-amazon-tag` and the paint pages grow two shop links plus the disclosure the
Associates agreement requires; set `-adsense` and the layout loads the ad
script and writes `ads.txt`.

## Data licence

The paint data is MIT, from
[Arcturus5404/miniature-paints](https://github.com/Arcturus5404/miniature-paints).
Ranges whose upstream source withheld redistribution are excluded by
`catalog.Publishable`, and a test fails the build if that filter is loosened.
See [LICENSE-DATA.md](LICENSE-DATA.md).

## Deploy

`.github/workflows/deploy.yml` builds the site and publishes it to GitHub
Pages on every push to `main`. The custom domain needs one DNS record:

```
CNAME   paints   paulosalum.github.io
```
