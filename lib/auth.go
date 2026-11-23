package lib

import (
	"errors"
	"net/http"
	"strings"
)

var (
	ErrMissingToken = errors.New("authorization required")
	ErrInvalidToken = errors.New("invalid or revoked token")
)

// Authenticate accepts either a paired phone device token or a desktop agent
// token. Both are stored as SHA-256 hashes, never in clear text.
func Authenticate(r *http.Request) error {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ErrMissingToken
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ErrMissingToken
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return ErrMissingToken
	}
	hash := Sha256Hex(token)
	db := NewTurso()
	res, err := db.Execute(
		`SELECT kind FROM (
			SELECT 'agent' AS kind FROM agent_tokens WHERE token_hash = ?
			UNION ALL
			SELECT 'device' AS kind FROM devices WHERE device_token_hash = ? AND revoked_at IS NULL
		) LIMIT 1`,
		Text(hash), Text(hash))
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return ErrInvalidToken
	}
	return nil
}

// RequireAuth writes the proper status code and returns false when the caller
// must not proceed.
func RequireAuth(w http.ResponseWriter, r *http.Request) bool {
	err := Authenticate(r)
	if err == nil {
		return true
	}
	status := http.StatusUnauthorized
	if err != ErrMissingToken && err != ErrInvalidToken {
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
	return false
}
