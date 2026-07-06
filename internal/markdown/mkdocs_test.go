package markdown_test

import (
	"strings"
	"testing"

	"github.com/mares/dyno/internal/markdown"
)

func TestRendererSupportsNestedMkDocsAdmonitions(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src := `# Chyby a logování ^4.1.8^

???+ failure "Aplikace musí logovat"
    * aktivity administrátorů aplikace
    * bezpečnostní události

    **Povinné události:**

    !!! info "**Bude součástí produktu** :star:"
        Včetně vnořeného obsahu.

Text za blokem.
`
	res, err := r.RenderString(src)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		`<sup>4.1.8</sup>`,
		`class="callout callout-danger mkdocs-admonition mkdocs-admonition-failure mkdocs-details" open`,
		`Aplikace musí logovat`,
		`<li>aktivity administrátorů aplikace</li>`,
		`class="callout callout-info mkdocs-admonition mkdocs-admonition-info"`,
		`<strong>Bude součástí produktu</strong> ⭐`,
		`Včetně vnořeného obsahu.`,
		`Text za blokem.`,
	} {
		if !strings.Contains(res.HTML, expected) {
			t.Fatalf("expected %q in rendered HTML, got: %s", expected, res.HTML)
		}
	}
	if strings.Contains(res.HTML, `sup418sup`) {
		t.Fatalf("superscript markup must not leak into the heading ID: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `id="chyby-a-logovn-418"`) {
		t.Fatalf("expected clean heading ID, got: %s", res.HTML)
	}
}

func TestRendererSupportsClosedMkDocsDetailsAndDefaultTitle(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.RenderString("??? note\n    Hidden by default.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `mkdocs-admonition-note mkdocs-details"`) {
		t.Fatalf("expected details block, got: %s", res.HTML)
	}
	if strings.Contains(res.HTML, `mkdocs-details" open`) {
		t.Fatalf("expected details block to be closed by default, got: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `<span>Note</span>`) {
		t.Fatalf("expected default title, got: %s", res.HTML)
	}
}

func TestRendererOmitsEmptyMkDocsAdmonitionBody(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.RenderString(`!!! danger "Custom development"`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, `mkdocs-admonition-body`) {
		t.Fatalf("empty admonition must render as a title bar only, got: %s", res.HTML)
	}
}

func TestRendererLeavesMkDocsInlineSyntaxInsideCode(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src := "Outside ^4.1.8^ :warning:.\n\nInline `^4.1.8^ :star:`.\n\n```text\n^4.1.8^ :star:\n```\n"
	res, err := r.RenderString(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `Outside <sup>4.1.8</sup> ⚠️.`) {
		t.Fatalf("expected inline syntax outside code to render, got: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `<code>^4.1.8^ :star:</code>`) {
		t.Fatalf("expected inline code to remain unchanged, got: %s", res.HTML)
	}
	if strings.Count(res.HTML, `<sup>4.1.8</sup>`) != 1 {
		t.Fatalf("expected fenced code to remain unchanged, got: %s", res.HTML)
	}
}
