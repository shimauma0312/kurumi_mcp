package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RuntimeConfig は、MCPトランスポートの起動に必要な設定だけを保持する。
type RuntimeConfig struct {
	Transport   string
	Addr        string
	BearerToken string
}

// Run は設定されたトランスポートでMCPサーバーを起動し、停止まで待機する。
func Run(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	if cfg.Transport == "stdio" {
		// stdioモードの起動と監視はMCPホストが行う。
		// Discord操作はすべてREST APIを使うため、Gateway接続は不要である。
		slog.Info("starting MCP server", "transport", "stdio")
		return server.Run(ctx, &mcpsdk.StdioTransport{})
	}
	return runHTTP(ctx, cfg, server)
}

func runHTTP(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	// このツールにはセッション状態やサーバーからクライアントへの要求がないため、
	// ステートレスなJSON応答で十分であり、プロキシやトンネルも構成しやすい。
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	mux := http.NewServeMux()
	// 特権操作を扱うMCPルートだけを認証で保護する。
	// healthzは設定やDiscordデータを公開しないため、監視用途に認証なしで提供する。
	mux.Handle("/mcp", bearer(cfg.BearerToken, mcpHandler))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting MCP server", "transport", "http", "address", cfg.Addr, "endpoint", "/mcp")
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

// bearer は、設定済みの固定トークンを持つリクエストだけを許可する。
// MCPを一般公開する場合は、このMVP用機構をMCP互換のOAuth 2.1へ置き換えること。
func bearer(expectedToken string, next http.Handler) http.Handler {
	// 比較前に両方を固定長へハッシュ化することで、文字列比較の処理時間や
	// トークン長の違いから情報が漏れることを避ける。
	expectedHash := sha256.Sum256([]byte(expectedToken))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		providedToken, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || strings.TrimSpace(providedToken) == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="walnut-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		providedHash := sha256.Sum256([]byte(providedToken))
		if subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="walnut-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
