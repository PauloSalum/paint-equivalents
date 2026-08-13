# Paint data licence

`paints.json` contains paint names, manufacturer codes, ranges and sampled
colour values from
[Arcturus5404/miniature-paints](https://github.com/Arcturus5404/miniature-paints),
released under the MIT licence, Copyright (c) 2022 Rick Fleuren.

```
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## What is deliberately absent

Rows from sources that granted no redistribution right are excluded, and the
exclusion is enforced in code rather than by hand: `catalog.Publishable`
requires the MIT source tag *and* rejects an explicit brand list, because the
source column and the upstream terms disagree for some ranges and the stricter
of the two has to win. `internal/catalog/catalog_test.go` fails the build if
that filter is ever loosened.

Six ranges are named in that list — Corfix, Talento, Daiara, Silverbright,
True Colors and Smooth3D. All six are tagged `mit-dataset` in the export and
none of them appears in any of the 34 brand files upstream actually publishes,
so the tag is wrong and they stay out until a licensed source exists.

**Acrilex and Tom Color are Brazilian too and are not in that list**, because
checking the same 34 files showed both of them there: `paints/Acrilex.md` and
`paints/TomColor.md`. Their rows are imported from those files rather than
copied out of the local export — for Acrilex the export carries the upstream
name beside a colour from elsewhere, disagreeing on the hex in 72 of 94 rows,
and the colour is the whole of what gets published. Lab is recomputed from the
upstream RGB with the same conversion the rest of the catalogue uses.

Also absent: harvested retailer barcodes and prices, and the RAL and Pantone
colour standards, which are reference systems rather than paints.

Brand and paint names are trademarks of their respective owners. This project
is not affiliated with, endorsed by or sponsored by any paint manufacturer.
