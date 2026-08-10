package site

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var hrefRE = regexp.MustCompile(`(?:href|src)="(/[^"#?]*)`)

// verify fails the build when a page links to a file that was never written.
// The generator decides in several places whether a page exists — a range under
// the chart minimum gets no chart, an excluded brand gets no pages — and a
// template that does not consult the same rule ships a dead link on every page
// it renders. That is invisible in a spot check of a site this size and costs
// crawl budget on every one of them, so the cheapest place to catch it is here,
// before the artifact is uploaded.
func (g *Generator) verify() error {
	// Which page a target was first seen on: enough to find the template at
	// fault without collecting every offender.
	seen := map[string]string{}
	err := filepath.Walk(g.cfg.Out, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range hrefRE.FindAllSubmatch(body, -1) {
			if target := string(m[1]); seen[target] == "" {
				seen[target] = p
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	var broken []string
	for target, from := range seen {
		rel := strings.TrimPrefix(strings.TrimPrefix(target, g.cfg.Prefix), "/")
		full := filepath.Join(g.cfg.Out, filepath.FromSlash(rel))
		if rel == "" || strings.HasSuffix(target, "/") {
			full = filepath.Join(full, "index.html")
		}
		if _, err := os.Stat(full); err != nil {
			broken = append(broken, fmt.Sprintf("%s (linked from %s)", target, from))
		}
	}
	if len(broken) == 0 {
		return nil
	}
	sort.Strings(broken)
	if len(broken) > 10 {
		broken = append(broken[:10], fmt.Sprintf("... and %d more", len(broken)-10))
	}
	return fmt.Errorf("broken internal links:\n  %s", strings.Join(broken, "\n  "))
}
