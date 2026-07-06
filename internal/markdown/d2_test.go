package markdown_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/markdown"
)

func TestD2Render(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src := "# Test\n\n```d2\nx -> y: hello\ny -> z: world\n```\n\nSome text after.\n"
	res, err := r.RenderString(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HTML) < 100 {
		t.Fatalf("too short: %s", res.HTML)
	}
	// Check SVG is present
	if !strings.Contains(res.HTML, "<svg") {
		t.Fatal("no SVG in output")
	}
	if !strings.Contains(res.HTML, "d2-diagram") {
		t.Fatal("no d2-diagram class")
	}
	// Check html.dark rewrite worked
	if strings.Contains(res.HTML, "prefers-color-scheme") {
		t.Fatal("media query not rewritten")
	}
}

func TestD2PageRenderUsesUniqueSVGIDs(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src, err := os.ReadFile("../../site/guides/d2-diagrams.md")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}

	idRe := regexp.MustCompile(`id="([^"]+)"`)
	counts := map[string]int{}
	for _, m := range idRe.FindAllStringSubmatch(res.HTML, -1) {
		counts[m[1]]++
	}

	for id, count := range counts {
		if count > 1 {
			t.Fatalf("duplicate id %q appears %d times", id, count)
		}
	}

	if strings.Contains(res.HTML, "<svg style=\"max-width:100%;height:auto;\"") &&
		strings.Contains(res.HTML, "<svg class=\"d2-") {
		nested := regexp.MustCompile(`(?s)<svg[^>]*>\s*<svg class="d2-`)
		if nested.MatchString(res.HTML) {
			t.Fatal("nested root svg found in D2 output")
		}
	}

	backgroundRect := regexp.MustCompile(`(?s)<svg[^>]*>\s*<rect[^>]*stroke-width="0"[^>]*/>`)
	if backgroundRect.MatchString(res.HTML) {
		t.Fatal("root background rect found in D2 output")
	}
}

func TestRendererAppliesUppercaseTemplateEnv(t *testing.T) {
	r, err := markdown.NewRendererWithEnv(map[string]string{
		"HTTP_SAMPLE_HOST":    "https://httpbin.org",
		"HTTP_METHOD_FOR_GET": "GET",
	})
	if err != nil {
		t.Fatal(err)
	}

	src := "```api\n{{HTTP_METHOD_FOR_GET}} {{ HTTP_SAMPLE_HOST }}/get\n```"
	res, err := r.RenderString(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "https://httpbin.org/get") {
		t.Fatalf("expected env-expanded URL in output, got: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, ">GET<") {
		t.Fatalf("expected env-expanded method in output, got: %s", res.HTML)
	}
}

func TestRendererMarksInsecureAPIWidget(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.RenderString("```api-insecure\nGET https://api.example.test/status\n```")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `"insecure":true`) {
		t.Fatalf("expected insecure API widget metadata, got: %s", res.HTML)
	}

	res, err = r.RenderString("```api\nGET https://api.example.test/status\n```")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `"insecure":false`) {
		t.Fatalf("expected secure API widget metadata, got: %s", res.HTML)
	}
}

func TestRendererCanHideAPIWidgetAuth(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.RenderString("```api-insecure no-auth\nGET https://api.example.test/status\n```")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, `api-auth-tabs`) || strings.Contains(res.HTML, `>Auth<`) {
		t.Fatalf("expected API widget without auth controls, got: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `"insecure":true`) {
		t.Fatalf("expected no-auth option to preserve insecure mode, got: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `data-api-action="copy-response"`) {
		t.Fatalf("expected response copy button, got: %s", res.HTML)
	}
}

func TestRendererConfiguresAPIFollowups(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.RenderString("```api-insecure no-auth\nGET https://api.example.test/discovery\n@follow-jsonpath: $[*].labels.__metrics_path__\n@follow-method: GET\n```")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"followJsonPath":"$[*].labels.__metrics_path__"`,
		`"followMethod":"GET"`,
		`data-role="followups"`,
	} {
		if !strings.Contains(res.HTML, expected) {
			t.Fatalf("expected %q in follow-up API widget, got: %s", expected, res.HTML)
		}
	}
	if strings.Contains(res.HTML, `api-field-label--fixed">@follow-`) {
		t.Fatalf("follow-up directive was rendered as an HTTP header: %s", res.HTML)
	}
}

func TestRendererParsesExternalRefsFrontmatter(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src := []byte(`---
title: Linked page
external_refs:
  - id: PROJ128
    label: Project PROJ128
    url: https://projects.example.test/PROJ128
    type: project
---
# Linked page
`)

	res, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frontmatter.ExternalRefs) != 1 {
		t.Fatalf("expected 1 external ref, got %d", len(res.Frontmatter.ExternalRefs))
	}
	ref := res.Frontmatter.ExternalRefs[0]
	if ref.ID != "PROJ128" || ref.Label != "Project PROJ128" || ref.URL != "https://projects.example.test/PROJ128" || ref.Type != "project" {
		t.Fatalf("unexpected external ref: %#v", ref)
	}
}

func TestRendererDoesNotConsumeLiteralAPIExampleInsideOuterFence(t *testing.T) {
	r, err := markdown.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	src := "````markdown\n```api\nGET https://api.example.com/endpoint\nHeader-Name: value\n```\n````\n"
	res, err := r.RenderString(src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "APIPLACEHOLDER") {
		t.Fatalf("literal api example was consumed: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, "```api") {
		t.Fatalf("expected literal api fence in rendered HTML, got: %s", res.HTML)
	}
}
