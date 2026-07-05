package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type jwtClaims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"` // "admin" o "cliente"
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
}

func b64(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// signJWT emite un token HS256 para el usuario y rol dados, válido por ttl.
func signJWT(secret, user, role string, ttl time.Duration) string {
	header := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now()
	payload, _ := json.Marshal(jwtClaims{Sub: user, Role: role, Iat: now.Unix(), Exp: now.Add(ttl).Unix()})
	signing := header + "." + b64(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + b64(mac.Sum(nil))
}

// verifyJWT valida firma y expiración; devuelve el subject y el rol.
func verifyJWT(secret, token string) (string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("formato de token inválido")
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	expected := b64(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return "", "", fmt.Errorf("firma inválida")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("payload ilegible")
	}
	var claims jwtClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return "", "", fmt.Errorf("claims ilegibles")
	}
	if time.Now().Unix() > claims.Exp {
		return "", "", fmt.Errorf("token expirado")
	}
	return claims.Sub, claims.Role, nil
}

func (s *Server) authMiddleware(requiredRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token requerido"})
			return
		}
		user, role, err := verifyJWT(s.jwtSecret, strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if requiredRole != "" && role != requiredRole {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "se requiere rol " + requiredRole})
			return
		}
		r.Header.Set("X-User", user)
		r.Header.Set("X-Role", role)
		next(w, r)
	}
}
