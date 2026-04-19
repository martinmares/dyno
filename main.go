package main

import (
	"context"
	"embed"
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
	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/server"
	"github.com/mares/dyno/internal/watcher"
	"github.com/spf13/cobra"
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
	var (
		sites     []string
		port      string
		dev       bool
		watch     bool
		logFormat string
	)

	root := &cobra.Command{
		Use:   "dyno",
		Short: "Self-hosted documentation server",
		Long:  "dyno serves Markdown documentation as a beautiful, searchable web site.",
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, buildCommit, buildDate),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(sites, port, dev, watch, logFormat)
		},
	}

	root.Flags().StringArrayVarP(&sites, "site", "s", []string{"./site"}, "Site directory (repeat for library mode)")
	root.Flags().StringVarP(&port, "port", "p", "3000", "Port to listen on")
	root.Flags().BoolVar(&dev, "dev", false, "Dev mode: reload templates and assets from disk on every request")
	root.Flags().BoolVar(&watch, "watch", false, "Watch site for changes and reload navigation/search (single-site only)")
	root.Flags().StringVar(&logFormat, "log-format", "text", "Log format: text or json")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(sites []string, port string, dev, watch bool, logFormat string) error {
	setupLogger(logFormat)

	if len(sites) > 1 && watch {
		slog.Warn("--watch is not supported in library mode, ignoring")
		watch = false
	}

	// Resolve all site paths to absolute.
	absSites := make([]string, 0, len(sites))
	for _, s := range sites {
		abs, err := filepath.Abs(s)
		if err != nil {
			return fmt.Errorf("invalid site path %q: %w", s, err)
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return fmt.Errorf("site directory not found: %s", abs)
		}
		absSites = append(absSites, abs)
	}

	if dev {
		slog.Info("dev mode enabled", "templates", "disk", "assets", "disk")
	}

	// Shared renderer — all books use the same Goldmark/D2/Mermaid setup.
	// Template env is loaded from the first site (global env applies to all).
	firstSiteRoot := resolveSiteRoot(absSites[0])
	templateEnv, err := config.LoadTemplateEnv(firstSiteRoot)
	if err != nil {
		return fmt.Errorf("failed to load template env: %w", err)
	}
	renderer, err := markdown.NewRendererWithEnv(templateEnv)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	staticFS := resolveStaticFS(dev, firstSiteRoot)

	if len(absSites) == 1 {
		return runSingle(absSites[0], port, dev, watch, renderer, staticFS)
	}
	return runLibrary(absSites, port, dev, renderer, staticFS)
}

// resolveSiteRoot: if siteDir has a site/ subdir, it is the root; otherwise parent is root.
func resolveSiteRoot(siteDir string) string {
	if _, err := os.Stat(filepath.Join(siteDir, "site")); err == nil {
		return siteDir
	}
	return filepath.Dir(siteDir)
}

func runSingle(siteDir, port string, dev, watch bool, renderer *markdown.Renderer, staticFS fs.FS) error {
	siteRoot := resolveSiteRoot(siteDir)

	siteCfg, err := config.Load(siteRoot)
	if err != nil {
		return fmt.Errorf("failed to load dyno.yaml: %w", err)
	}

	slog.Info("loading docs",
		"version", version,
		"build_commit", buildCommit,
		"title", siteCfg.Title,
		"site_dir", siteDir,
	)

	nav, err := navigation.BuildTree(siteRoot, siteCfg.GetBasePath())
	if err != nil {
		return fmt.Errorf("failed to build navigation tree: %w", err)
	}
	slog.Info("navigation loaded", "pages", countNodes(nav))

	idx, err := search.BuildIndex(nav, func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	})
	if err != nil {
		return fmt.Errorf("failed to build search index: %w", err)
	}
	slog.Info("search indexed", "documents", idx.DocCount())

	cfg := server.Config{
		SiteRoot:  siteRoot,
		Port:      port,
		DevMode:   dev,
		SiteCfg:   siteCfg,
		Version:   version,
		Commit:    buildCommit,
		BuildTime: parseBuildTime(buildDate),
	}
	srv, err := server.New(cfg, staticFS, nav, idx, renderer)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	if watch {
		plainText := func(src string) (string, error) {
			return renderer.ToPlainText([]byte(src))
		}
		if err := watcher.Watch(siteRoot, siteCfg.GetBasePath(), srv, plainText); err != nil {
			return fmt.Errorf("failed to start watcher: %w", err)
		}
		slog.Info("watcher enabled", "site_dir", filepath.Join(siteRoot, "site"))
	}

	return serve(srv, port)
}

func runLibrary(siteDirs []string, port string, dev bool, renderer *markdown.Renderer, staticFS fs.FS) error {
	slog.Info("library mode", "books", len(siteDirs))

	// Load global basePath from first site's dyno.yaml (or use default).
	firstRoot := resolveSiteRoot(siteDirs[0])
	globalCfg, err := config.Load(firstRoot)
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}
	globalBasePath := globalCfg.GetBasePath()

	plainText := func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	}

	books := make([]*library.Book, 0, len(siteDirs))
	for _, siteDir := range siteDirs {
		book, err := library.Load(siteDir, globalBasePath, plainText)
		if err != nil {
			return fmt.Errorf("failed to load book %q: %w", siteDir, err)
		}
		books = append(books, book)
	}

	libCfg := server.LibraryConfig{
		Title:     globalCfg.Title,
		LogoText:  globalCfg.LogoText,
		Subtitle:  globalCfg.Description,
		BasePath:  globalBasePath,
		DevMode:   dev,
		Version:   version,
		Commit:    buildCommit,
		BuildTime: parseBuildTime(buildDate),
	}
	libSrv, err := server.NewLibrary(libCfg, staticFS, books, renderer)
	if err != nil {
		return fmt.Errorf("failed to create library server: %w", err)
	}

	return serveLibrary(libSrv, port)
}

func resolveStaticFS(dev bool, siteRoot string) fs.FS {
	if !dev {
		return embeddedFS
	}
	sourceRoot := siteRoot
	if _, err := os.Stat(filepath.Join(sourceRoot, "templates")); os.IsNotExist(err) {
		exe, _ := os.Executable()
		sourceRoot = filepath.Dir(exe)
	}
	return os.DirFS(sourceRoot)
}

func serveLibrary(srv *server.LibraryServer, port string) error {
	addr := ":" + port
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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

	slog.Info("library server listening", "addr", "http://localhost"+addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func serve(srv *server.Server, port string) error {
	addr := ":" + port
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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
		return fmt.Errorf("server error: %w", err)
	}
	return nil
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
