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
	"github.com/mares/dyno/internal/gitrepo"
	"github.com/mares/dyno/internal/library"
	"github.com/mares/dyno/internal/libraryconfig"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/server"
	"github.com/mares/dyno/internal/sitepath"
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
		sites       []string
		gitRepos    []string
		workDir     string
		libraryFile string
		port        string
		dev         bool
		edit        bool
		watch       bool
		logFormat   string
	)

	root := &cobra.Command{
		Use:          "dyno",
		Short:        "Self-hosted documentation server",
		Long:         "dyno serves Markdown documentation as a beautiful, searchable web site.",
		Version:      fmt.Sprintf("%s (commit %s, built %s)", version, buildCommit, buildDate),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(sites, gitRepos, workDir, libraryFile, port, dev, edit, watch, logFormat)
		},
	}

	root.Flags().StringArrayVarP(&sites, "site", "s", []string{"./site"}, "Content directory (repeat for library mode)")
	root.Flags().StringArrayVar(&gitRepos, "git-repo-site", nil, "Git repo URL to clone and serve as a site (repeat for library mode)")
	root.Flags().StringVar(&workDir, "work-dir", "", "Writable directory for git clones (required with --git-repo-site)")
	root.Flags().StringVar(&libraryFile, "library", "", "Path to dyno-library.yaml with repo metadata and overrides")
	root.Flags().StringVarP(&port, "port", "p", "3000", "Port to listen on")
	root.Flags().BoolVar(&dev, "dev", false, "Dev mode: reload templates and assets from disk on every request")
	root.Flags().BoolVar(&edit, "edit", false, "Enable in-browser markdown editor (local use only)")
	root.Flags().BoolVar(&watch, "watch", false, "Watch site for changes and reload navigation/search (single-site only)")
	root.Flags().StringVar(&logFormat, "log-format", "text", "Log format: text or json")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(sites, gitRepos []string, workDir, libraryFile, port string, dev, edit, watch bool, logFormat string) error {
	setupLogger(logFormat)

	// DYNO_EDIT=true enables editor mode even without --edit flag.
	if os.Getenv("DYNO_EDIT") == "true" {
		edit = true
	}
	if edit {
		slog.Info("edit mode enabled — in-browser markdown editor active")
	}

	// gitConfigs maps resolvedDir → gitrepo.Config for auto-pull wiring.
	gitConfigs := map[string]gitrepo.Config{}
	// cfgOverrides maps resolvedDir → SiteConfig fallback from dyno-library.yaml.
	cfgOverrides := map[string]*config.SiteConfig{}

	// Track whether the user explicitly passed --site flags (vs default).
	hasExplicitSites := len(sites) > 0 && !(len(sites) == 1 && sites[0] == "./site")

	// Load dyno-library.yaml if provided.
	if libraryFile != "" {
		libFile, err := libraryconfig.Load(libraryFile)
		if err != nil {
			return fmt.Errorf("--library: %w", err)
		}

		// Resolve work_dir: CLI --work-dir takes precedence over yaml work_dir.
		if workDir == "" && libFile.WorkDir != "" {
			workDir = libFile.WorkDir
		}

		// Process sites from the library file (in order).
		// They are prepended before CLI --site / --git-repo-site so file order is preserved.
		var libSites []string
		for _, entry := range libFile.Sites {
			if entry.IsGit() {
				// Defer git clone until workDir is resolved — collect into gitRepos.
				found := false
				for _, r := range gitRepos {
					if r == entry.URL {
						found = true
						break
					}
				}
				if !found {
					gitRepos = append([]string{entry.URL}, append(gitRepos[:0:0], gitRepos...)...)
					// store override keyed by URL temporarily; resolved to cloneDir below
				}
				// Store entry by URL for later cloneDir resolution.
				cfgOverrides["__url__"+entry.URL] = siteEntryToSiteConfig(entry)
			} else {
				libSites = append(libSites, entry.Path)
				cfgOverrides[entry.Path] = siteEntryToSiteConfig(entry)
			}
		}

		// Prepend library sites before CLI sites.
		// Drop default "./site" if no explicit --site was given.
		if !hasExplicitSites {
			sites = append(libSites, gitRepos...) // will be replaced by resolved dirs below
			sites = libSites
		} else {
			sites = append(libSites, sites...)
		}
	}

	// Clone git repos and append their local paths to sites.
	if len(gitRepos) > 0 {
		if workDir == "" {
			return fmt.Errorf("--work-dir is required when using --git-repo-site or git repos in --library")
		}
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid --work-dir: %w", err)
		}
		for _, repoURL := range gitRepos {
			// Retrieve override that was stored temporarily by URL.
			override := cfgOverrides["__url__"+repoURL]
			delete(cfgOverrides, "__url__"+repoURL)

			var branch string
			var pullInterval time.Duration
			if override != nil {
				branch = override.GitBranch
				if override.GitPullInterval != "" {
					pullInterval, err = gitrepo.ParseInterval(override.GitPullInterval)
					if err != nil {
						return fmt.Errorf("pull_interval for %q: %w", repoURL, err)
					}
				}
			}
			cfg := gitrepo.Config{
				RepoURL:      repoURL,
				Branch:       branch,
				PullInterval: pullInterval,
				WorkDir:      absWorkDir,
			}
			cloneDir, err := gitrepo.CloneOrUpdate(cfg)
			if err != nil {
				return fmt.Errorf("git clone %q: %w", repoURL, err)
			}
			sites = append(sites, cloneDir)
			gitConfigs[cloneDir] = cfg
			if override != nil {
				cfgOverrides[cloneDir] = override
			}
		}
	}

	// Drop default "./site" if library or git repos provided anything.
	if !hasExplicitSites && (libraryFile != "" || len(gitRepos) > 0) {
		filtered := sites[:0]
		for _, s := range sites {
			if s != "./site" {
				filtered = append(filtered, s)
			}
		}
		sites = filtered
	}

	if len(sites) > 1 && watch {
		slog.Warn("--watch is not supported in library mode, ignoring")
		watch = false
	}

	// Resolve all site paths to absolute.
	absSites := make([]string, 0, len(sites))
	for _, s := range sites {
		paths, err := sitepath.Resolve(s)
		if err != nil {
			return fmt.Errorf("invalid site path %q: %w", s, err)
		}
		absSites = append(absSites, paths.ContentDir)
	}

	if dev {
		slog.Info("dev mode enabled", "templates", "disk", "assets", "disk")
	}

	// Shared renderer — all books use the same Goldmark/D2/Mermaid setup.
	// Template env is loaded from the first site (global env applies to all).
	firstPaths, err := sitepath.Resolve(absSites[0])
	if err != nil {
		return err
	}
	firstSiteRoot := firstPaths.RootDir
	templateEnv, err := config.LoadTemplateEnvForContent(firstSiteRoot, absSites[0])
	if err != nil {
		return fmt.Errorf("failed to load template env: %w", err)
	}
	renderer, err := markdown.NewRendererWithEnv(templateEnv)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	staticFS := resolveStaticFS(dev, firstSiteRoot)

	if len(absSites) == 1 {
		return runSingleWithGit(absSites[0], port, dev, edit, watch, renderer, staticFS, gitConfigs[absSites[0]], cfgOverrides[absSites[0]])
	}
	return runLibraryWithGit(absSites, port, dev, renderer, staticFS, gitConfigs, cfgOverrides)
}

