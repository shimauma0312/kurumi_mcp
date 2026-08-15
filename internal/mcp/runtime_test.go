package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
