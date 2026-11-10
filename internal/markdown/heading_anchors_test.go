package markdown

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestUnicodeHeadingAliasesKeepLegacyAndExplicitAnchors(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.RenderString("[Bloky](#stavební-bloky-yaml) · [Scénáře](#scénáře-sestavené-ze-stavebních-bloků) · [Nasazení](#deployment-flow)\n\n## Stavební bloky YAML\n\n## Scénáře sestavené ze stavebních bloků\n\n<a id=\"deployment-flow\"></a>\n\n## Jak funguje nasazení\n\n## Stavební bloky YAML\n\n## **Příliš** `žluťoučký` & kůň\n")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(result.HTML))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"stavební-bloky-yaml", "scénáře-sestavené-ze-stavebních-bloků", "deployment-flow", "stavební-bloky-yaml-1", "příliš-žluťoučký--kůň", "stavebn-bloky-yaml"} {
		if doc.Find(`[id="`+id+`"]`).Length() != 1 {
			t.Errorf("missing or duplicate anchor %q: %s", id, result.HTML)
		}
	}
	for _, item := range result.TOC {
		if doc.Find(`[id="`+item.ID+`"]`).Length() != 1 {
			t.Errorf("TOC link no longer resolves: %+v", item)
		}
	}
}

func TestUnicodeHeadingAliasesRespectExistingIDsAndCode(t *testing.T) {
	fragment := `<a id="český-název"></a><h2 id="custom">Český název</h2><h2 id="other">Český název</h2><pre><code>&lt;h2 id="code"&gt;Čeština&lt;/h2&gt;</code></pre>`
	got := addUnicodeHeadingAnchors(fragment)
	if strings.Count(got, `id="český-název"`) != 1 || !strings.Contains(got, `id="český-název-1"`) || strings.Contains(got, `id="čeština"`) {
		t.Fatalf("incorrect aliases: %s", got)
	}
}

func TestUnicodeHeadingSlug(t *testing.T) {
	for label, want := range map[string]string{
		"  Stavební bloky YAML  ": "stavební-bloky-yaml",
		"Pra\u0301va a přístupy?": "práva-a-přístupy",
		"日本語 見出し":                 "日本語-見出し",
		"Table_name - 2.0":        "table_name---20",
	} {
		if got := unicodeHeadingSlug(label); got != want {
			t.Errorf("%q: got %q, want %q", label, got, want)
		}
	}
}
