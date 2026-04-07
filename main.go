package main

import (
	"embed"
	"flag"
	"fmt"
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/server"
	"github.com/mares/dyno/internal/watcher"
)

//go:embed templates assets
var embeddedFS embed.FS

const version = "0.1.0"

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	dir := flag.String("dir", ".", "Directory containing site/ folder")
	dev := flag.Bool("dev", false, "Dev mode: reload templates and assets from disk on every request")
	watch := flag.Bool("watch", false, "Watch site/ for changes and reload navigation/search automatically")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.StringVar(port, "p", "3000", "Port (shorthand)")
	flag.StringVar(dir, "d", ".", "Directory (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("dyno v%s\n", version)
		return
	}

	siteRoot, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("invalid directory: %v", err)
	}

	siteDir := filepath.Join(siteRoot, "site")
	if _, err := os.Stat(siteDir); os.IsNotExist(err) {
		log.Fatalf("site/ directory not found in %s\n\nCreate a site/ directory with Markdown files to get started.", siteRoot)
	}

	siteCfg, err := config.Load(siteRoot)
	if err != nil {
		log.Fatalf("failed to load dyno.yaml: %v", err)
	}

	log.Printf("dyno v%s — %s — loading docs from %s", version, siteCfg.Title, siteDir)
	if *dev {
		log.Printf("dev mode: templates and assets loaded from disk (no rebuild needed)")
	}

	nav, err := navigation.BuildTree(siteRoot, siteCfg.GetBasePath())
	if err != nil {
		log.Fatalf("failed to build navigation tree: %v", err)
	}
	log.Printf("navigation: loaded %d pages", countNodes(nav))

	renderer, err := markdown.NewRenderer()
	if err != nil {
		log.Fatalf("failed to create renderer: %v", err)
	}

	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		log.Fatalf("failed to build search index: %v", err)
	}
	log.Printf("search: indexed %d documents", idx.DocCount())

	// In dev mode: read templates/assets from the real filesystem.
	// In prod mode: use the embedded FS baked into the binary.
	var staticFS fs.FS
	if *dev {
		// Resolve the source directory containing templates/ and assets/.
		// Assumes dyno is run from its own source root (where go.mod lives).
		sourceRoot, err := filepath.Abs(*dir)
		if err != nil {
			log.Fatalf("cannot resolve source root: %v", err)
		}
		// Check if templates/ exists here; if not, try the binary's own directory.
		if _, err := os.Stat(filepath.Join(sourceRoot, "templates")); os.IsNotExist(err) {
			exe, _ := os.Executable()
			sourceRoot = filepath.Dir(exe)
		}
		staticFS = os.DirFS(sourceRoot)
	} else {
		staticFS = embeddedFS
	}

	cfg := server.Config{
		SiteRoot: siteRoot,
		Port:     *port,
		DevMode:  *dev,
		SiteCfg:  siteCfg,
	}
	srv, err := server.New(cfg, staticFS, nav, idx, renderer)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if *watch {
		plainText := func(src string) (string, error) {
			return renderer.ToPlainText([]byte(src))
		}
		if err := watcher.Watch(siteRoot, siteCfg.GetBasePath(), srv, plainText); err != nil {
			log.Fatalf("failed to start watcher: %v", err)
		}
		log.Printf("watch: monitoring %s/site/ for changes", siteRoot)
	}

	addr := ":" + *port
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGTERM / SIGINT
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("dyno listening on http://localhost%s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func countNodes(node *navigation.NavNode) int {
	count := 0
	if node.FSPath != "" {
		count = 1
	}
	for _, child := range node.Children {
		count += countNodes(child)
	}
	return count
}
