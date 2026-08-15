package httpauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

func Bearer(expectedToken string, next http.Handler) http.Handler {
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
