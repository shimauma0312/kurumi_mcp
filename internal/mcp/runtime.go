package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxMCPRequestBodyLength = 1 << 20

// MCPトランスポートの起動設定。
type RuntimeConfig struct {
	Transport   string
	Addr        string
	BearerToken string
}

// MCPサーバーを起動し、停止まで待機。
func Run(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	// ローカルMCPホストからの子プロセス接続。
	if cfg.Transport == "stdio" {
		slog.Info("starting MCP server", "transport", "stdio")
		return server.Run(ctx, &mcpsdk.StdioTransport{})
	}

	// ネットワーク経由のStreamable HTTP接続。
	return runHTTP(ctx, cfg, server)
}

func runHTTP(ctx context.Context, cfg RuntimeConfig, server *mcpsdk.Server) error {
	// RuntimeConfigを直接組み立てても外部インターフェースでは待ち受けない。
	if !isLoopbackAddress(cfg.Addr) {
		return errors.New("MCP HTTP address must use a loopback host and port")
	}

	handler := newHTTPHandler(cfg, server)

	// 待受アドレスとHTTPタイムアウトを設定。
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	return serveHTTP(ctx, httpServer, listener)
}

// 待受を開始し、終了結果をメイン処理へ通知。

func serveHTTP(ctx context.Context, httpServer *http.Server, listener net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting MCP server", "transport", "http", "address", listener.Addr().String(), "endpoint", "/mcp")
		errCh <- httpServer.Serve(listener)
	}()

	// サーバー異常終了またはOSの停止要求を待機。
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

// ローカルHTTP用のルーティングと防御を構成。
func newHTTPHandler(cfg RuntimeConfig, server *mcpsdk.Server) http.Handler {
	// MCPリクエストをステートレスJSONとして処理。
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	limitedHandler := limitRequestBody(mcpHandler, maxMCPRequestBodyLength)
	crossOriginProtection := http.NewCrossOriginProtection()

	// MCPだけに本文上限、ブラウザーの別オリジン拒否、Bearer認証を適用。
	mux := http.NewServeMux()
	mux.Handle("/mcp", crossOriginProtection.Handler(bearer(cfg.BearerToken, limitedHandler)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return mux
}

// Content-Lengthと実読込量の両方で本文上限を適用。
func limitRequestBody(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.MaxBytesHandler(next, limit).ServeHTTP(w, r)
	})
}

// HTTP待受がループバックに限定されているか判定。
func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// トークン認証。
func bearer(expectedToken string, next http.Handler) http.Handler {
	// 設定トークンを固定長へ変換。
	expectedHash := sha256.Sum256([]byte(expectedToken))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// AuthorizationヘッダーからBearer Tokenを取得。
		authorization := r.Header.Get("Authorization")
		providedToken, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || strings.TrimSpace(providedToken) == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="walnut-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// トークン長に依存しない比較。
		providedHash := sha256.Sum256([]byte(providedToken))
		if subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="walnut-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 認証成功時だけMCPハンドラーへ引き渡し。
		next.ServeHTTP(w, r)
	})
}
