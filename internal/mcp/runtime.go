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

// MCPトランスポートの起動設定。
type RuntimeConfig struct {
	Transport   string
	Addr        string
	BearerToken string
}

// MCPサーバーを起動し、停止まで待機。
func Run(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	if cfg.Transport == "stdio" {
		slog.Info("starting MCP server", "transport", "stdio")
		return server.Run(ctx, &mcpsdk.StdioTransport{})
	}
	return runHTTP(ctx, cfg, server)
}

func runHTTP(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	// セッション不要のステートレスJSON応答。
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	mux := http.NewServeMux()
	// MCPのみ認証。healthzは監視用に公開。
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
		// 処理中の投稿を待って終了。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// 固定トークン認証。一般公開時はOAuth 2.1へ置換。
func bearer(expectedToken string, next http.Handler) http.Handler {
	// トークン長に依存しない比較。
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