func runSingle(siteDir, port string, dev, watch bool, renderer *markdown.Renderer, staticFS fs.FS) error {
	return runSingleWithGit(siteDir, port, dev, false, watch, renderer, staticFS, gitrepo.Config{}, nil)
}

func runSingleWithGit(siteDir, port string, dev, edit, watch bool, renderer *markdown.Renderer, staticFS fs.FS, gitCfg gitrepo.Config, cfgOverride *config.SiteConfig) error {
	paths, err := sitepath.Resolve(siteDir)
	if err != nil {
		return err
	}
	siteRoot := paths.RootDir
	contentDir := paths.ContentDir

	siteCfg, err := config.LoadForContent(siteRoot, contentDir)
	if err != nil {
		return fmt.Errorf("failed to load dyno.yaml: %w", err)
	}

	// Apply external override (dyno-library.yaml) as fallback — dyno.yaml wins.
	if cfgOverride != nil {
		siteCfg.MergeDefaults(*cfgOverride)
	}

	// Merge dyno.yaml git settings into gitCfg (CLI values take precedence).
	if gitCfg.RepoURL != "" {
		if gitCfg.Branch == "" && siteCfg.GitBranch != "" {
			gitCfg.Branch = siteCfg.GitBranch
		}
		if gitCfg.PullInterval == 0 && siteCfg.GitPullInterval != "" {
			interval, err := gitrepo.ParseInterval(siteCfg.GitPullInterval)
			if err != nil {
				slog.Warn("invalid git_pull_interval in dyno.yaml, using default", "err", err)
			} else {
				gitCfg.PullInterval = interval
			}
		}
	}

	slog.Info("loading docs",
		"version", version,
		"build_commit", buildCommit,
		"title", siteCfg.Title,
		"site_dir", siteDir,
	)

	nav, err := navigation.BuildTree(contentDir, siteCfg.GetBasePath())
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
		SiteRoot:   siteRoot,
		ContentDir: contentDir,
		Port:       port,
		DevMode:    dev,
		EditMode:   edit,
		SiteCfg:    siteCfg,
		Version:    version,
		Commit:     buildCommit,
		BuildTime:  parseBuildTime(buildDate),
	}
	srv, err := server.New(cfg, staticFS, nav, idx, renderer)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	if watch {
		plainText := func(src string) (string, error) {
			return renderer.ToPlainText([]byte(src))
		}
		if err := watcher.Watch(contentDir, siteCfg.GetBasePath(), srv, plainText); err != nil {
			return fmt.Errorf("failed to start watcher: %w", err)
		}
		slog.Info("watcher enabled", "content_dir", contentDir)
	}

	// Start auto-pull if this site was cloned from a git repo.
	if gitCfg.RepoURL != "" {
		plainText := func(src string) (string, error) {
			return renderer.ToPlainText([]byte(src))
		}
		gitrepo.StartAutoPull(gitCfg, siteDir, func() {
			nav, err := navigation.BuildTree(contentDir, siteCfg.GetBasePath())
			if err != nil {
				slog.Error("git reload: build nav failed", "err", err)
				return
			}
			idx, err := search.BuildIndex(nav, plainText)
			if err != nil {
				slog.Error("git reload: build index failed", "err", err)
				return
			}
			srv.Reload(nav, idx)
		})
		interval := gitCfg.PullInterval
		if interval == 0 {
			interval = 5 * time.Minute
		}
		slog.Info("git auto-pull enabled", "repo", gitCfg.RepoURL, "interval", interval)
	}

	return serve(srv, port)
}

