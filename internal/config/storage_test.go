package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageIDLoadsAndMergesAsFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dyno.yaml"), []byte("title: Docs\nstorage_id: project-docs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MergeDefaults(SiteConfig{StorageID: "library-fallback"})
	if cfg.StorageID != "project-docs" {
		t.Fatalf("dyno.yaml must win: %q", cfg.StorageID)
	}
	empty := &SiteConfig{}
	empty.MergeDefaults(SiteConfig{StorageID: "library-fallback"})
	if empty.StorageID != "library-fallback" {
		t.Fatal("library storage identity was not inherited")
	}
}
