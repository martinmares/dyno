package watcher

import (
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
)

// Reloader is implemented by anything that can accept a new nav+index.
type Reloader interface {
	Reload(nav *navigation.NavNode, idx *search.Index)
}

// Watch watches contentDir for changes and calls srv.Reload() on any event.
// plainText converts Markdown source to plain text for search indexing.
func Watch(contentDir, basePath string, filter navigation.Filter, srv Reloader, plainText func(string) (string, error)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := w.Add(contentDir); err != nil {
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
					rebuild(contentDir, basePath, filter, srv, plainText)
				})

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Error("watcher error", "err", err)
			}
		}
	}()

	return nil
}

func rebuild(contentDir, basePath string, filter navigation.Filter, srv Reloader, plainText func(string) (string, error)) {
	nav, err := navigation.BuildTreeWithFilter(contentDir, basePath, filter)
	if err != nil {
		slog.Error("watch rebuild nav failed", "err", err)
		return
	}
	idx, err := search.BuildIndex(nav, plainText)
	if err != nil {
		slog.Error("watch rebuild index failed", "err", err)
		return
	}
	srv.Reload(nav, idx)
	slog.Info("watch reload complete", "pages", countNodes(nav))
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
