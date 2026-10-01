package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mares/dyno/internal/browse"
	"github.com/mares/dyno/internal/navigation"
)

func TestBrowseCommandRejectsInvalidSelectionBeforeStarting(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(file, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--port", "-1"}, {"--port", "65536"}, {"--no-open", file}, {"--no-open", file + ".missing"}} {
		cmd := newBrowseCommand()
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestBrowseWatcherIgnoresSiblingsAndReloadsSelectedFile(t *testing.T) {
	dir := t.TempDir()
	doc, sibling := filepath.Join(dir, "selected.md"), filepath.Join(dir, "other.md")
	if err := os.WriteFile(doc, []byte("# Selected"), 0644); err != nil {
		t.Fatal(err)
	}
	catalog, err := browse.New([]string{doc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	_, signature, err := catalog.Build()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan int, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchBrowse(ctx, catalog, signature, 10*time.Millisecond, func(nav *navigation.NavNode) { changes <- len(navigation.FlatPages(nav)) })
	}()
	defer func() { cancel(); <-done }()
	if err := os.WriteFile(sibling, []byte("# Unselected"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
		t.Fatal("unselected sibling triggered refresh")
	case <-time.After(60 * time.Millisecond):
	}
	if err := os.WriteFile(doc, []byte("# Updated selected document"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case count := <-changes:
		if count != 1 {
			t.Fatalf("selection widened to %d documents", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selected change did not trigger refresh")
	}
}
