package server

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/metadata"
)

type metadataFilterChip struct {
	Label, Value, URL string
}

type metadataFilterInput struct {
	Name, Value string
}

type metadataFilterControls struct {
	Action, ClearURL, BulkURL string
	HTMX                      bool
	Facets                    []metadata.Facet
	Chips                     []metadataFilterChip
	Hidden                    []metadataFilterInput
}

func newMetadataFilterControls(r *http.Request, facets []metadata.Facet, htmx bool) *metadataFilterControls {
	if len(facets) == 0 {
		return nil
	}
	controls := &metadataFilterControls{Action: r.URL.Path, HTMX: htmx, Facets: facets}
	values := r.URL.Query()
	values.Del("page")
	values.Del("_search_anchor")
	clear := url.Values{}
	for name, items := range values {
		if !strings.HasPrefix(name, metadata.QueryPrefix) {
			clear[name] = items
		}
	}
	link := func(query url.Values) string {
		if encoded := query.Encode(); encoded != "" {
			return controls.Action + "?" + encoded
		}
		return controls.Action
	}
	controls.ClearURL = link(clear)
	keys := make([]string, 0, len(clear))
	for name := range clear {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		for _, value := range clear[name] {
			controls.Hidden = append(controls.Hidden, metadataFilterInput{name, value})
		}
	}
	for _, facet := range facets {
		for _, value := range facet.Values {
			if !value.Selected {
				continue
			}
			query, _ := url.ParseQuery(values.Encode())
			name := metadata.QueryPrefix + facet.Name
			query.Del(name)
			for _, item := range values[name] {
				if item != value.Value {
					query.Add(name, item)
				}
			}
			controls.Chips = append(controls.Chips, metadataFilterChip{facet.Label, value.Value, link(query)})
		}
	}
	return controls
}
