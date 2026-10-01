package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mares/dyno/internal/browse"
	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/markdown"
	"github.com/mares/dyno/internal/navigation"
	"github.com/mares/dyno/internal/search"
	"github.com/mares/dyno/internal/server"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func newBrowseCommand() *cobra.Command {
	var port int
	var noOpen, noWatch, edit bool
	var excludes []string
	cmd := &cobra.Command{
		Use:     "browse [PATH...]",
		Short:   "Browse selected local Markdown files and directories",
		Long:    "Read local Markdown in a browser with a filesystem sidebar and search. Directories are scanned recursively. With no paths, browse the current directory. Local links cannot add sources to the selection.",
		Example: "  dyno browse README.md\n  dyno browse ./docs ./notes ./TODO.md\n  dyno browse . --exclude '**/archive/**' --edit",
		RunE: func(cmd *cobra.Command, paths []string) error {
			if port < 0 || port > 65535 {
				return fmt.Errorf("--port must be between 0 and 65535")
			}
			return runBrowse(cmd.Context(), paths, excludes, port, !noOpen, !noWatch, edit)
		},
	}
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Local port (0 selects a free port)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the web browser automatically")
	cmd.Flags().BoolVar(&noWatch, "no-watch", false, "Do not watch sources or refresh the reader automatically")
	cmd.Flags().BoolVar(&edit, "edit", false, "Enable editing the selected Markdown documents")
	cmd.Flags().StringArrayVar(&excludes, "exclude", nil, "Exclude a relative glob from recursive scans (repeatable; supports **)")
	return cmd
}

func runBrowse(ctx context.Context, paths, excludes []string, port int, open, watch, edit bool) error {
	setupLogger("text")
	catalog, err := browse.New(paths, excludes)
	if err != nil {
		return err
	}
	defer catalog.Close()
	nav, signature, err := catalog.Build()
	if err != nil {
		return fmt.Errorf("browse navigation: %w", err)
	}
	renderer, err := markdown.NewRenderer()
	if err != nil {
		return err
	}
	plainText := func(src string) (string, error) { return renderer.ToPlainText([]byte(src)) }
	idx, err := search.BuildIndexWithReader(nav, plainText, catalog.ReadFile)
	if err != nil {
		return err
	}
	basePath := browse.BasePath
	siteCfg := &config.SiteConfig{Title: "Dyno Browse", BasePath: &basePath}
	siteCfg.Defaults()
	srv, err := server.New(server.Config{
		SiteCfg: siteCfg, EditMode: edit, Browse: catalog, BrowseWatch: watch,
		Version: version, Commit: buildCommit, BuildTime: parseBuildTime(buildDate),
	}, embeddedFS, nav, idx, renderer)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("browse listener: %w", err)
	}
	defer listener.Close()
	home := basePath + "/"
	if len(nav.Children) == 1 && !nav.Children[0].IsDir {
		home = nav.Children[0].FullPath
	}
	handler := srv.Handler()
	httpSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, home, http.StatusTemporaryRedirect)
				return
			}
			handler.ServeHTTP(w, r)
		}),
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	if watch {
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			watchBrowse(watchCtx, catalog, signature, time.Second, func(tree *navigation.NavNode) {
				index, err := search.BuildIndexWithReader(tree, plainText, catalog.ReadFile)
				if err != nil {
					slog.Error("browse index", "err", err)
					return
				}
				srv.Reload(tree, index)
			})
		}()
		defer func() { cancelWatch(); <-watchDone }()
	}
	go func() {
		<-watchCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	address := "http://" + listener.Addr().String() + home
	fmt.Println(address)
	slog.Info("browse ready", "documents", idx.DocCount(), "watch", watch, "edit", edit)
	if open {
		go func() {
			if err := browser.OpenURL(address); err != nil {
				slog.Warn("could not open browser; use the printed URL", "err", err)
			}
		}()
	}
	if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Scanning opened roots avoids watching a standalone file's entire parent tree.
// Only changed snapshots rebuild the search index and refresh the reader.
func watchBrowse(ctx context.Context, catalog *browse.Catalog, signature string, interval time.Duration, reload func(*navigation.NavNode)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nav, next, err := catalog.Build()
			if err != nil {
				slog.Warn("browse refresh", "err", err)
				continue
			}
			if next != signature {
				reload(nav)
				signature = next
			}
		}
	}
}
