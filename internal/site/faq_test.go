package site

import (
	"encoding/json"
	"strings"
	"testing"

	"tintaequivalente/internal/match"
)

func TestAskAnswersInTheWordsThePageIsSearchedWith(t *testing.T) {
	cross := []match.BrandMatches{
		brandMatch("Arteza", "Bronze", 1.3),
		brandMatch("Vallejo", "Raw Sienna", 1.6),
	}
	got := ask("Citadel Balthasar Gold", "Citadel", "Vallejo", cross)
	if len(got) != 2 {
		t.Fatalf("got %d questions, want 2: %+v", len(got), got)
	}

	// The wording is the point: these are the phrasings the queries and the
	// People-also-ask entries use, and a page that never says them cannot be
	// quoted for them.
	joined := got[0].Q + got[0].A + got[1].Q + got[1].A
	for _, word := range []string{"equivalent", "alternative", "substitute", "conversion chart"} {
		if !strings.Contains(joined, word) {
			t.Errorf("%q never appears in the answers:\n%s", word, joined)
		}
	}
	if !strings.Contains(got[0].A, "Arteza Bronze") || !strings.Contains(got[0].A, "1.3") {
		t.Errorf("the first answer must name the closest match and its distance:\n%s", got[0].A)
	}
	// The nearest paint in Lab is often a craft range nobody paints models
	// with, so the answer has to carry the runners up too.
	if !strings.Contains(got[0].A, "Vallejo Raw Sienna") {
		t.Errorf("the first answer should name the next nearest too:\n%s", got[0].A)
	}
	if strings.Contains(joined, " a indistinguishable") || strings.Contains(joined, " a an") {
		t.Errorf("article does not agree with the word after it:\n%s", joined)
	}
	if !strings.Contains(got[1].Q, "Vallejo") || !strings.Contains(got[1].A, "Raw Sienna") {
		t.Errorf("the second answer must cover the named range:\n%s\n%s", got[1].Q, got[1].A)
	}
}

// An answer that calls a visible mismatch a substitute is worse than no answer:
// it is the one thing on the page a reader can catch being wrong.
func TestAskDoesNotCallADistantPaintASubstitute(t *testing.T) {
	got := ask("Citadel Corvus Black", "Citadel", "Vallejo", []match.BrandMatches{
		brandMatch("Vallejo", "Black Grey", 7.4),
	})
	if strings.Contains(got[0].A, "substitute") {
		t.Errorf("ΔE 7.4 is not a substitute:\n%s", got[0].A)
	}
	if none := ask("Citadel Corvus Black", "Citadel", "Vallejo", nil); none != nil {
		t.Errorf("a paint with no match should ask nothing, got %+v", none)
	}
}

// The JSON-LD is assembled by hand, so a name carrying a quote or a backslash
// would break the block silently — the page still renders and the structured
// data is simply dropped.
func TestGraphStaysParseableWithAwkwardNames(t *testing.T) {
	qs := ask(`Vallejo "Panzer" Aces`, "Vallejo", "Citadel", []match.BrandMatches{
		brandMatch("Citadel", `Rhinox\Hide`, 2.2),
	})
	var out struct {
		Graph []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal([]byte(graph(faqList(qs))), &out); err != nil {
		t.Fatalf("structured data is not valid JSON: %v", err)
	}
	if len(out.Graph) != 1 || out.Graph[0]["@type"] != "FAQPage" {
		t.Fatalf("graph does not carry the FAQ: %+v", out.Graph)
	}
}

// "a indistinguishable substitute" is the kind of line a reader stops reading
// at, and it is on every page whose closest match is under ΔE 1.
func TestVerdictAgreesWithItsArticle(t *testing.T) {
	for de, want := range map[float64]string{0.4: "an indistinguishable", 1.5: "a near-perfect", 3: "a very close"} {
		if got := verdict(de); !strings.HasPrefix(got, want) {
			t.Errorf("verdict(%.1f) = %q, want it to start %q", de, got, want)
		}
	}
}
