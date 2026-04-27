package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	charmlog "github.com/charmbracelet/log"
	"github.com/mares/dyno/internal/mcp"
	"github.com/mares/dyno/internal/mcpengine"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var (
		sites         []string
		transport     string
		listen        string
		path          string
		logFormat     string
		authToken     string
		allowOrigins  []string
		publicBaseURL string
	)

	root := &cobra.Command{
		Use:           "dyno-mcp",
		Short:         "MCP server for dyno documentation",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve dyno documentation via MCP",
		RunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(logFormat)

			if authToken == "" {
				authToken = strings.TrimSpace(os.Getenv("DYNO_MCP_AUTH_TOKEN"))
			}

			engine, err := mcpengine.Load(sites, publicBaseURL)
			if err != nil {
				return err
			}

			server := mcp.New(mcp.Config{
				Name:         "dyno-mcp",
				Version:      version,
				Engine:       engine,
				AuthToken:    authToken,
				AllowOrigins: allowOrigins,
			})

			switch transport {
			case "stdio":
				slog.Info("dyno-mcp stdio server started", "mode", engine.Mode(), "books", len(engine.ListBooks()))
				return server.ServeStdio()
			case "http":
				return serveHTTP(server, listen, path, engine)
			default:
				return fmt.Errorf("unsupported transport %q", transport)
			}
		},
	}

	serveCmd.Flags().StringArrayVarP(&sites, "site", "s", []string{"./site"}, "Content directory (repeat for library mode)")
	serveCmd.Flags().StringVar(&transport, "transport", "stdio", "MCP transport: stdio or http")
	serveCmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8090", "HTTP listen address")
	serveCmd.Flags().StringVar(&path, "path", "/mcp", "HTTP MCP endpoint path")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "text", "Log format: text or json")
	serveCmd.Flags().StringVar(&authToken, "auth-token", "", "Bearer token for HTTP mode (or DYNO_MCP_AUTH_TOKEN)")
	serveCmd.Flags().StringArrayVar(&allowOrigins, "allow-origin", nil, "Allowed Origin header (repeatable)")
	serveCmd.Flags().StringVar(&publicBaseURL, "public-base-url", "", "Public dyno base URL used to return clickable links")

	root.AddCommand(serveCmd)

	if err := root.Execute(); err != nil {
		slog.Error("dyno-mcp failed", "err", err)
		os.Exit(1)
	}
}

func serveHTTP(server *mcp.Server, listen, path string, engine *mcpengine.Engine) error {
	if path == "" {
		path = "/mcp"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	mux := http.NewServeMux()
	mux.Handle(path, server)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	httpServer := &http.Server{
		Addr:         listen,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		slog.Info("shutting down dyno-mcp")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
	}()

	slog.Info("dyno-mcp http server listening",
		"addr", "http://"+listen+path,
		"mode", engine.Mode(),
		"books", len(engine.ListBooks()),
	)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func setupLogger(format string) {
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = charmlog.NewWithOptions(os.Stderr, charmlog.Options{
			Level:           charmlog.InfoLevel,
			TimeFormat:      time.RFC3339,
			ReportTimestamp: true,
		})
	}
	slog.SetDefault(slog.New(handler))
}
