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

	"github.com/joho/godotenv"

	"walnut_mcp/internal/config"
	"walnut_mcp/internal/discord"
	mcpservice "walnut_mcp/internal/mcp"
)

func main() {
	if err := run(); err != nil {
		slog.Error("walnut-mcp stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 既存の環境変数を優先して.envを読み込み。
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
	mcpServer := mcpservice.NewServer(discordClient, cfg.DiscordEmbedColor)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return mcpservice.Run(ctx, mcpservice.RuntimeConfig{
		Transport:   cfg.MCPTransport,
		Addr:        cfg.MCPAddr,
		BearerToken: cfg.MCPBearerToken,
	}, mcpServer)
}
