package mcpengine_test

import (
	"path/filepath"
	"testing"

	"github.com/mares/dyno/internal/mcpengine"
)

func TestLoadSingleSiteAndGetPage(t *testing.T) {
	engine, err := mcpengine.Load([]string{filepath.Join("..", "..", "site")}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if engine.Mode() != mcpengine.ModeSingle {
		t.Fatalf("unexpected mode: %s", engine.Mode())
	}

	page, err := engine.GetPage("", "/guides/d2-diagrams")
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != "https://docs.example.test/guides/d2-diagrams" {
		t.Fatalf("unexpected URL: %s", page.URL)
	}
	if page.BookSlug != mcpengine.DefaultBookSlug {
		t.Fatalf("unexpected book slug: %s", page.BookSlug)
	}
	if page.Title == "" || page.Markdown == "" || page.PlainText == "" {
		t.Fatal("page payload is unexpectedly empty")
	}
}

func TestLoadLibraryMode(t *testing.T) {
	engine, err := mcpengine.Load([]string{
		filepath.Join("..", "..", "site"),
		filepath.Join("..", "..", "site-demo-go", "site"),
	}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if engine.Mode() != mcpengine.ModeLibrary {
		t.Fatalf("unexpected mode: %s", engine.Mode())
	}

	books := engine.ListBooks()
	if len(books) != 2 {
		t.Fatalf("unexpected book count: %d", len(books))
	}

	var demoSlug string
	for _, book := range books {
		if book.Slug != "site" {
			demoSlug = book.Slug
		}
	}
	if demoSlug == "" {
		t.Fatal("expected second library book slug")
	}

	results, err := engine.Search("Go", demoSlug, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one library search result")
	}
	if results[0].BookSlug != demoSlug {
		t.Fatalf("unexpected search result book slug: %s", results[0].BookSlug)
	}
}

func TestGetPageSectionAndResources(t *testing.T) {
	engine, err := mcpengine.Load([]string{filepath.Join("..", "..", "site")}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}

	section, err := engine.GetPageSection("", "/guides/d2-diagrams", "Architecture example")
	if err != nil {
		t.Fatal(err)
	}
	if section.Heading != "Architecture example" {
		t.Fatalf("unexpected heading: %s", section.Heading)
	}
	if section.PageURL != "https://docs.example.test/guides/d2-diagrams" {
		t.Fatalf("unexpected page URL: %s", section.PageURL)
	}
	if section.Markdown == "" || section.PlainText == "" {
		t.Fatal("section payload is unexpectedly empty")
	}

	resources, nextCursor, err := engine.ListResources("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) == 0 {
		t.Fatal("expected at least one resource")
	}
	if nextCursor == "" {
		t.Fatal("expected pagination cursor for resource list")
	}

	contents, err := engine.ReadResource("dyno://book/_default/page?path=%2Fguides%2Fd2-diagrams")
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 1 || contents[0].Text == "" {
		t.Fatal("expected page resource content")
	}
}
