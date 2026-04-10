package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	charmlog "github.com/charmbracelet/log"
	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/server"
	"github.com/mares/dyno/internal/watcher"
)

//go:embed templates assets
var embeddedFS embed.FS

//go:embed VERSION
var versionFile string

var (
	version     = strings.TrimSpace(versionFile)
	buildCommit = "dev"
	buildDate   = "unknown"
)

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	dir := flag.String("dir", ".", "Directory containing site/ folder")
	dev := flag.Bool("dev", false, "Dev mode: reload templates and assets from disk on every request")
	watch := flag.Bool("watch", false, "Watch site/ for changes and reload navigation/search automatically")
	logFormat := flag.String("log-format", "text", "Log format: text or json")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.StringVar(port, "p", "3000", "Port (shorthand)")
	flag.StringVar(dir, "d", ".", "Directory (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("dyno v%s (commit %s, built %s)\n", version, buildCommit, buildDate)
		return
	}

	setupLogger(*logFormat)

	siteRoot, err := filepath.Abs(*dir)
	if err != nil {
		slog.Error("invalid directory", "err", err)
		os.Exit(1)
	}

	siteDir := filepath.Join(siteRoot, "site")
	if _, err := os.Stat(siteDir); os.IsNotExist(err) {
		slog.Error("site directory not found", "site_root", siteRoot)
		fmt.Fprintf(os.Stderr, "Create a site/ directory with Markdown files to get started.\n")
		os.Exit(1)
	}

	siteCfg, err := config.Load(siteRoot)
	if err != nil {
		slog.Error("failed to load dyno.yaml", "err", err)
		os.Exit(1)
	}

	slog.Info("loading docs", "version", version, "build_commit", buildCommit, "build_date", buildDate, "title", siteCfg.Title, "site_dir", siteDir)
	if *dev {
		slog.Info("dev mode enabled", "templates", "disk", "assets", "disk")
	}

	nav, err := navigation.BuildTree(siteRoot, siteCfg.GetBasePath())
	if err != nil {
		slog.Error("failed to build navigation tree", "err", err)
		os.Exit(1)
	}
	slog.Info("navigation loaded", "pages", countNodes(nav))

	renderer, err := markdown.NewRenderer()
	if err != nil {
		slog.Error("failed to create renderer", "err", err)
		os.Exit(1)
	}

	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		slog.Error("failed to build search index", "err", err)
		os.Exit(1)
	}
	slog.Info("search indexed", "documents", idx.DocCount())

	// In dev mode: read templates/assets from the real filesystem.
	// In prod mode: use the embedded FS baked into the binary.
	var staticFS fs.FS
	if *dev {
		// Resolve the source directory containing templates/ and assets/.
		// Assumes dyno is run from its own source root (where go.mod lives).
		sourceRoot, err := filepath.Abs(*dir)
		if err != nil {
			slog.Error("cannot resolve source root", "err", err)
			os.Exit(1)
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
		SiteRoot:  siteRoot,
		Port:      *port,
		DevMode:   *dev,
		SiteCfg:   siteCfg,
		BuildTime: parseBuildTime(buildDate),
	}
	srv, err := server.New(cfg, staticFS, nav, idx, renderer)
	if err != nil {
		slog.Error("failed to create server", "err", err)
		os.Exit(1)
	}

	if *watch {
		plainText := func(src string) (string, error) {
			return renderer.ToPlainText([]byte(src))
		}
		if err := watcher.Watch(siteRoot, siteCfg.GetBasePath(), srv, plainText); err != nil {
			slog.Error("failed to start watcher", "err", err)
			os.Exit(1)
		}
		slog.Info("watcher enabled", "site_dir", filepath.Join(siteRoot, "site"))
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
		slog.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
	}()

	slog.Info("server listening", "addr", "http://localhost"+addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func setupLogger(format string) {
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = charmlog.NewWithOptions(os.Stdout, charmlog.Options{
			Level:           charmlog.InfoLevel,
			TimeFormat:      time.RFC3339,
			ReportTimestamp: true,
		})
	}
	slog.SetDefault(slog.New(handler))
}

func parseBuildTime(value string) time.Time {
	if value == "" || value == "unknown" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
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
