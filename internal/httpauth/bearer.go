// Package httpauth は、単一ユーザー向けStreamable HTTP構成で使用する
// 認証ミドルウェアを提供する。
package httpauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// Bearer は、設定済みの固定トークンを持つリクエストだけを許可する。
// MCPを一般公開する場合は、このMVP用機構をMCP互換のOAuth 2.1へ置き換えること。
func Bearer(expectedToken string, next http.Handler) http.Handler {
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
