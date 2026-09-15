package markdown

import (
	"strings"
	"testing"
)

func TestRendererUsesFrontmatterForInteractiveTables(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	result, err := renderer.RenderString(`---
dyno:
  tables:
    sortable: true
    filter: true
---

| Property | Value |
| --- | --- |
| Database | **tsmp** |
`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Frontmatter.Dyno.Tables.Sortable || !result.Frontmatter.Dyno.Tables.Filter {
		t.Fatalf("expected Dyno table options in front matter: %#v", result.Frontmatter.Dyno.Tables)
	}
	for _, expected := range []string{
		`class="dyno-table-widget"`,
		`data-table-sortable="true"`,
		`data-table-filter="true"`,
		`dyno-table-filter`,
		`<strong>tsmp</strong>`,
	} {
		if !strings.Contains(result.HTML, expected) {
			t.Fatalf("expected %q in rendered table, got: %s", expected, result.HTML)
		}
	}
}

func TestRendererKeepsRegularTablesStatic(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	result, err := renderer.RenderString("| Name |\n| --- |\n| Dyno |\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.HTML, "dyno-table-widget") {
		t.Fatalf("regular Markdown table should remain static: %s", result.HTML)
	}
}

func TestRendererAppliesTableOptionsInsideMkDocsBlock(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	result, err := renderer.RenderString(`---
dyno:
  tables:
    filter: true
---

!!! note "Details"
    | Name | Value |
    | --- | --- |
    | Dyno | docs |
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.HTML, `data-table-sortable="false"`) ||
		!strings.Contains(result.HTML, `data-table-filter="true"`) {
		t.Fatalf("expected table options inside MkDocs block, got: %s", result.HTML)
	}
}
