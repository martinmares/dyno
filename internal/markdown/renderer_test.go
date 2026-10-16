package markdown

import (
	"strings"
	"testing"
)

func TestSyntaxHighlightingUsesDistinctLightAndDarkThemes(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	light := renderer.LightCSS()
	dark := renderer.DarkCSS()
	if !strings.Contains(light, "html:not(.dark) .chroma .k") || !strings.Contains(dark, "html.dark .chroma .k") {
		t.Fatal("expected each syntax palette to be scoped to its own theme")
	}
	if strings.ReplaceAll(light, "html:not(.dark) .chroma", ".chroma") == strings.ReplaceAll(dark, "html.dark .chroma", ".chroma") {
		t.Fatal("light and dark modes must use different syntax palettes")
	}
}

func TestRendererTerminalCodeChromeIsOptIn(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	result, err := renderer.RenderString("```text\nplain code\n```\n\n```console\n$ dyno --site .\n```")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(result.HTML, `dyno-terminal-code`) != 1 {
		t.Fatalf("expected terminal class only on the console block, got:\n%s", result.HTML)
	}
	if !strings.Contains(result.HTML, `class="dyno-terminal-code"><pre`) {
		t.Fatalf("expected terminal class to wrap the highlighted block, got:\n%s", result.HTML)
	}
}
