package watcher

import (
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// Reloader is implemented by anything that can accept a new nav+index.
type Reloader interface {
	Reload(nav *navigation.NavNode, idx *search.Index)
}

// Watch watches siteRoot/site/ for changes and calls srv.Reload() on any event.
// plainText converts Markdown source to plain text for search indexing.
func Watch(siteRoot, basePath string, srv Reloader, plainText func(string) (string, error)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := w.Add(siteRoot + "/site"); err != nil {
		return err
	}

	go func() {
		defer w.Close()
		var timer *time.Timer

		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				// Watch newly created subdirectories
				if event.Has(fsnotify.Create) {
					_ = w.Add(event.Name)
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(300*time.Millisecond, func() {
					rebuild(siteRoot, basePath, srv, plainText)
				})

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	return nil
}

func rebuild(siteRoot, basePath string, srv Reloader, plainText func(string) (string, error)) {
	nav, err := navigation.BuildTree(siteRoot, basePath)
	if err != nil {
		log.Printf("watch: rebuild nav failed: %v", err)
		return
	}
	idx, err := search.BuildIndex(nav, plainText)
	if err != nil {
		log.Printf("watch: rebuild index failed: %v", err)
		return
	}
	srv.Reload(nav, idx)
	log.Printf("watch: reloaded (%d pages)", countNodes(nav))
}

func countNodes(node *navigation.NavNode) int {
	n := 0
	if node.FSPath != "" {
		n = 1
	}
	for _, c := range node.Children {
		n += countNodes(c)
	}
	return n
}
