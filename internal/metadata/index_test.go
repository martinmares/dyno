package metadata

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/navigation"
)

func TestIndexFiltersAndBuildsContextualFacets(t *testing.T) {
	root := t.TempDir()
	writeMetadataFile(t, filepath.Join(root, "one.md"), "---\nowner: alice\nstatus: DRAFT\ntags: [api, go]\n---\n# One\n")
	writeMetadataFile(t, filepath.Join(root, "two.md"), "---\nowner: bob\nstatus: REVIEW\ntags: [go]\n---\n# Two\n")
	writeMetadataFile(t, filepath.Join(root, "three.md"), "---\nowner: alice\nstatus: REVIEW\n---\n# Three\n")
	nav, err := navigation.BuildTree(root, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.FrontmatterConfig{Fields: map[string]config.FrontmatterFieldConfig{
		"owner":  {Label: "Owner", Filterable: true},
		"status": {Label: "Status", Filterable: true, Options: []string{"DRAFT", "REVIEW"}},
		"tags":   {Label: "Tags", Filterable: false},
	}}
	idx, err := Build(nav, cfg)
	if err != nil {
		t.Fatal(err)
	}
	filter := idx.Parse(url.Values{"meta.owner": {"alice"}, "meta.status": {"REVIEW"}})
	allowed := idx.Allowed(filter)
	if len(allowed) != 1 || !allowed["/three"] {
		t.Fatalf("unexpected allowed paths: %#v", allowed)
	}
	facets := idx.Facets(filter)
	if len(facets) != 2 {
		t.Fatalf("expected two filterable facets, got %#v", facets)
	}
	for _, facet := range facets {
		if facet.Name == "status" {
			if len(facet.Values) != 2 || facet.Values[0].Count != 1 || facet.Values[1].Count != 1 {
				t.Fatalf("status counts must ignore the status selection: %#v", facet.Values)
			}
		}
	}
}

func writeMetadataFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
