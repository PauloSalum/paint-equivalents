// Package match ranks every paint against every other one by CIEDE2000 and
// groups the result by brand, which is the question the site exists to answer:
// "I have this pot, what is the closest thing in that other range".
package match

import (
	"runtime"
	"sort"
	"sync"

	"tintaequivalente/internal/catalog"
	"tintaequivalente/internal/color"
)

// Match points into the catalog rather than copying it: every paint is ranked
// against every other one, so a copy here multiplies the catalog by itself.
type Match struct {
	Paint *catalog.Paint
	DE    float64
}

func (m Match) Quality() string { return color.Quality(m.DE) }
func (m Match) Grade() string   { return color.Grade(m.DE) }

// BrandMatches is one target brand and the closest pots it offers, nearest
// first.
type BrandMatches struct {
	Brand string
	Slug  string
	Best  []Match
}

// Table is everything one paint's page needs: the cross-brand answers, ordered
// by how good the best answer in each brand is, and the near neighbours inside
// the paint's own range.
type Table struct {
	Cross   []BrandMatches
	SameRow []Match
}

// Build computes a Table for every paint. perBrand caps how many alternatives
// each brand contributes and sameBrand caps the in-range neighbours.
func Build(ps []catalog.Paint, perBrand, sameBrand int) []Table {
	if perBrand < 1 {
		perBrand = 1
	}
	if sameBrand < 0 {
		sameBrand = 0
	}
	out := make([]Table, len(ps))
	labs := make([]color.Lab, len(ps))
	for i, p := range ps {
		labs[i] = p.Lab()
	}

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	jobs := make(chan int, workers*4)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = one(i, ps, labs, perBrand, sameBrand)
			}
		}()
	}
	for i := range ps {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func one(i int, ps []catalog.Paint, labs []color.Lab, perBrand, sameBrand int) Table {
	byBrand := map[string]*BrandMatches{}
	same := make([]Match, 0, sameBrand)
	src := labs[i]
	for j := range ps {
		if j == i {
			continue
		}
		m := Match{Paint: &ps[j], DE: color.Distance(src, labs[j])}
		if ps[j].Brand == ps[i].Brand {
			same = keepBest(same, m, sameBrand)
			continue
		}
		bm := byBrand[ps[j].Brand]
		if bm == nil {
			bm = &BrandMatches{Brand: ps[j].Brand, Slug: ps[j].BrandSlug}
			byBrand[ps[j].Brand] = bm
		}
		bm.Best = keepBest(bm.Best, m, perBrand)
	}

	cross := make([]BrandMatches, 0, len(byBrand))
	for _, bm := range byBrand {
		cross = append(cross, *bm)
	}
	// Brands whose closest pot is nearer come first: the reader wants the best
	// answer at the top, not an alphabet.
	sort.Slice(cross, func(a, b int) bool {
		if cross[a].Best[0].DE != cross[b].Best[0].DE {
			return cross[a].Best[0].DE < cross[b].Best[0].DE
		}
		return cross[a].Brand < cross[b].Brand
	})

	return Table{Cross: cross, SameRow: same}
}

// keepBest inserts m into an already-sorted list capped at n, so a paint is
// compared against the whole catalog without ever holding the whole catalog.
func keepBest(ms []Match, m Match, n int) []Match {
	if n == 0 {
		return ms
	}
	if len(ms) == n && !closer(m, ms[len(ms)-1]) {
		return ms
	}
	pos := sort.Search(len(ms), func(k int) bool { return closer(m, ms[k]) })
	if len(ms) < n {
		ms = append(ms, Match{})
	}
	copy(ms[pos+1:], ms[pos:])
	ms[pos] = m
	return ms
}

// closer breaks ties on name so a rebuild of the same data produces the same
// pages, which is what keeps a diff reviewable.
func closer(a, b Match) bool {
	if a.DE != b.DE {
		return a.DE < b.DE
	}
	return a.Paint.Name < b.Paint.Name
}

// Pair is one row of a brand-to-brand conversion chart.
type Pair struct {
	From *catalog.Paint
	To   *catalog.Paint
	DE   float64
}

// Chart builds the full from-brand to-brand table, which is the page that
// answers the broad query rather than the single-pot one.
func Chart(ps []catalog.Paint, tables []Table, fromBrand, toBrand string) []Pair {
	var out []Pair
	for i := range ps {
		if ps[i].Brand != fromBrand {
			continue
		}
		for _, bm := range tables[i].Cross {
			if bm.Brand != toBrand {
				continue
			}
			out = append(out, Pair{From: &ps[i], To: bm.Best[0].Paint, DE: bm.Best[0].DE})
			break
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].From.Name < out[b].From.Name })
	return out
}
