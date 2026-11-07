package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type pinSource struct {
	ID    string `json:"id"`
	Base  string `json:"base"`
	Label string `json:"label"`
}

// Only a digest of the filesystem identity is exposed to the browser. Version,
// branch and URL mount point are deliberately not part of the storage key.
func (s *Server) pinSource() pinSource {
	identity := []string{"site", canonicalPinRoot(s.contentDir)}
	if s.browse != nil {
		identity = append([]string{"browse"}, s.browse.SourcePaths()...)
	}
	if id := strings.TrimSpace(s.siteCfg.StorageID); id != "" {
		identity = []string{"explicit", id}
	}
	data, _ := json.Marshal(identity)
	return pinSource{ID: fmt.Sprintf("%x", sha256.Sum256(data)), Base: s.basePath, Label: s.siteCfg.Title}
}

func canonicalPinRoot(root string) string {
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return filepath.Clean(root)
}

func encodePinSources(sources []pinSource) string {
	data, _ := json.Marshal(sources)
	return string(data)
}

func (s *Server) pinSources() string { return encodePinSources([]pinSource{s.pinSource()}) }

func (ls *LibraryServer) pinSources() string {
	var sources []pinSource
	for _, book := range ls.books {
		if server := ls.servers[book.Slug]; server != nil {
			sources = append(sources, server.pinSource())
		}
	}
	return encodePinSources(sources)
}
