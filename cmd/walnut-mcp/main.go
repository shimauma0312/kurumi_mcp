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

	"github.com/shimauma0312/kurumi_mcp/internal/config"
	"github.com/shimauma0312/kurumi_mcp/internal/discord"
	"github.com/shimauma0312/kurumi_mcp/internal/linkpreview"
	mcpservice "github.com/shimauma0312/kurumi_mcp/internal/mcp"
	"github.com/shimauma0312/kurumi_mcp/internal/persona"
)

func main() {
	if err := run(); err != nil {
		slog.Error("walnut-mcp stopped", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	// 既存の環境変数を優先して.envを読み込み。
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	// DiscordとMCPの設定をまとめて検証。
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Git管理外のファイルから投稿ペルソナを読み込み。
	personaInstructions, err := persona.Load(cfg.MCPPersonaFile)
	if err != nil {
		return fmt.Errorf("load persona: %w", err)
	}

	// 出典ページからOGP画像を安全に取得。
	previewClient, err := linkpreview.NewClient(cfg.HTTPTimeout)
	if err != nil {
		return fmt.Errorf("create link preview client: %w", err)
	}

	// 固定チャンネル専用のDiscordクライアントを生成。
	discordClient, err := discord.NewClient(
		&http.Client{Timeout: cfg.HTTPTimeout},
		cfg.DiscordAPIBaseURL,
		cfg.DiscordBotToken,
		cfg.DiscordChannelID,
		cfg.DiscordEmbedThumbnailURL,
		discord.WithLinkPreview(previewClient),
	)
	if err != nil {
		return fmt.Errorf("create Discord client: %w", err)
	}

	// Gateway接続でDiscord上のオンライン表示を維持。
	discordGateway, err := discord.NewGateway(cfg.DiscordBotToken)
	if err != nil {
		return fmt.Errorf("create Discord Gateway: %w", err)
	}
	if err := discordGateway.Open(); err != nil {
		return err
	}
	slog.Info("Discord Gateway connected", "presence", "online", "intents", "none")
	defer func() {
		if err := discordGateway.Close(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()

	// Discord操作をMCPツールとして公開。
	mcpServer := mcpservice.NewServer(
		discordClient,
		cfg.DiscordEmbedColor,
		personaInstructions,
		cfg.MCPMessageSuffix,
	)

	// OSの停止要求をMCPランタイムへ伝達。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 選択したトランスポートで待受開始。
	return mcpservice.Run(ctx, mcpservice.RuntimeConfig{
		Transport:   cfg.MCPTransport,
		Addr:        cfg.MCPAddr,
		BearerToken: cfg.MCPBearerToken,
	}, mcpServer)
}
