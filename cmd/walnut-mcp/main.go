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
	// プロセスに設定済みの環境変数は.envより優先される。
	// ローカル開発の利便性を保ちつつ、コンテナやサービス管理下の設定を
	// .envで意図せず上書きしないための挙動である。
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
		// stdioモードの起動と監視はMCPホストが行う。
		// Discord操作はすべてREST APIを使うため、Gateway接続は不要である。
		slog.Info("starting MCP server", "transport", "stdio")
		return mcpServer.Run(ctx, &mcp.StdioTransport{})
	}
	return runHTTP(ctx, cfg, mcpServer)
}

func runHTTP(ctx context.Context, cfg config.Config, mcpServer *mcp.Server) error {
	// このツールにはセッション状態やサーバーからクライアントへの要求がないため、
	// ステートレスなJSON応答で十分であり、プロキシやトンネルも構成しやすい。
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	mux := http.NewServeMux()
	// 特権操作を扱うMCPルートだけを認証で保護する。
	// healthzは設定やDiscordデータを公開しないため、監視用途に認証なしで提供する。
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
		// サービス管理機構またはユーザーによる停止時、処理中のDiscord投稿が
		// 完了できるよう短い猶予時間を設ける。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
