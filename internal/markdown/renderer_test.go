package markdown

import (
	"strings"
	"testing"
)

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
