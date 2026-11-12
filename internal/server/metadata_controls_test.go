package server

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mares/dyno/internal/metadata"
)

func TestMetadataFilterControls(t *testing.T) {
	r := httptest.NewRequest("GET", "/_search?q=Postgres&meta.owner=alice&meta.owner=bob&meta.status=DRAFT&page=3&_search_anchor=old", nil)
	facets := []metadata.Facet{
		{Name: "owner", Label: "Owner", Values: []metadata.Value{{Value: "alice", Selected: true}, {Value: "bob", Selected: true}}},
		{Name: "status", Label: "Status", Values: []metadata.Value{{Value: "DRAFT", Selected: true}}},
	}
	controls := newMetadataFilterControls(r, facets, false)
	if controls.Action != "/_search" || controls.HTMX || controls.ClearURL != "/_search?q=Postgres" || len(controls.Chips) != 3 {
		t.Fatalf("unexpected controls: %+v", controls)
	}
	if len(controls.Hidden) != 1 || controls.Hidden[0] != (metadataFilterInput{"q", "Postgres"}) {
		t.Fatalf("search query must survive applying filters: %+v", controls.Hidden)
	}
	u, err := url.Parse(controls.Chips[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("meta.owner") != "bob" || len(q["meta.owner"]) != 1 || q.Get("meta.status") != "DRAFT" || q.Get("q") != "Postgres" || q.Has("page") || q.Has("_search_anchor") {
		t.Fatalf("removing one chip must preserve the other conditions: %v", q)
	}
	if r.URL.Query().Get("page") != "3" {
		t.Fatal("must not modify the original request")
	}
}

func TestMetadataFilterControlsWithoutFacets(t *testing.T) {
	if controls := newMetadataFilterControls(httptest.NewRequest("GET", "/", nil), nil, true); controls != nil {
		t.Fatalf("unexpected controls: %+v", controls)
	}
}
