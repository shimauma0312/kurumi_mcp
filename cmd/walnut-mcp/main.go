package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"walnut_mcp/internal/config"
	"walnut_mcp/internal/discord"
	"walnut_mcp/internal/httpauth"
	"walnut_mcp/internal/mcpserver"
)

func main() {
	if err := run(); err != nil {
		slog.Error("walnut-mcp stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	discordClient, err := discord.NewClient(
		&http.Client{Timeout: cfg.HTTPTimeout},
		cfg.DiscordAPIBaseURL,
		cfg.DiscordBotToken,
		cfg.DiscordChannelID,
	)
	if err != nil {
		return fmt.Errorf("create Discord client: %w", err)
	}
	mcpServer := mcpserver.New(discordClient, cfg.DiscordEmbedColor)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.MCPTransport == "stdio" {
		slog.Info("starting MCP server", "transport", "stdio")
		return mcpServer.Run(ctx, &mcp.StdioTransport{})
	}
	return runHTTP(ctx, cfg, mcpServer)
}

func runHTTP(ctx context.Context, cfg config.Config, mcpServer *mcp.Server) error {
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpauth.Bearer(cfg.MCPBearerToken, mcpHandler))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	httpServer := &http.Server{
		Addr:              cfg.MCPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting MCP server", "transport", "http", "address", cfg.MCPAddr, "endpoint", "/mcp")
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