func runLibrary(siteDirs []string, port string, dev bool, renderer *markdown.Renderer, staticFS fs.FS) error {
	return runLibraryWithGit(siteDirs, port, dev, renderer, staticFS, nil, nil)
}

func runLibraryWithGit(siteDirs []string, port string, dev bool, renderer *markdown.Renderer, staticFS fs.FS, gitConfigs map[string]gitrepo.Config, cfgOverrides map[string]*config.SiteConfig) error {
	slog.Info("library mode", "books", len(siteDirs))

	// Load global basePath from first site's dyno.yaml (or use default).
	firstPaths, err := sitepath.Resolve(siteDirs[0])
	if err != nil {
		return err
	}
	firstRoot := firstPaths.RootDir
	globalCfg, err := config.LoadForContent(firstRoot, siteDirs[0])
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}
	globalBasePath := globalCfg.GetBasePath()

	plainText := func(src string) (string, error) {
		return renderer.ToPlainText([]byte(src))
	}

	books := make([]*library.Book, 0, len(siteDirs))
	for _, siteDir := range siteDirs {
		var override *config.SiteConfig
		if cfgOverrides != nil {
			override = cfgOverrides[siteDir]
		}
		book, err := library.Load(siteDir, globalBasePath, plainText, override)
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

	// Start auto-pull for any git-cloned books.
	for _, siteDir := range siteDirs {
		gitCfg, ok := gitConfigs[siteDir]
		if !ok || gitCfg.RepoURL == "" {
			continue
		}
		siteDir := siteDir // capture
		override := cfgOverrides[siteDir]
		gitrepo.StartAutoPull(gitCfg, siteDir, func() {
			book, err := library.Load(siteDir, globalBasePath, plainText, override)
			if err != nil {
				slog.Error("git reload: load book failed", "dir", siteDir, "err", err)
				return
			}
			libSrv.ReloadBook(book)
		})
		interval := gitCfg.PullInterval
		if interval == 0 {
			interval = 5 * time.Minute
		}
		slog.Info("git auto-pull enabled", "repo", gitCfg.RepoURL, "interval", interval)
	}

	return serveLibrary(libSrv, port)
}

func resolveStaticFS(dev bool, siteRoot string) fs.FS {
	if !dev {
		return embeddedFS
	}

	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	candidates = append(candidates, siteRoot)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, candidate := range candidates {
		if hasStaticSource(candidate) {
			return os.DirFS(candidate)
		}
	}
	return embeddedFS
}

func hasStaticSource(root string) bool {
	if root == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "templates")); err != nil || !info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "assets")); err != nil || !info.IsDir() {
		return false
	}
	return true
}

func siteEntryToSiteConfig(e libraryconfig.SiteEntry) *config.SiteConfig {
	cfg := &config.SiteConfig{
		Title:           e.Title,
		Description:     e.Description,
		LogoText:        e.LogoText,
		Slug:            e.Slug,
		Icon:            e.Icon,
		Color:           e.Color,
		GitHubURL:       e.GitHubURL,
		GitHubBranch:    e.Branch,
		GitPullInterval: e.PullInterval,
		GitBranch:       e.Branch,
	}
	if e.BasePath != "" {
		cfg.BasePath = &e.BasePath
	}
	return cfg
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
