# Paint Equivalents

**Live: [paints.paulosalum.com](https://paints.paulosalum.com)**

A static site that answers one question: *I own this pot of paint, the recipe
calls for a different range — what is the closest thing I can actually buy?*

Start points: [conversion charts](https://paints.paulosalum.com/charts/) ·
[find a paint from a hex code](https://paints.paulosalum.com/find/) ·
[all 31 ranges](https://paints.paulosalum.com/brands/)

9,693 miniature paints across 31 ranges, every one of them compared against
every other by **CIEDE2000**, published as ~10,000 pre-rendered pages. No
server, no database, no request-time work.

## Ranges covered

Every range below is matched against all 30 others, both directions.

| Range | Paints | Range | Paints |
| --- | --- | --- | --- |
| [AK Interactive](https://paints.paulosalum.com/brand/ak-interactive/) | 1,130 | [Mig](https://paints.paulosalum.com/brand/mig/) | 252 |
| [Acrilex](https://paints.paulosalum.com/brand/acrilex/) | 95 | [Mission Models](https://paints.paulosalum.com/brand/mission-models/) | 201 |
| [Apple Barrel](https://paints.paulosalum.com/brand/apple-barrel/) | 213 | [Monument](https://paints.paulosalum.com/brand/monument/) | 131 |
| [Arteza](https://paints.paulosalum.com/brand/arteza/) | 88 | [Mr. Hobby](https://paints.paulosalum.com/brand/mr-hobby/) | 668 |
| [Citadel](https://paints.paulosalum.com/brand/citadel/) | 451 | [Mr. Paint](https://paints.paulosalum.com/brand/mr-paint/) | 568 |
| [Coat d'Arms](https://paints.paulosalum.com/brand/coat-d-arms/) | 150 | [P3](https://paints.paulosalum.com/brand/p3/) | 131 |
| [Creature](https://paints.paulosalum.com/brand/creature/) | 53 | [Reaper](https://paints.paulosalum.com/brand/reaper/) | 438 |
| [Duncan](https://paints.paulosalum.com/brand/duncan/) | 180 | [Revell](https://paints.paulosalum.com/brand/revell/) | 88 |
| [FolkArt](https://paints.paulosalum.com/brand/folkart/) | 435 | [Scale75](https://paints.paulosalum.com/brand/scale75/) | 358 |
| [Foundry](https://paints.paulosalum.com/brand/foundry/) | 360 | [Tamiya](https://paints.paulosalum.com/brand/tamiya/) | 320 |
| [Golden](https://paints.paulosalum.com/brand/golden/) | 346 | [The Army Painter](https://paints.paulosalum.com/brand/the-army-painter/) | 703 |
| [Green Stuff World](https://paints.paulosalum.com/brand/green-stuff-world/) | 220 | [Tom Color](https://paints.paulosalum.com/brand/tom-color/) | 271 |
| [Humbrol](https://paints.paulosalum.com/brand/humbrol/) | 115 | [Turbo Dork](https://paints.paulosalum.com/brand/turbo-dork/) | 40 |
| [Italeri](https://paints.paulosalum.com/brand/italeri/) | 100 | [Vallejo](https://paints.paulosalum.com/brand/vallejo/) | 1,267 |
| [Kimera Kolors](https://paints.paulosalum.com/brand/kimera-kolors/) | 38 | [Warcolours](https://paints.paulosalum.com/brand/warcolours/) | 178 |
| [Liquitex](https://paints.paulosalum.com/brand/liquitex/) | 105 |  |  |

The pairs people ask for most:
[Citadel → Vallejo](https://paints.paulosalum.com/convert/citadel-to-vallejo/) ·
[Vallejo → Citadel](https://paints.paulosalum.com/convert/vallejo-to-citadel/) ·
[Citadel → The Army Painter](https://paints.paulosalum.com/convert/citadel-to-the-army-painter/) ·
[Citadel → P3](https://paints.paulosalum.com/convert/citadel-to-p3/) ·
[Tamiya → Vallejo](https://paints.paulosalum.com/convert/tamiya-to-vallejo/)

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
| `-adsense-slot` | — | ad unit id for the one in-content slot; needs `-adsense` |
| `-amazon-tag` | — | Associates tracking id; adds shop links and the required disclosure |
| `-amazon-host` | `www.amazon.com.br` | store the shop links point at; must match the marketplace that issued the tag |
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

`internal/match` compares 9,693 × 9,693 pairs. It keeps only a bounded top-N
per range as it goes and stores pointers rather than paint copies — the naive
version allocates 87 million structs and exhausts a 32-bit heap.

## Monetisation

Both channels are off until an id is passed, and neither changes what the site
says: the match order is the colour distance and nothing else. Set
`-amazon-tag` and the paint pages grow two shop links plus the disclosure the
Associates agreement requires; set `-adsense` and the layout loads the ad
script and writes `ads.txt`.

A tag is issued by one marketplace and earns nothing on any other, so
`-amazon-host` follows the tag, and two things follow the host: the search
qualifier (`tinta` on the Brazilian store, where `paint` matches almost
nothing) and the disclosure sentence, because each operating agreement
prescribes its own wording and the Brazilian one treats a departure from it as
a material breach.

`-adsense-slot` adds one explicit in-content placement — above the table on a
conversion chart, below the summary on a paint page. Both sit after the answer
the visitor came for and before the long tail of the page, which is the only
position that is worth anything without pushing the content down. Everything
else is left to Auto ads.

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
