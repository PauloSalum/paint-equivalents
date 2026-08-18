package site

import (
	"bytes"
	"html/template"
	"testing"
)

// The Zenital link is the highest-value outbound click on the site and it lives
// in a template block rather than in any struct, so nothing else notices when a
// rewrite of one of these pages quietly drops the call. The block disappearing
// is the worse half: an undefined template is not a parse error, it surfaces as
// an execute error six minutes into a build.
func TestPaintChartAndFindCarryTheZenitalCTA(t *testing.T) {
	tp, err := template.ParseFS(tmplFS, "tmpl/layout.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tp.ExecuteTemplate(&buf, "zenital", nil); err != nil {
		t.Fatalf("zenital block: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`href="https://zenital.art/"`)) {
		t.Error("the zenital block rendered without a link to zenital.art")
	}

	for _, name := range []string{"tmpl/paint.html", "tmpl/chart.html", "tmpl/find.html"} {
		body, err := tmplFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(`{{template "zenital" .}}`)) {
			t.Errorf("%s no longer calls the zenital block", name)
		}
	}
}
