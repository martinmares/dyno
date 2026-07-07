package markdown

import (
	"strings"
	"testing"
)

func TestRendererCreatesFileDownloadPlaceholder(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	result, err := renderer.Render([]byte("# Downloads\n\n```file-download\nprometheus/dashboard.json\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.HTML, `class="file-download-placeholder"`) ||
		!strings.Contains(result.HTML, `data-file-download="cHJvbWV0aGV1cy9kYXNoYm9hcmQuanNvbg"`) {
		t.Fatalf("expected download placeholder, got %s", result.HTML)
	}
}
