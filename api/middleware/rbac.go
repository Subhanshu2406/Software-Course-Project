package middleware

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// RequireRole ensures the user has a specific role.
func RequireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		userRole, ok := claims["role"].(string)
		if !ok || userRole != role {
			http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}
