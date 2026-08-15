package mcp

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPのBearer認証が正しいAuthorizationヘッダーだけを許可することを検証。
// 正しいトークンは後段ハンドラーまで到達して204となり、ヘッダーなし、
// 不正トークン、Bearer以外の認証方式はいずれも401となることを確認する。
func TestBearer(t *testing.T) {
	// 認証成功時だけ呼ばれる後段ハンドラー。204を到達確認に使用。
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bearer("correct-token", next)

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "valid", authorization: "Bearer correct-token", wantStatus: http.StatusNoContent},
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong", authorization: "Bearer wrong-token", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authorization: "Basic correct-token", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 各認証ヘッダーでMCPへのPOSTリクエストを再現。
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

// HTTPルーティングで、監視用healthzだけが認証なしで利用できることを検証。
// MCPエンドポイントとは認証境界を分け、死活監視がTokenを必要とせず200と本文okを返すことを確認する。
func TestHTTPHandlerExposesHealthCheck(t *testing.T) {
	handler := newHTTPHandler(RuntimeConfig{BearerToken: strings.Repeat("a", 32)}, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok\n" {
		t.Fatalf("status = %d, body = %q, want 200 and ok", recorder.Code, recorder.Body.String())
	}
}

// ブラウザーから別オリジンで送られたMCP書き込みを、正しいBearer Token付きでも拒否することを検証。
// Token認証だけでは防げないCSRF経路をCrossOriginProtectionで閉じ、後段のMCP処理へ到達させない。
func TestHTTPHandlerRejectsCrossOriginRequest(t *testing.T) {
	token := strings.Repeat("b", 32)
	handler := newHTTPHandler(RuntimeConfig{BearerToken: token}, nil)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// MCPリクエスト本文が1MiBを超えた場合、SDKが全量を処理する前にHTTP層で拒否することを検証。
// 有効なBearer Tokenと同一オリジン条件を満たしても、本文サイズ制限は独立して働くことを確認する。
func TestHTTPHandlerLimitsRequestBody(t *testing.T) {
	token := strings.Repeat("c", 32)
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
	handler := newHTTPHandler(RuntimeConfig{BearerToken: token}, server)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(make([]byte, maxMCPRequestBodyLength+1)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %q, want 413", recorder.Code, recorder.Body.String())
	}
}

// RuntimeConfigを直接渡した場合でも、外部インターフェースのHTTP待受を開始しないことを検証。
// config.Loadを経由しない呼び出しでも0.0.0.0を拒否し、Bearer Tokenの平文公開を防ぐ多層防御を確認する。
func TestRunHTTPRejectsNonLoopbackAddress(t *testing.T) {
	err := runHTTP(context.Background(), RuntimeConfig{Addr: "0.0.0.0:8765"}, nil)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error = %v, want loopback validation error", err)
	}
}

// 実際のTCP ListenerでHTTPサーバーを開始し、context取消によって正常終了できることを検証。
// Serve開始後にhealthzへ接続できることを確認してから停止し、起動直後の競合を成功扱いしない。
func TestServeHTTPStartsAndStops(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{
		Handler: newHTTPHandler(RuntimeConfig{BearerToken: strings.Repeat("d", 32)}, nil),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTP(ctx, httpServer, listener)
	}()

	// Listenerは呼び出し前に確保済みなので、短い期限内にhealthzが応答するまで接続を試す。
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		cancel()
		t.Fatalf("health check: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("health status = %d, want 200", response.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTP() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveHTTP did not stop after context cancellation")
	}
}
